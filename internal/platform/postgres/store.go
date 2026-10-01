// Package postgres implements the service store ports on PostgreSQL: pgx
// connection pool, sqlc-generated queries, and schema migrations run at
// startup.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/golang-migrate/migrate/v4"
	// Blank import registers the "pgx5" database driver for golang-migrate;
	// the scheme never appears in our code by name.
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

var (
	_ auth.Store            = (*Store)(nil)
	_ users.Store           = (*Store)(nil)
	_ jobs.Store            = (*Store)(nil)
	_ analytics.ReportStore = (*Store)(nil)
	_ tickets.Store         = (*Store)(nil)

	_ streams.Store          = (*Store)(nil)
	_ kernel.IdempotencyRepo = (*Store)(nil)
)

// Store is a PostgreSQL-backed implementation of every service port.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// Option tunes the store connection.
type Option func(*poolConfig)

type poolConfig struct {
	connTracer pgx.QueryTracer
	tuning     Tuning
}

// Tuning carries the pool knobs; zero values keep the driver defaults.
// StatementTimeout and IdleTxTimeout are enforced server-side via
// statement_timeout / idle_in_transaction_session_timeout runtime params.
type Tuning struct {
	MaxConns          int32
	MinConns          int32
	ConnMaxLifetime   time.Duration
	ConnMaxIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	StatementTimeout  time.Duration
	IdleTxTimeout     time.Duration
}

// WithQueryTracing instruments every query with OpenTelemetry spans
// (otelpgx); enable only after the global tracer provider is installed.
func WithQueryTracing() Option {
	return func(c *poolConfig) { c.connTracer = otelpgx.NewTracer() }
}

// WithTuning applies pool sizing and server-side statement timeouts.
func WithTuning(t Tuning) Option {
	return func(c *poolConfig) { c.tuning = t }
}

// Open connects the pool, verifies connectivity, and applies pending
// migrations from the given directory (an fs.FS rooted at migrations/postgres).
func Open(ctx context.Context, url string, migrations fs.FS, opts ...Option) (*Store, error) {
	cfg := poolConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres parse config: %w", err)
	}
	if cfg.connTracer != nil {
		poolCfg.ConnConfig.Tracer = cfg.connTracer
	}
	applyTuning(poolCfg, cfg.tuning)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	if err := migrateUp(url, migrations); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// Stats exposes the pool statistics snapshot for the Prometheus collector;
// nil after Close.
func (s *Store) Stats() *pgxpool.Stat { return s.pool.Stat() }

// applyTuning overlays the optional knobs on the parsed pool config; unset
// fields keep whatever the connection URL (or the driver default) chose.
func applyTuning(poolCfg *pgxpool.Config, t Tuning) {
	if t.MaxConns > 0 {
		poolCfg.MaxConns = t.MaxConns
	}
	if t.MinConns > 0 {
		poolCfg.MinConns = t.MinConns
	}
	if t.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = t.ConnMaxLifetime
	}
	if t.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = t.ConnMaxIdleTime
	}
	if t.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = t.HealthCheckPeriod
	}
	if t.StatementTimeout > 0 {
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(t.StatementTimeout.Milliseconds(), 10)
	}
	if t.IdleTxTimeout > 0 {
		poolCfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(t.IdleTxTimeout.Milliseconds(), 10)
	}
}

// Ping reports database readiness for the health endpoints.
func (s *Store) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.pool.Ping(pingCtx)
}

func migrateUp(url string, fsys fs.FS) error {
	source, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	driverURL := "pgx5://" + strings.TrimPrefix(url, "postgres://")
	m, err := migrate.NewWithSourceInstance("iofs", source, driverURL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// mapErr converts driver errors to the domain sentinels the HTTP layer maps
// to status codes.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", kernel.ErrNotFound, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", kernel.ErrConflict, err)
	}
	return fmt.Errorf("postgres: %w", err)
}

func (s *Store) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return mapErr(tx.Commit(ctx))
}

func userFromRow(row db.User) users.User {
	return userFromColumns(row.ID, row.Email, row.Name, row.ExternalRef,
		row.EnrichProfile, row.EnrichContact, row.EnrichedAt, row.CreatedAt, row.UpdatedAt)
}

// userFromColumns assembles the domain user from the shared column set every
// user-reading query returns; the generated row structs feed it.
func userFromColumns(id, email, name string, externalRef pgtype.Text,
	enrichProfile, enrichContact string, enrichedAt pgtype.Timestamptz, createdAt, updatedAt time.Time,
) users.User {
	var at *time.Time
	if enrichedAt.Valid {
		t := enrichedAt.Time
		at = &t
	}
	return users.User{
		ID: id, Email: email, Name: name,
		ExternalRef:   externalRef.String,
		EnrichProfile: enrichProfile,
		EnrichContact: enrichContact,
		EnrichedAt:    at,
		CreatedAt:     createdAt, UpdatedAt: updatedAt,
	}
}

// textOrNull maps "" to SQL NULL for nullable text columns.
func textOrNull(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

// timeFromPtr converts a nullable timestamptz back to a *time.Time.
func timeFromPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
