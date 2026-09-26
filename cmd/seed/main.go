// Command seed loads synthetic data into PostgreSQL for performance
// testing. It requires APP_DATABASE_URL and refuses to run without one.
//
//	make seed-users USERS=100000   (or: go run ./cmd/seed -users 100000)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/go-template/internal/config"
)

func main() {
	users := flag.Int("users", 100000, "number of users to insert")
	flag.Parse()

	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		logger.Error("seed requires APP_DATABASE_URL (in-memory mode has nothing to seed)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	started := time.Now()
	inserted, err := seedUsers(ctx, pool, *users)
	if err != nil {
		logger.Error("seed users", "err", err)
		os.Exit(1)
	}
	logger.Info("seed complete", "users", inserted, "duration", time.Since(started).Round(time.Millisecond))
}

// seedUsers bulk-inserts users with pgx COPY. Timestamps spread over the
// past months so keyset pagination pages through real variety.
func seedUsers(ctx context.Context, pool *pgxpool.Pool, count int) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows := make([][]any, 0, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		createdAt := now.Add(-time.Duration(i) * time.Second)
		rows = append(rows, []any{
			"perf-" + strconv.Itoa(i),
			"perf-" + strconv.Itoa(i) + "@example.com",
			"Perf User " + strconv.Itoa(i),
			createdAt,
			createdAt,
		})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"users"}, []string{"id", "email", "name", "created_at", "updated_at"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return 0, fmt.Errorf("copy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}
