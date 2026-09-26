//go:build integration

// The store contract suite proves the in-memory fake and the PostgreSQL
// adapter behave identically; run it with `make test-integration`.
package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// storeContract is the union of the service ports under test.
type storeContract interface {
	service.AuthRepo
	service.UsersRepo
	service.JobsRepo
	service.ReportsRepo
	service.AnalyticsRepo
}

func TestStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStoreContract(t, repository.NewMemoryStore())
	})

	t.Run("postgres", func(t *testing.T) {
		ctx := context.Background()
		container, err := tcpostgres.Run(ctx,
			"postgres:16",
			tcpostgres.WithDatabase("app"),
			tcpostgres.WithUsername("app"),
			tcpostgres.WithPassword("app"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("connection string: %v", err)
		}
		store, err := postgres.Open(ctx, dsn, os.DirFS("../../migrations/postgres"))
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		t.Cleanup(store.Close)
		runStoreContract(t, store)
	})
}

// runStoreContract exercises every behavior both adapters must share.
func runStoreContract(t *testing.T, store storeContract) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	user := func(id string, offsetHours int) model.User {
		at := base.Add(time.Duration(offsetHours) * time.Hour)
		return model.User{ID: id, Email: id + "@example.com", Name: "name-" + id, CreatedAt: at, UpdatedAt: at}
	}

	t.Run("user lifecycle and conflicts", func(t *testing.T) {
		for i := 1; i <= 5; i++ {
			id := "u" + string(rune('0'+i))
			if err := store.CreateUser(ctx, user(id, i), ""); err != nil {
				t.Fatalf("create %s: %v", id, err)
			}
		}
		if err := store.CreateUser(ctx, user("U1", 9), ""); !errors.Is(err, repository.ErrConflict) {
			t.Errorf("duplicate email: want ErrConflict, got %v", err)
		}
		if _, err := store.GetUser(ctx, "u1"); err != nil {
			t.Errorf("get u1: %v", err)
		}
		if _, _, err := store.GetUserByEmail(ctx, "U3@EXAMPLE.COM"); err != nil {
			t.Errorf("get by email is case insensitive: %v", err)
		}
		if _, _, err := store.GetUserByEmail(ctx, "ghost@example.com"); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("missing email: want ErrNotFound, got %v", err)
		}
		renamed := user("u2", 2)
		renamed.Name = "renamed"
		if err := store.UpdateUser(ctx, renamed); err != nil {
			t.Errorf("update: %v", err)
		}
		if err := store.UpdateUser(ctx, user("ghost", 0)); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("update missing: want ErrNotFound, got %v", err)
		}
		if err := store.DeleteUser(ctx, "u5"); err != nil {
			t.Errorf("delete: %v", err)
		}
		if err := store.DeleteUser(ctx, "u5"); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("delete missing: want ErrNotFound, got %v", err)
		}
		if store.CountUsers(ctx) != 4 {
			t.Errorf("count users: want 4, got %d", store.CountUsers(ctx))
		}
	})

	t.Run("pagination modes agree", func(t *testing.T) {
		cases := []struct {
			name     string
			opts     repository.ListUsersOptions
			wantIDs  []string
			wantNext string
		}{
			{name: "offset", opts: repository.ListUsersOptions{Mode: "offset", Limit: 2}, wantIDs: []string{"u4", "u3"}, wantNext: "u3"},
			{name: "offset tail", opts: repository.ListUsersOptions{Mode: "offset", Limit: 2, Offset: 2}, wantIDs: []string{"u2", "u1"}},
			{name: "cursor", opts: repository.ListUsersOptions{Mode: "cursor", Limit: 2, After: "u3"}, wantIDs: []string{"u2", "u1"}},
			{name: "keyset", opts: repository.ListUsersOptions{Mode: "keyset", Limit: 2, After: "u3"}, wantIDs: []string{"u2", "u1"}},
			{name: "query filter", opts: repository.ListUsersOptions{Query: "u2"}, wantIDs: []string{"u2"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				users, next, err := store.ListUsers(ctx, tc.opts)
				if err != nil {
					t.Fatalf("list: %v", err)
				}
				if len(users) != len(tc.wantIDs) {
					t.Fatalf("got %d users %v, want %v", len(users), ids(users), tc.wantIDs)
				}
				for i, want := range tc.wantIDs {
					if users[i].ID != want {
						t.Errorf("users[%d]: want %s, got %s", i, want, users[i].ID)
					}
				}
				if next != tc.wantNext {
					t.Errorf("next: want %q, got %q", tc.wantNext, next)
				}
			})
		}
	})

	t.Run("identities", func(t *testing.T) {
		identity := model.AuthIdentity{UserID: "u1", Provider: "google", ProviderSubject: "sub-1", Email: "u1@example.com"}
		if err := store.LinkIdentity(ctx, identity); err != nil {
			t.Fatalf("link: %v", err)
		}
		if err := store.LinkIdentity(ctx, identity); err != nil {
			t.Errorf("re-link same user: %v", err)
		}
		other := identity
		other.UserID = "u2"
		if err := store.LinkIdentity(ctx, other); !errors.Is(err, repository.ErrConflict) {
			t.Errorf("link to another user: want ErrConflict, got %v", err)
		}
		if _, err := store.GetIdentity(ctx, "google", "sub-1"); err != nil {
			t.Errorf("get identity: %v", err)
		}
		listed, err := store.ListIdentities(ctx, "u1")
		if err != nil || len(listed) != 1 {
			t.Fatalf("list identities: n=%d err=%v", len(listed), err)
		}
		if err := store.UnlinkIdentity(ctx, "u1", "google"); err != nil {
			t.Errorf("unlink: %v", err)
		}
		if listed, _ = store.ListIdentities(ctx, "u1"); len(listed) != 0 {
			t.Errorf("after unlink: want 0, got %d", len(listed))
		}
	})

	t.Run("sessions with ttl and sliding", func(t *testing.T) {
		if err := store.CreateSession(ctx, "tok-1", "u1", time.Hour); err != nil {
			t.Fatalf("create session: %v", err)
		}
		if got, err := store.GetSession(ctx, "tok-1", time.Hour); err != nil || got != "u1" {
			t.Errorf("get session: user=%q err=%v", got, err)
		}
		if err := store.DeleteSession(ctx, "tok-1"); err != nil {
			t.Errorf("delete session: %v", err)
		}
		if _, err := store.GetSession(ctx, "tok-1", time.Hour); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("deleted session: want ErrNotFound, got %v", err)
		}

		// Expired sessions vanish even before an explicit delete.
		if err := store.CreateSession(ctx, "tok-2", "u1", -time.Minute); err != nil {
			t.Fatalf("create expired session: %v", err)
		}
		if _, err := store.GetSession(ctx, "tok-2", time.Hour); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("expired session: want ErrNotFound, got %v", err)
		}

		// Logout-all revokes every session of the user.
		for _, tok := range []string{"tok-3", "tok-4"} {
			if err := store.CreateSession(ctx, tok, "u2", time.Hour); err != nil {
				t.Fatalf("create %s: %v", tok, err)
			}
		}
		if err := store.DeleteSessionsForUser(ctx, "u2"); err != nil {
			t.Fatalf("logout all: %v", err)
		}
		for _, tok := range []string{"tok-3", "tok-4"} {
			if _, err := store.GetSession(ctx, tok, time.Hour); !errors.Is(err, repository.ErrNotFound) {
				t.Errorf("%s after logout-all: want ErrNotFound, got %v", tok, err)
			}
		}
	})

	t.Run("oauth state single use and expiry", func(t *testing.T) {
		now := time.Now()
		if err := store.SaveOAuthState(ctx, model.OAuthState{State: "fresh", Provider: "google", ExpiresAt: now.Add(time.Minute)}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := store.SaveOAuthState(ctx, model.OAuthState{State: "stale", Provider: "google", ExpiresAt: now.Add(-time.Minute)}); err != nil {
			t.Fatalf("save expired: %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "fresh", now); err != nil {
			t.Errorf("consume fresh: %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "fresh", now); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("state is single use: want ErrNotFound, got %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "stale", now); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("expired state: want ErrNotFound, got %v", err)
		}
	})

	t.Run("jobs reports enrichments analytics", func(t *testing.T) {
		now := time.Now()
		jobs := []model.Job{
			{ID: "j1", Type: "user.enrich_batch", Status: "queued", Payload: "{}", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
			{ID: "j2", Type: "user.enrich_batch", Status: "done", Payload: "{}", CreatedAt: now.Add(1 * time.Second), UpdatedAt: now},
			{ID: "j3", Type: "user.enrich_batch", Status: "failed", Payload: "{}", Error: "boom", CreatedAt: now, UpdatedAt: now},
		}
		for _, job := range jobs {
			if err := store.SaveJob(ctx, job); err != nil {
				t.Fatalf("save job %s: %v", job.ID, err)
			}
		}
		if err := store.SaveReport(ctx, model.Report{ID: "r1", Status: "done", Result: "users=4", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("save report: %v", err)
		}
		if err := store.AddEnrichment(ctx, model.EnrichmentProfile{UserID: "u1", Source: "randomuser.me", Payload: "{}", CreatedAt: now}); err != nil {
			t.Fatalf("add enrichment: %v", err)
		}
		store.AddEvent(ctx, "job_queued")

		listed, err := store.ListJobs(ctx)
		if err != nil || len(listed) != 3 {
			t.Fatalf("list jobs: n=%d err=%v", len(listed), err)
		}
		if listed[0].ID != "j1" {
			t.Errorf("jobs are newest first: want j1, got %s", listed[0].ID)
		}
		reports, err := store.ListReports(ctx)
		if err != nil || len(reports) != 1 || reports[0].ID != "r1" {
			t.Fatalf("list reports: %+v err=%v", reports, err)
		}

		summary := store.Analytics(ctx)
		if summary.TotalUsers != 4 {
			t.Errorf("TotalUsers: want 4, got %d", summary.TotalUsers)
		}
		if summary.JobsQueued != 1 || summary.JobsDone != 1 || summary.JobsFailed != 1 {
			t.Errorf("jobs: queued=%d done=%d failed=%d", summary.JobsQueued, summary.JobsDone, summary.JobsFailed)
		}
		if summary.EnrichmentSucceeded != 1 {
			t.Errorf("EnrichmentSucceeded: want 1, got %d", summary.EnrichmentSucceeded)
		}
	})
}

func ids(users []model.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.ID)
	}
	return out
}
