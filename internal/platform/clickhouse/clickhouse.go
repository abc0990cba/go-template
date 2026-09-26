// Package clickhouse implements the analytics warehouse side: a batching
// event sink and event-count aggregates for the dashboard. Every failure
// degrades gracefully — the operational store remains the source of truth.
package clickhouse

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/your-org/go-template/internal/model"

	"github.com/your-org/go-template/internal/service"
)

var (
	_ service.EventSink  = (*EventBuffer)(nil)
	_ service.EventStats = (*Client)(nil)
)

// Client wraps the ClickHouse connection.
type Client struct {
	conn driver.Conn
}

// Open connects and applies the migration files (one statement per file)
// from the given directory, an fs.FS rooted at migrations/clickhouse.
func Open(ctx context.Context, addr, database, username, password string, migrations fs.FS) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{addr},
		Auth:        clickhouse.Auth{Database: database, Username: username, Password: password},
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse connect: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}
	if err := applyMigrations(ctx, conn, migrations); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func applyMigrations(ctx context.Context, conn driver.Conn, fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := conn.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// EventCounts aggregates stored events by name. Unknown names are ignored;
// counters the dashboard knows about are always present (zero when absent).
func (c *Client) EventCounts(ctx context.Context) (map[string]int64, error) {
	rows, err := c.conn.Query(ctx, "SELECT event_name, toInt64(count()) FROM analytics_events GROUP BY event_name")
	if err != nil {
		return nil, fmt.Errorf("query event counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int64{}
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, fmt.Errorf("scan event counts: %w", err)
		}
		counts[name] = n
	}
	return counts, rows.Err()
}

// EventTimeseries returns per-day per-event counts for the last days days.
func (c *Client) EventTimeseries(ctx context.Context, days int) ([]model.EventDayPoint, error) {
	rows, err := c.conn.Query(ctx,
		"SELECT toString(toDate(occurred_at)) AS day, event_name, toInt64(count()) "+
			"FROM analytics_events WHERE occurred_at >= now() - toIntervalDay(?) "+
			"GROUP BY day, event_name ORDER BY day", days)
	if err != nil {
		return nil, fmt.Errorf("query event timeseries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byDay := map[string]map[string]int64{}
	for rows.Next() {
		var day, name string
		var n int64
		if err := rows.Scan(&day, &name, &n); err != nil {
			return nil, fmt.Errorf("scan event timeseries: %w", err)
		}
		counts, ok := byDay[day]
		if !ok {
			counts = map[string]int64{}
			byDay[day] = counts
		}
		counts[name] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Same ordering helper as the memory store keeps both shapes identical.
	days2 := make([]string, 0, len(byDay))
	for day := range byDay {
		days2 = append(days2, day)
	}
	sort.Strings(days2)
	points := make([]model.EventDayPoint, 0, len(days2))
	for _, day := range days2 {
		var total int64
		for _, n := range byDay[day] {
			total += n
		}
		points = append(points, model.EventDayPoint{Day: day, Events: byDay[day], Total: total})
	}
	return points, nil
}

// RecentEvents returns the newest events of the trail with their acting
// user ids, newest first.
func (c *Client) RecentEvents(ctx context.Context, limit int) ([]model.EventRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := c.conn.Query(ctx,
		"SELECT event_name, occurred_at, user_id, properties FROM analytics_events "+
			"ORDER BY occurred_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]model.EventRecord, 0, limit)
	for rows.Next() {
		var record model.EventRecord
		if err := rows.Scan(&record.Name, &record.OccurredAt, &record.UserID, &record.Properties); err != nil {
			return nil, fmt.Errorf("scan recent events: %w", err)
		}
		events = append(events, record)
	}
	return events, rows.Err()
}

// InsertEvents writes a batch of historical events (demo seed backfill) in
// one prepared-batch insert.
func (c *Client) InsertEvents(ctx context.Context, events []model.EventRecord) error {
	if len(events) == 0 {
		return nil
	}
	statement, err := c.conn.PrepareBatch(ctx, "INSERT INTO analytics_events (occurred_at, event_name, user_id, request_id, properties)")
	if err != nil {
		return fmt.Errorf("prepare backfill batch: %w", err)
	}
	for _, e := range events {
		properties := e.Properties
		if properties == "" {
			properties = "{}"
		}
		if err := statement.Append(e.OccurredAt, e.Name, e.UserID, "", properties); err != nil {
			return fmt.Errorf("append backfill row: %w", err)
		}
	}
	if err := statement.Send(); err != nil {
		return fmt.Errorf("send backfill batch: %w", err)
	}
	return nil
}

// EventBreakdown groups the window's events by one JSON property
// dimension; missing dimensions collapse into the empty name.
func (c *Client) EventBreakdown(ctx context.Context, dimension string, days int) ([]model.NameCount, error) {
	rows, err := c.conn.Query(ctx,
		"SELECT JSONExtractString(properties, ?) AS dim, toInt64(count()) "+
			"FROM analytics_events WHERE occurred_at >= now() - toIntervalDay(?) "+
			"GROUP BY dim HAVING dim != '' ORDER BY count() DESC LIMIT 10", dimension, days)
	if err != nil {
		return nil, fmt.Errorf("query breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.NameCount, 0, 10)
	for rows.Next() {
		var row model.NameCount
		if err := rows.Scan(&row.Name, &row.Count); err != nil {
			return nil, fmt.Errorf("scan breakdown: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// DailyActiveUsers counts distinct event-attributed users per day.
func (c *Client) DailyActiveUsers(ctx context.Context, days int) ([]model.DayCount, error) {
	rows, err := c.conn.Query(ctx,
		"SELECT toString(toDate(occurred_at)) AS day, toInt64(uniqExact(user_id)) "+
			"FROM analytics_events WHERE occurred_at >= now() - toIntervalDay(?) AND user_id != '' "+
			"GROUP BY day ORDER BY day", days)
	if err != nil {
		return nil, fmt.Errorf("query dau: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.DayCount, 0, days)
	for rows.Next() {
		var row model.DayCount
		if err := rows.Scan(&row.Day, &row.Count); err != nil {
			return nil, fmt.Errorf("scan dau: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// DeleteDemoEvents removes seed-generated events; ClickHouse deletions are
// asynchronous mutations, which is acceptable for a demo reset.
func (c *Client) DeleteDemoEvents(ctx context.Context) error {
	return c.conn.Exec(ctx, "ALTER TABLE analytics_events DELETE WHERE user_id LIKE 'demo-%'")
}

// EventBuffer batches events in memory and flushes them as a single insert
// when the batch fills or the interval elapses. Close flushes what remains;
// drops are logged, never propagated to the caller.
type EventBuffer struct {
	mu     sync.Mutex
	events []eventRow
	client *Client
	logger *slog.Logger
	size   int
	every  time.Duration
	done   chan struct{}
	closed sync.Once
}

type eventRow struct {
	name      string
	occurred  time.Time
	requestID string
	userID    string
}

// NewEventBuffer starts the background flush loop.
func (c *Client) NewEventBuffer(logger *slog.Logger, size int, every time.Duration) *EventBuffer {
	b := &EventBuffer{
		client: c,
		logger: logger,
		size:   size,
		every:  every,
		done:   make(chan struct{}),
	}
	go b.loop()
	return b
}

func (b *EventBuffer) loop() {
	ticker := time.NewTicker(b.every)
	defer ticker.Stop()
	for {
		select {
		case <-b.done:
			return
		case <-ticker.C:
			b.flush()
		}
	}
}

// AddEvent buffers an event, flushing synchronously when the batch is full.
// It never blocks on the warehouse longer than the insert itself and never
// fails the caller: analytics must not take the request path down.
func (b *EventBuffer) AddEvent(_ context.Context, name, userID string) {
	b.mu.Lock()
	b.events = append(b.events, eventRow{name: name, occurred: time.Now(), userID: userID})
	full := len(b.events) >= b.size
	b.mu.Unlock()
	if full {
		b.flush()
	}
}

func (b *EventBuffer) flush() {
	b.mu.Lock()
	if len(b.events) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.events
	b.events = nil
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	statement, err := b.client.conn.PrepareBatch(ctx, "INSERT INTO analytics_events (occurred_at, event_name, user_id, request_id, properties)")
	if err != nil {
		b.logger.Error("clickhouse prepare batch failed", "dropped", len(batch), "err", err)
		return
	}
	for _, e := range batch {
		if err := statement.Append(e.occurred, e.name, e.userID, e.requestID, "{}"); err != nil {
			b.logger.Error("clickhouse append failed", "dropped", len(batch), "err", err)
			return
		}
	}
	if err := statement.Send(); err != nil {
		b.logger.Error("clickhouse send failed", "dropped", len(batch), "err", err)
		return
	}
	b.logger.Debug("events flushed", "count", len(batch))
}

// Close stops the loop and flushes buffered events; safe to call once.
func (b *EventBuffer) Close() error {
	b.closed.Do(func() {
		close(b.done)
		b.flush()
	})
	return nil
}
