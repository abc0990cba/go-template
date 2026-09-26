// Package postgres implements the service store ports on PostgreSQL: pgx
// connection pool, sqlc-generated queries, and schema migrations run at
// startup.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

var (
	_ service.AuthRepo      = (*Store)(nil)
	_ service.UsersRepo     = (*Store)(nil)
	_ service.JobsRepo      = (*Store)(nil)
	_ service.ReportsRepo   = (*Store)(nil)
	_ service.AnalyticsRepo = (*Store)(nil)
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
}

// WithQueryTracing instruments every query with OpenTelemetry spans
// (otelpgx); enable only after the global tracer provider is installed.
func WithQueryTracing() Option {
	return func(c *poolConfig) { c.connTracer = otelpgx.NewTracer() }
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
		return fmt.Errorf("%w: %s", repository.ErrNotFound, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", repository.ErrConflict, err)
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

// CreateUser writes the user and, with a hash, its credential and password identity in one transaction.
func (s *Store) CreateUser(ctx context.Context, user model.User, passwordHash string) error {
	params := db.CreateUserParams{ID: user.ID, Email: user.Email, Name: user.Name, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
	if passwordHash == "" {
		_, err := s.q.CreateUser(ctx, params)
		return mapErr(err)
	}
	// A user with a password brings its credential and the "password"
	// identity in the same transaction, mirroring the memory fake.
	return s.tx(ctx, func(q *db.Queries) error {
		created, err := q.CreateUser(ctx, params)
		if err != nil {
			return err
		}
		if err := q.UpsertPasswordCredential(ctx, db.UpsertPasswordCredentialParams{UserID: created.ID, PasswordHash: passwordHash}); err != nil {
			return err
		}
		return q.InsertIdentity(ctx, db.InsertIdentityParams{
			Provider:        "password",
			ProviderSubject: strings.ToLower(user.Email),
			UserID:          user.ID,
			Email:           user.Email,
			Name:            user.Name,
		})
	})
}

// UpdateUser rewrites email and name and refreshes updated_at.
func (s *Store) UpdateUser(ctx context.Context, user model.User) error {
	_, err := s.q.UpdateUser(ctx, db.UpdateUserParams{ID: user.ID, Email: user.Email, Name: user.Name})
	return mapErr(err)
}

// DeleteUser removes the user and reports ErrNotFound when absent.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	n, err := s.q.DeleteUser(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// GetUser returns the user by id.
func (s *Store) GetUser(ctx context.Context, id string) (model.User, error) {
	row, err := s.q.GetUser(ctx, id)
	if err != nil {
		return model.User{}, mapErr(err)
	}
	return userFromRow(row), nil
}

// GetUserByEmail matches case-insensitively and returns the password hash.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return model.User{}, "", mapErr(err)
	}
	return userFromRow(db.User{
		ID: row.ID, Email: row.Email, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), row.PasswordHash, nil
}

// ListUsers pages newest first; cursor/keyset continue after After, and the returned cursor is empty on the last page.
func (s *Store) ListUsers(ctx context.Context, opts repository.ListUsersOptions) ([]model.User, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	var (
		rows []db.User
		err  error
	)
	if (opts.Mode == "cursor" || opts.Mode == "keyset") && opts.After != "" {
		rows, err = s.q.ListUsersKeyset(ctx, db.ListUsersKeysetParams{Query: opts.Query, After: opts.After, Lim: limit + 1}) // #nosec G115 -- limit is clamped to 1..100
	} else {
		off := int32(0)
		if opts.Offset > 0 {
			off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
		}
		rows, err = s.q.ListUsersOffset(ctx, db.ListUsersOffsetParams{Query: opts.Query, Off: off, Lim: limit + 1})
	}
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	users := make([]model.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromRow(row))
	}
	return users, next, nil
}

func userFromRow(row db.User) model.User {
	return model.User{ID: row.ID, Email: row.Email, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

// CountUsers returns the number of users, 0 when the query fails.
func (s *Store) CountUsers(ctx context.Context) int {
	n, err := s.q.CountUsers(ctx)
	if err != nil {
		return 0
	}
	return int(n)
}

// LinkIdentity is idempotent for the same owner and reports ErrConflict when another user owns the identity.
func (s *Store) LinkIdentity(ctx context.Context, identity model.AuthIdentity) error {
	owner, err := s.q.GetIdentityOwner(ctx, db.GetIdentityOwnerParams{Provider: identity.Provider, ProviderSubject: identity.ProviderSubject})
	switch {
	case err == nil:
		if owner == identity.UserID {
			return nil
		}
		return fmt.Errorf("%w: identity owned by another user", repository.ErrConflict)
	case !errors.Is(err, pgx.ErrNoRows):
		return mapErr(err)
	}
	return mapErr(s.q.InsertIdentity(ctx, db.InsertIdentityParams{
		Provider:        identity.Provider,
		ProviderSubject: identity.ProviderSubject,
		UserID:          identity.UserID,
		Email:           identity.Email,
		Name:            identity.Name,
		AvatarUrl:       identity.AvatarURL,
	}))
}

// GetIdentity returns the identity for a (provider, subject) pair.
func (s *Store) GetIdentity(ctx context.Context, provider, subject string) (model.AuthIdentity, error) {
	row, err := s.q.GetIdentity(ctx, db.GetIdentityParams{Provider: provider, ProviderSubject: subject})
	if err != nil {
		return model.AuthIdentity{}, mapErr(err)
	}
	return model.AuthIdentity{
		UserID: row.UserID, Provider: row.Provider, ProviderSubject: row.ProviderSubject,
		Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl,
		LinkedAt: row.LinkedAt, LastLoginAt: row.LastLoginAt,
	}, nil
}

// UnlinkIdentity detaches the provider identity from the user.
func (s *Store) UnlinkIdentity(ctx context.Context, userID, provider string) error {
	return mapErr(s.q.UnlinkIdentity(ctx, db.UnlinkIdentityParams{UserID: userID, Provider: provider}))
}

// ListIdentities lists the user's identities, oldest link first.
func (s *Store) ListIdentities(ctx context.Context, userID string) ([]model.AuthIdentity, error) {
	rows, err := s.q.ListIdentities(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	identities := make([]model.AuthIdentity, 0, len(rows))
	for _, row := range rows {
		identities = append(identities, model.AuthIdentity{
			UserID: row.UserID, Provider: row.Provider, ProviderSubject: row.ProviderSubject,
			Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl,
			LinkedAt: row.LinkedAt, LastLoginAt: row.LastLoginAt,
		})
	}
	return identities, nil
}

// CreateSession stores a token with the default expiry.
func (s *Store) CreateSession(ctx context.Context, token, userID string) error {
	return mapErr(s.q.CreateSession(ctx, db.CreateSessionParams{Token: token, UserID: userID}))
}

// GetSession resolves only unexpired tokens.
func (s *Store) GetSession(ctx context.Context, token string) (string, error) {
	userID, err := s.q.GetSession(ctx, token)
	return userID, mapErr(err)
}

// DeleteSession revokes a token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return mapErr(s.q.DeleteSession(ctx, token))
}

// SaveOAuthState stores a single-use state with its expiry.
func (s *Store) SaveOAuthState(ctx context.Context, state model.OAuthState) error {
	return mapErr(s.q.SaveOAuthState(ctx, db.SaveOAuthStateParams{
		State: state.State, Provider: state.Provider, UserID: state.UserID,
		RedirectAfter: state.RedirectAfter, ExpiresAt: state.ExpiresAt,
	}))
}

// ConsumeOAuthState deletes the state first, then honors expiry: consumed or expired states always yield ErrNotFound.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string, now time.Time) (model.OAuthState, error) {
	row, err := s.q.ConsumeOAuthState(ctx, state)
	if err != nil {
		return model.OAuthState{}, mapErr(err)
	}
	if now.After(row.ExpiresAt) {
		return model.OAuthState{}, repository.ErrNotFound
	}
	return model.OAuthState{
		State: row.State, Provider: row.Provider, UserID: row.UserID,
		RedirectAfter: row.RedirectAfter, ExpiresAt: row.ExpiresAt,
	}, nil
}

// SaveJob upserts the full job record.
func (s *Store) SaveJob(ctx context.Context, job model.Job) error {
	return mapErr(s.q.UpsertJob(ctx, db.UpsertJobParams{
		ID: job.ID, Type: job.Type, Status: job.Status, Column4: job.Payload,
		Error: job.Error, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}))
}

// GetJob returns a job by id.
func (s *Store) GetJob(ctx context.Context, id string) (model.Job, error) {
	row, err := s.q.GetJob(ctx, id)
	if err != nil {
		return model.Job{}, mapErr(err)
	}
	return model.Job{
		ID: row.ID, Type: row.Type, Status: row.Status, Payload: row.Payload,
		Error: row.Error, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// ListJobs returns all jobs newest first.
func (s *Store) ListJobs(ctx context.Context) ([]model.Job, error) {
	rows, err := s.q.ListJobs(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	jobs := make([]model.Job, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, model.Job{
			ID: row.ID, Type: row.Type, Status: row.Status, Payload: row.Payload,
			Error: row.Error, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return jobs, nil
}

// AddEnrichment appends one enrichment profile.
func (s *Store) AddEnrichment(ctx context.Context, profile model.EnrichmentProfile) error {
	return mapErr(s.q.AddEnrichment(ctx, db.AddEnrichmentParams{
		UserID: profile.UserID, Source: profile.Source, Column3: profile.Payload, CreatedAt: profile.CreatedAt,
	}))
}

// SaveReport upserts the report.
func (s *Store) SaveReport(ctx context.Context, report model.Report) error {
	return mapErr(s.q.UpsertReport(ctx, db.UpsertReportParams{
		ID: report.ID, Status: report.Status, Result: report.Result, Error: report.Error,
		CreatedAt: report.CreatedAt, UpdatedAt: report.UpdatedAt,
	}))
}

// ListReports returns all reports newest first.
func (s *Store) ListReports(ctx context.Context) ([]model.Report, error) {
	rows, err := s.q.ListReports(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	reports := make([]model.Report, 0, len(rows))
	for _, row := range rows {
		reports = append(reports, model.Report{
			ID: row.ID, Status: row.Status, Result: row.Result, Error: row.Error,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return reports, nil
}

// AddEvent is a no-op until the analytics pipeline stores events in
// ClickHouse; operational counters do not depend on it.
func (s *Store) AddEvent(_ context.Context, _ string) {}

// Analytics computes dashboard counters with filtered subqueries.
func (s *Store) Analytics(ctx context.Context) model.AnalyticsSummary {
	row, err := s.q.Analytics(ctx)
	if err != nil {
		return model.AnalyticsSummary{}
	}
	return model.AnalyticsSummary{
		TotalUsers:          int(row.TotalUsers),
		PasswordIdentities:  int(row.PasswordIdentities),
		GoogleIdentities:    int(row.GoogleIdentities),
		GitHubIdentities:    int(row.GithubIdentities),
		EnrichmentSucceeded: int(row.EnrichmentSucceeded),
		JobsQueued:          int(row.JobsQueued),
		JobsDone:            int(row.JobsDone),
		JobsFailed:          int(row.JobsFailed),
	}
}
