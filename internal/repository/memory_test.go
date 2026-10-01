package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

func testUser(id, email string, createdAt time.Time) users.User {
	return users.User{ID: id, Email: email, Name: "name-" + id, CreatedAt: createdAt, UpdatedAt: createdAt}
}

// seedUsers stores five users with hourly timestamps; u1 is the oldest, so
// listing order (newest first) is u5..u1.
func seedUsers(t *testing.T, s *MemoryStore) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		id := "u" + string(rune('0'+i))
		if err := s.CreateUser(ctx, testUser(id, id+"@example.com", base.Add(time.Duration(i)*time.Hour)), ""); err != nil {
			t.Fatalf("seed user %s: %v", id, err)
		}
	}
}

func TestCreateUserRejectsDuplicateEmailCaseInsensitively(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.CreateUser(ctx, testUser("u1", "ada@example.com", time.Now()), "hash"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	for _, email := range []string{"ada@example.com", "ADA@EXAMPLE.COM", "Ada@Example.com"} {
		err := store.CreateUser(ctx, testUser("u2", email, time.Now()), "")
		if !errors.Is(err, kernel.ErrConflict) {
			t.Errorf("create with email %q: want kernel.ErrConflict, got %v", email, err)
		}
	}
}

func TestUserLifecycle(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.CreateUser(ctx, testUser("u1", "ada@example.com", time.Now()), "hash"); err != nil {
		t.Fatalf("create: %v", err)
	}

	stored, hash, err := store.GetUserByEmail(ctx, "ADA@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if hash != "hash" {
		t.Errorf("password hash: want %q, got %q", "hash", hash)
	}
	if stored.ID != "u1" {
		t.Errorf("user id: want u1, got %q", stored.ID)
	}

	if _, _, err := store.GetUserByEmail(ctx, "missing@example.com"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("missing email: want kernel.ErrNotFound, got %v", err)
	}
	if _, err := store.GetUser(ctx, "nope"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("missing id: want kernel.ErrNotFound, got %v", err)
	}

	stored.Name = "renamed"
	if err := store.UpdateUser(ctx, stored); err != nil {
		t.Fatalf("update: %v", err)
	}
	ghost := testUser("ghost", "ghost@example.com", time.Now())
	if err := store.UpdateUser(ctx, ghost); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("update missing: want kernel.ErrNotFound, got %v", err)
	}

	if err := store.DeleteUser(ctx, "u1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.DeleteUser(ctx, "u1"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("delete missing: want kernel.ErrNotFound, got %v", err)
	}
}

func TestListUsersPagination(t *testing.T) {
	store := NewMemoryStore()
	seedUsers(t, store)
	ctx := context.Background()

	cases := []struct {
		name     string
		opts     kernel.ListOptions
		wantIDs  []string
		wantNext string
		wantNil  bool
	}{
		{
			name:    "offset default limit returns all",
			opts:    kernel.ListOptions{Mode: "offset"},
			wantIDs: []string{"u5", "u4", "u3", "u2", "u1"},
		},
		{
			name:     "offset page with next cursor",
			opts:     kernel.ListOptions{Mode: "offset", Limit: 2},
			wantIDs:  []string{"u5", "u4"},
			wantNext: "u4",
		},
		{
			name:    "offset tail page has no next cursor",
			opts:    kernel.ListOptions{Mode: "offset", Limit: 2, Offset: 4},
			wantIDs: []string{"u1"},
		},
		{
			name:    "offset beyond result set",
			opts:    kernel.ListOptions{Mode: "offset", Offset: 10},
			wantNil: true,
		},
		{
			name:     "cursor continues after id",
			opts:     kernel.ListOptions{Mode: "cursor", Limit: 2, After: "u4"},
			wantIDs:  []string{"u3", "u2"},
			wantNext: "u2",
		},
		{
			name:     "keyset continues after id",
			opts:     kernel.ListOptions{Mode: "keyset", Limit: 2, After: "u4"},
			wantIDs:  []string{"u3", "u2"},
			wantNext: "u2",
		},
		{
			name:    "limit is clamped to the default when out of range",
			opts:    kernel.ListOptions{Mode: "offset", Limit: 1000},
			wantIDs: []string{"u5", "u4", "u3", "u2", "u1"},
		},
		{
			name:    "query filters by email and name",
			opts:    kernel.ListOptions{Query: "u4"},
			wantIDs: []string{"u4"},
		},
		{
			name:    "query is case insensitive",
			opts:    kernel.ListOptions{Query: "NAME-U3"},
			wantIDs: []string{"u3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users, next, err := store.ListUsers(ctx, tc.opts)
			if err != nil {
				t.Fatalf("list users: %v", err)
			}
			if tc.wantNil {
				if users != nil {
					t.Fatalf("want nil users, got %d", len(users))
				}
				return
			}
			if len(users) != len(tc.wantIDs) {
				t.Fatalf("got %d users, want %v", len(users), tc.wantIDs)
			}
			for i, want := range tc.wantIDs {
				if users[i].ID != want {
					t.Errorf("users[%d]: want %s, got %s", i, want, users[i].ID)
				}
			}
			if next != tc.wantNext {
				t.Errorf("next cursor: want %q, got %q", tc.wantNext, next)
			}
		})
	}
}

func TestIdentities(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	identity := auth.Identity{UserID: "u1", Provider: "google", ProviderSubject: "sub-1", Email: "ada@example.com"}

	if err := store.LinkIdentity(ctx, identity); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := store.LinkIdentity(ctx, identity); err != nil {
		t.Fatalf("re-link same user is idempotent: %v", err)
	}

	hijack := identity
	hijack.UserID = "u2"
	if err := store.LinkIdentity(ctx, hijack); !errors.Is(err, kernel.ErrConflict) {
		t.Errorf("link same subject to another user: want kernel.ErrConflict, got %v", err)
	}

	got, err := store.GetIdentity(ctx, "google", "sub-1")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if got.UserID != "u1" {
		t.Errorf("identity owner: want u1, got %s", got.UserID)
	}
	if _, err := store.GetIdentity(ctx, "google", "missing"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("missing identity: want kernel.ErrNotFound, got %v", err)
	}

	if err := store.LinkIdentity(ctx, auth.Identity{UserID: "u2", Provider: "github", ProviderSubject: "gh-1"}); err != nil {
		t.Fatalf("link github: %v", err)
	}
	identities, err := store.ListIdentities(ctx, "u2")
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != "github" {
		t.Fatalf("identities for u2: want only github, got %+v", identities)
	}

	if err := store.UnlinkIdentity(ctx, "u2", "github"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	identities, _ = store.ListIdentities(ctx, "u2")
	if len(identities) != 0 {
		t.Errorf("identities after unlink: want 0, got %d", len(identities))
	}
}

func TestOAuthStateExpiryAndSingleUse(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	if err := store.SaveOAuthState(ctx, auth.OAuthState{State: "s1", Provider: "google", UserID: "u1", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := store.SaveOAuthState(ctx, auth.OAuthState{State: "expired", Provider: "google", ExpiresAt: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("save expired state: %v", err)
	}

	if _, err := store.ConsumeOAuthState(ctx, "s1", now); err != nil {
		t.Fatalf("consume fresh state: %v", err)
	}
	if _, err := store.ConsumeOAuthState(ctx, "s1", now); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("state is single use: want kernel.ErrNotFound, got %v", err)
	}
	if _, err := store.ConsumeOAuthState(ctx, "expired", now); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("consume expired state: want kernel.ErrNotFound, got %v", err)
	}
	if _, err := store.ConsumeOAuthState(ctx, "unknown", now); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("consume unknown state: want kernel.ErrNotFound, got %v", err)
	}
}

func TestSessions(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.CreateSession(ctx, "tok", "u1", "", time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	session, err := store.GetSession(ctx, "tok", time.Hour)
	if err != nil || session.UserID != "u1" {
		t.Fatalf("get session: session=%+v err=%v", session, err)
	}
	if err := store.DeleteSession(ctx, "tok"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := store.GetSession(ctx, "tok", time.Hour); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("deleted session: want kernel.ErrNotFound, got %v", err)
	}
}

func TestJobsListIsNewestFirst(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i, id := range []string{"j1", "j2", "j3"} {
		job := jobs.Job{ID: id, Type: "user.enrich_batch", Status: "queued", CreatedAt: base.Add(time.Duration(i) * time.Minute), UpdatedAt: base}
		if err := store.SaveJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", id, err)
		}
	}

	jobs, _, err := store.ListJobs(ctx, kernel.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	wantOrder := []string{"j3", "j2", "j1"}
	for i, want := range wantOrder {
		if jobs[i].ID != want {
			t.Errorf("jobs[%d]: want %s, got %s", i, want, jobs[i].ID)
		}
	}
}

func TestAnalyticsSummary(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.CreateUser(ctx, testUser("u1", "ada@example.com", time.Now()), "hash"); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := store.CreateUser(ctx, testUser("u2", "bob@example.com", time.Now()), "hash"); err != nil {
		t.Fatalf("create u2: %v", err)
	}
	if err := store.LinkIdentity(ctx, auth.Identity{UserID: "u1", Provider: "google", ProviderSubject: "google:ada@example.com"}); err != nil {
		t.Fatalf("link google: %v", err)
	}
	if err := store.LinkIdentity(ctx, auth.Identity{UserID: "u1", Provider: "github", ProviderSubject: "github:ada@example.com"}); err != nil {
		t.Fatalf("link github: %v", err)
	}
	now := time.Now()
	seeded := []jobs.Job{
		{ID: "j1", Status: "queued", CreatedAt: now, UpdatedAt: now},
		{ID: "j2", Status: "done", CreatedAt: now, UpdatedAt: now},
		{ID: "j3", Status: "failed", CreatedAt: now, UpdatedAt: now},
	}
	for _, job := range seeded {
		if err := store.SaveJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", job.ID, err)
		}
	}
	if err := store.AddEnrichment(ctx, jobs.EnrichmentProfile{UserID: "u1", Source: "randomuser.me"}); err != nil {
		t.Fatalf("add enrichment: %v", err)
	}

	summary := store.Analytics(ctx)
	if summary.TotalUsers != 2 {
		t.Errorf("TotalUsers: want 2, got %d", summary.TotalUsers)
	}
	if summary.PasswordIdentities != 2 {
		t.Errorf("PasswordIdentities: want 2, got %d", summary.PasswordIdentities)
	}
	if summary.GoogleIdentities != 1 || summary.GitHubIdentities != 1 {
		t.Errorf("provider identities: google=%d github=%d, want 1/1", summary.GoogleIdentities, summary.GitHubIdentities)
	}
	if summary.JobsQueued != 1 || summary.JobsDone != 1 || summary.JobsFailed != 1 {
		t.Errorf("job counts: queued=%d done=%d failed=%d, want 1/1/1", summary.JobsQueued, summary.JobsDone, summary.JobsFailed)
	}
	if summary.EnrichmentSucceeded != 1 {
		t.Errorf("EnrichmentSucceeded: want 1, got %d", summary.EnrichmentSucceeded)
	}
}
