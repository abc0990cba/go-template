// Command seed loads synthetic data into PostgreSQL for performance
// testing. It requires APP_DATABASE_URL and refuses to run without one.
//
//	make seed-users USERS=100000   (or: go run ./cmd/seed -users 100000)
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"

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
	credentials, err := seedUserCredentials(ctx, pool, *users)
	if err != nil {
		logger.Error("seed credentials", "err", err)
		os.Exit(1)
	}
	logger.Info("seed complete", "users", inserted, "password_credentials", credentials, "duration", time.Since(started).Round(time.Millisecond))
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

// seedUserCredentials gives every perf user the password "password123" and
// the matching "password" identity so the k6 login scenarios authenticate
// against a freshly seeded database. The hash is computed once and shared:
// Argon2id costs ~20 ms per hash, which would dominate a 100k seed.
func seedUserCredentials(ctx context.Context, pool *pgxpool.Pool, count int) (int64, error) {
	hash := hashPassword("password123")
	const batch = 1000
	var total int64
	now := time.Now()
	for start := 0; start < count; start += batch {
		end := min(start+batch, count)
		credentials := make([][]any, 0, end-start)
		identities := make([][]any, 0, end-start)
		for i := start; i < end; i++ {
			email := "perf-" + strconv.Itoa(i) + "@example.com"
			credentials = append(credentials, []any{"perf-" + strconv.Itoa(i), hash, now})
			identities = append(identities, []any{"perf-" + strconv.Itoa(i), "password", email, "Perf User " + strconv.Itoa(i), email})
		}
		n1, err := pool.CopyFrom(ctx, pgx.Identifier{"password_credentials"}, []string{"user_id", "password_hash", "updated_at"},
			pgx.CopyFromRows(credentials))
		if err != nil {
			return total, fmt.Errorf("copy credentials: %w", err)
		}
		n2, err := pool.CopyFrom(ctx, pgx.Identifier{"auth_identities"}, []string{"user_id", "provider", "provider_subject", "name", "email"},
			pgx.CopyFromRows(identities))
		if err != nil {
			return total, fmt.Errorf("copy identities: %w", err)
		}
		total += n1 + n2
	}
	return total, nil
}

// hashPassword mirrors internal/auth (passwords.go) so seeded credentials verify
// against the real login path.
func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(key)
}
