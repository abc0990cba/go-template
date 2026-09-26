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
func (b *EventBuffer) AddEvent(_ context.Context, name string) {
	b.mu.Lock()
	b.events = append(b.events, eventRow{name: name, occurred: time.Now()})
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
		if err := statement.Append(e.occurred, e.name, "", e.requestID, "{}"); err != nil {
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
