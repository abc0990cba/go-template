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

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

// storeContract is the union of the service ports under test.
type storeContract interface {
	auth.Store
	users.Store
	jobs.Store
	analytics.ReportStore
	tickets.Store
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
	user := func(id string, offsetHours int) users.User {
		at := base.Add(time.Duration(offsetHours) * time.Hour)
		return users.User{ID: id, Email: id + "@example.com", Name: "name-" + id, CreatedAt: at, UpdatedAt: at}
	}

	t.Run("user lifecycle and conflicts", func(t *testing.T) {
		for i := 1; i <= 5; i++ {
			id := "u" + string(rune('0'+i))
			if err := store.CreateUser(ctx, user(id, i), ""); err != nil {
				t.Fatalf("create %s: %v", id, err)
			}
		}
		if err := store.CreateUser(ctx, user("U1", 9), ""); !errors.Is(err, kernel.ErrConflict) {
			t.Errorf("duplicate email: want ErrConflict, got %v", err)
		}
		if _, err := store.GetUser(ctx, "u1"); err != nil {
			t.Errorf("get u1: %v", err)
		}
		if _, _, err := store.GetUserByEmail(ctx, "U3@EXAMPLE.COM"); err != nil {
			t.Errorf("get by email is case insensitive: %v", err)
		}
		if _, _, err := store.GetUserByEmail(ctx, "ghost@example.com"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("missing email: want ErrNotFound, got %v", err)
		}
		renamed := user("u2", 2)
		renamed.Name = "renamed"
		if err := store.UpdateUser(ctx, renamed); err != nil {
			t.Errorf("update: %v", err)
		}
		if err := store.UpdateUser(ctx, user("ghost", 0)); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("update missing: want ErrNotFound, got %v", err)
		}
		if err := store.DeleteUser(ctx, "u5"); err != nil {
			t.Errorf("delete: %v", err)
		}
		if err := store.DeleteUser(ctx, "u5"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("delete missing: want ErrNotFound, got %v", err)
		}
		if store.CountUsers(ctx) != 4 {
			t.Errorf("count users: want 4, got %d", store.CountUsers(ctx))
		}
	})

	t.Run("pagination modes agree", func(t *testing.T) {
		cases := []struct {
			name     string
			opts     kernel.ListOptions
			wantIDs  []string
			wantNext string
		}{
			{name: "offset", opts: kernel.ListOptions{Mode: "offset", Limit: 2}, wantIDs: []string{"u4", "u3"}, wantNext: "u3"},
			{name: "offset tail", opts: kernel.ListOptions{Mode: "offset", Limit: 2, Offset: 2}, wantIDs: []string{"u2", "u1"}},
			{name: "cursor", opts: kernel.ListOptions{Mode: "cursor", Limit: 2, After: "u3"}, wantIDs: []string{"u2", "u1"}},
			{name: "keyset", opts: kernel.ListOptions{Mode: "keyset", Limit: 2, After: "u3"}, wantIDs: []string{"u2", "u1"}},
			{name: "query filter", opts: kernel.ListOptions{Query: "u2"}, wantIDs: []string{"u2"}},
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
		identity := auth.Identity{UserID: "u1", Provider: "google", ProviderSubject: "sub-1", Email: "u1@example.com"}
		if err := store.LinkIdentity(ctx, identity); err != nil {
			t.Fatalf("link: %v", err)
		}
		if err := store.LinkIdentity(ctx, identity); err != nil {
			t.Errorf("re-link same user: %v", err)
		}
		other := identity
		other.UserID = "u2"
		if err := store.LinkIdentity(ctx, other); !errors.Is(err, kernel.ErrConflict) {
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
		if err := store.CreateSession(ctx, "tok-1", "u1", "", time.Hour); err != nil {
			t.Fatalf("create session: %v", err)
		}
		if got, err := store.GetSession(ctx, "tok-1", time.Hour); err != nil || got.UserID != "u1" {
			t.Errorf("get session: user=%q err=%v", got.UserID, err)
		}
		if err := store.DeleteSession(ctx, "tok-1"); err != nil {
			t.Errorf("delete session: %v", err)
		}
		if _, err := store.GetSession(ctx, "tok-1", time.Hour); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("deleted session: want ErrNotFound, got %v", err)
		}

		// Expired sessions vanish even before an explicit delete.
		if err := store.CreateSession(ctx, "tok-2", "u1", "", -time.Minute); err != nil {
			t.Fatalf("create expired session: %v", err)
		}
		if _, err := store.GetSession(ctx, "tok-2", time.Hour); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("expired session: want ErrNotFound, got %v", err)
		}

		// Logout-all revokes every session of the user.
		for _, tok := range []string{"tok-3", "tok-4"} {
			if err := store.CreateSession(ctx, tok, "u2", "", time.Hour); err != nil {
				t.Fatalf("create %s: %v", tok, err)
			}
		}
		if err := store.DeleteSessionsForUser(ctx, "u2"); err != nil {
			t.Fatalf("logout all: %v", err)
		}
		for _, tok := range []string{"tok-3", "tok-4"} {
			if _, err := store.GetSession(ctx, tok, time.Hour); !errors.Is(err, kernel.ErrNotFound) {
				t.Errorf("%s after logout-all: want ErrNotFound, got %v", tok, err)
			}
		}
	})

	t.Run("oauth state single use and expiry", func(t *testing.T) {
		now := time.Now()
		if err := store.SaveOAuthState(ctx, auth.OAuthState{State: "fresh", Provider: "google", ExpiresAt: now.Add(time.Minute)}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := store.SaveOAuthState(ctx, auth.OAuthState{State: "stale", Provider: "google", ExpiresAt: now.Add(-time.Minute)}); err != nil {
			t.Fatalf("save expired: %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "fresh", now); err != nil {
			t.Errorf("consume fresh: %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "fresh", now); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("state is single use: want ErrNotFound, got %v", err)
		}
		if _, err := store.ConsumeOAuthState(ctx, "stale", now); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("expired state: want ErrNotFound, got %v", err)
		}
	})

	t.Run("jobs reports enrichments analytics", func(t *testing.T) {
		now := time.Now()
		seeded := []jobs.Job{
			{ID: "j1", Type: "user.enrich_batch", Status: "queued", Payload: "{}", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
			{ID: "j2", Type: "user.enrich_batch", Status: "done", Payload: "{}", CreatedAt: now.Add(1 * time.Second), UpdatedAt: now},
			{ID: "j3", Type: "user.enrich_batch", Status: "failed", Payload: "{}", Error: "boom", CreatedAt: now, UpdatedAt: now},
		}
		for _, job := range seeded {
			if err := store.SaveJob(ctx, job); err != nil {
				t.Fatalf("save job %s: %v", job.ID, err)
			}
		}
		if err := store.SaveReport(ctx, analytics.Report{ID: "r1", Status: "done", Result: "users=4", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("save report: %v", err)
		}
		if err := store.AddEnrichment(ctx, jobs.EnrichmentProfile{UserID: "u1", Source: "randomuser.me", Payload: "{}", CreatedAt: now}); err != nil {
			t.Fatalf("add enrichment: %v", err)
		}
		listed, _, err := store.ListJobs(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
		if err != nil || len(listed) != 3 {
			t.Fatalf("list jobs: n=%d err=%v", len(listed), err)
		}
		if listed[0].ID != "j1" {
			t.Errorf("jobs are newest first: want j1, got %s", listed[0].ID)
		}
		reports, _, err := store.ListReports(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
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

	t.Run("tickets lifecycle and scoping", func(t *testing.T) {
		now := base.Add(48 * time.Hour)
		typ := tickets.TicketType{ID: "tt1", Key: "expense", Name: "Expense",
			Steps:  []tickets.TicketStep{{Name: "Review", ApproverRole: "approver"}},
			Active: true, CreatedAt: now, UpdatedAt: now}
		if err := store.CreateTicketType(ctx, typ); err != nil {
			t.Fatalf("create type: %v", err)
		}
		if err := store.CreateTicketType(ctx, typ); !errors.Is(err, kernel.ErrConflict) {
			t.Errorf("duplicate type key: want ErrConflict, got %v", err)
		}
		if _, err := store.GetTicketTypeByKey(ctx, "", "expense"); err != nil {
			t.Errorf("get type by key: %v", err)
		}
		orgType := tickets.TicketType{ID: "tt2", OrgID: "org1", Key: "vacation", Name: "Vacation",
			Steps: []tickets.TicketStep{{Name: "Review", ApproverRole: "manager"}}, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := store.CreateTicketType(ctx, orgType); err != nil {
			t.Fatalf("create org type: %v", err)
		}
		if types, err := store.ListTicketTypes(ctx, "org1"); err != nil || len(types) != 2 {
			t.Errorf("org types: want 2, got %d (%v)", len(types), err)
		}
		if types, err := store.ListTicketTypes(ctx, "elsewhere"); err != nil || len(types) != 1 {
			t.Errorf("platform types: want 1, got %d (%v)", len(types), err)
		}

		ticket := func(id, orgID, title, creator string, at time.Time) tickets.Ticket {
			return tickets.Ticket{ID: id, OrgID: orgID, TypeID: "tt1", TypeKey: "expense",
				Title: title, Priority: "high", Status: tickets.TicketDraft, CreatedBy: creator, CreatedAt: at, UpdatedAt: at}
		}
		t1 := ticket("t1", "org1", "Alpha", "u1", now)
		if number, err := store.CreateTicket(ctx, t1); err != nil || number != 1 {
			t.Fatalf("create t1: number=%d err=%v", number, err)
		}
		if number, err := store.CreateTicket(ctx, ticket("t2", "org1", "Beta", "u2", now.Add(time.Hour))); err != nil || number != 2 {
			t.Fatalf("create t2: number=%d err=%v", number, err)
		}
		// The platform namespace numbers independently of org1.
		if number, err := store.CreateTicket(ctx, ticket("t3", "", "Gamma", "u1", now.Add(2*time.Hour))); err != nil || number != 1 {
			t.Fatalf("create t3: number=%d err=%v", number, err)
		}

		t1.Status = tickets.TicketInReview
		if err := store.UpdateTicket(ctx, t1); err != nil {
			t.Fatalf("update t1: %v", err)
		}
		got, err := store.GetTicket(ctx, "t1")
		if err != nil || got.Status != tickets.TicketInReview || got.Number != 1 {
			t.Fatalf("get t1: %+v err=%v", got, err)
		}
		if err := store.UpdateTicket(ctx, ticket("ghost", "", "x", "u1", now)); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("update missing ticket: want ErrNotFound, got %v", err)
		}

		// Approval steps round-trip their decision data.
		decidedAt := now.Add(time.Hour)
		steps := []tickets.TicketApproval{
			{ID: "a1", TicketID: "t1", StepIndex: 0, Name: "Review", ApproverRole: "approver",
				Status: tickets.ApprovalApproved, DecidedBy: "u2", DecidedAt: &decidedAt, Comment: "ok", CreatedAt: now},
		}
		if err := store.ReplaceApprovals(ctx, "t1", steps); err != nil {
			t.Fatalf("replace approvals: %v", err)
		}
		loaded, err := store.ListApprovals(ctx, "t1")
		if err != nil || len(loaded) != 1 || loaded[0].DecidedAt == nil || loaded[0].Comment != "ok" {
			t.Fatalf("list approvals: %+v err=%v", loaded, err)
		}
		if decisions, err := store.ListDecidedApprovals(ctx, "u2", 10, 0); err != nil || len(decisions) != 1 || decisions[0].Ticket.ID != "t1" {
			t.Errorf("decided by u2: %+v err=%v", decisions, err)
		}

		// The approver inbox: t2 waits in org1, t3 lives on the platform.
		if err := store.ReplaceApprovals(ctx, "t2", []tickets.TicketApproval{{ID: "a2", TicketID: "t2", StepIndex: 0, Name: "Review", ApproverRole: "approver", Status: tickets.ApprovalPending, CreatedAt: now}}); err != nil {
			t.Fatalf("t2 approvals: %v", err)
		}
		if err := store.UpdateTicket(ctx, func() tickets.Ticket {
			t := ticket("t2", "org1", "Beta", "u2", now.Add(time.Hour))
			t.Status = tickets.TicketInReview
			return t
		}()); err != nil {
			t.Fatalf("mark t2 in review: %v", err)
		}
		inbox, next, err := store.ListTicketsForApprover(ctx, "org1", []string{"approver"}, kernel.ListOptions{Mode: "offset", Limit: 10})
		if err != nil || len(inbox) != 1 || inbox[0].ID != "t2" || next != "" {
			t.Errorf("inbox: %v next=%q err=%v", inbox, next, err)
		}
		if n, err := store.CountTicketsForApprover(ctx, "org1", []string{"approver"}); err != nil || n != 1 {
			t.Errorf("inbox count: n=%d err=%v", n, err)
		}
		if platformInbox, _, _ := store.ListTicketsForApprover(ctx, "elsewhere", []string{"approver"}, kernel.ListOptions{Mode: "offset", Limit: 10}); len(platformInbox) != 0 {
			t.Errorf("inbox for a foreign org: want empty, got %d", len(platformInbox))
		}

		// Listings honor the scope filters and pagination modes.
		if rows, _, err := store.ListTickets(ctx, kernel.TicketOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, OrgID: "org1"}); err != nil || len(rows) != 2 {
			t.Errorf("org listing: %d rows err=%v", len(rows), err)
		}
		if rows, _, err := store.ListTickets(ctx, kernel.TicketOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, CreatedBy: "u1", OrgID: "org1"}); err != nil || len(rows) != 1 {
			t.Errorf("creator listing: %d rows err=%v", len(rows), err)
		}
		if rows, _, err := store.ListTickets(ctx, kernel.TicketOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10, Status: tickets.TicketInReview}, OrgID: "org1"}); err != nil || len(rows) != 2 {
			t.Errorf("status listing: %+v err=%v", rows, err)
		}
		if rows, _, err := store.ListTickets(ctx, kernel.TicketOptions{ListOptions: kernel.ListOptions{Mode: "keyset", Limit: 10, After: "t2"}, OrgID: "org1"}); err != nil || len(rows) != 1 || rows[0].ID != "t1" {
			t.Errorf("keyset after t2: %+v err=%v", rows, err)
		}
		counts, err := store.CountTicketsByStatus(ctx, "org1", "")
		if err != nil || counts[tickets.TicketInReview] != 2 {
			t.Errorf("counts: %v err=%v", counts, err)
		}

		// Comments and events attach to the ticket and disappear with it.
		if err := store.AddTicketComment(ctx, tickets.TicketComment{ID: "c1", TicketID: "t1", AuthorID: "u1", Body: "hi", CreatedAt: now}); err != nil {
			t.Fatalf("comment: %v", err)
		}
		if err := store.AddTicketEvent(ctx, tickets.TicketEvent{ID: "e1", TicketID: "t1", ActorID: "u1", Action: "created", CreatedAt: now}); err != nil {
			t.Fatalf("event: %v", err)
		}
		if comments, err := store.ListTicketComments(ctx, "t1", 10, 0); err != nil || len(comments) != 1 {
			t.Errorf("comments: %d err=%v", len(comments), err)
		}
		if events, err := store.ListTicketEvents(ctx, "t1", 10, 0); err != nil || len(events) != 1 {
			t.Errorf("events: %d err=%v", len(events), err)
		}

		if err := store.DeleteTicket(ctx, "t1"); err != nil {
			t.Fatalf("delete t1: %v", err)
		}
		if approvals, _ := store.ListApprovals(ctx, "t1"); len(approvals) != 0 {
			t.Errorf("approvals survived delete: %d", len(approvals))
		}
		if err := store.DeleteTicket(ctx, "t1"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("delete missing: want ErrNotFound, got %v", err)
		}
	})
}

func ids(rows []users.User) []string {
	out := make([]string, 0, len(rows))
	for _, u := range rows {
		out = append(out, u.ID)
	}
	return out
}
