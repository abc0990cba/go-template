package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	"github.com/your-org/go-template/internal/config"
)

// The demo dataset gives the SPA something real to show: users spread over
// 90 days, OAuth identities, jobs in every state, reports, enrichment
// profiles, and a 30-day event history. Every id carries the demo- prefix
// so a reset can find and remove exactly what was seeded.

const (
	demoIDPrefix = "demo-"
	// demoMarkerID doubles as the first seeded user: its presence signals
	// "dataset already seeded" without needing an extra marker row.
	demoMarkerID   = "demo-u-0001"
	demoUserCount  = 500
	demoJobCount   = 160
	demoReportN    = 40
	demoEnrichN    = 220
	demoLoginable  = 10 // users 1..10 get a working password
	demoPassword   = "password123"
	demoEventDays  = 90
	demoUserSpan   = 90 * 24 * time.Hour
	demoTraceSpan  = 90 * 24 * time.Hour
	demoDataDomain = "@demo.example"
	demoOrgN       = 3
	demoAuditN     = 200
	demoNotifN     = 60
	demoAPIKeyN    = 2
	demoWebhookN   = 8
	demoTicketN    = 120
)

// name pools keep the dataset realistic without a faker dependency.
var (
	demoFirst = []string{"Ada", "Grace", "Alan", "Linus", "Edsger", "Barbara", "Donald", "Radia", "Tim", "Margaret",
		"Ken", "Frances", "John", "Anita", "Dennis", "Katherine", "Brian", "Shafi", "Peter", "Leslie",
		"Vint", "Elizabeth", "Michael", "Annie", "Larry", "Sophie", "Guido", "Brenda", "James", "Maria"}
	demoLast = []string{"Lovelace", "Hopper", "Turing", "Torvalds", "Dijkstra", "Liskov", "Knuth", "Perlman", "Berners-Lee", "Hamilton",
		"Thompson", "Allen", "Backus", "Borg", "Ritchie", "Johnson", "Kernighan", "Goldwasser", "Norvig", "Lamport",
		"Cerf", "Feinler", "Stonebraker", "Easley", "Wall", "Wilson", "van Rossum", "Fox", "Gosling", "Klawe"}
	demoStatuses = []struct {
		name  string
		share int
	}{{"done", 60}, {"failed", 12}, {"queued", 5}, {"running", 3}}
	// demoTicketStatuses spread the seeded tickets over the whole state
	// machine, with the two terminal outcomes clearly separated.
	demoTicketStatuses = []struct {
		name  string
		share int
	}{{"draft", 15}, {"in_review", 25}, {"approved", 12}, {"completed", 20}, {"rejected", 15}, {"withdrawn", 13}}
)

// SeedDemoData fills the store with the demo dataset unless it already
// exists (the marker user doubles as the idempotency check). It is safe to
// call on every startup when APP_DEMO_SEED=1.
func SeedDemoData(services *Container, repos Repos, cfg config.Config, logger *slog.Logger) {
	started := time.Now()
	if DemoDataExists(repos) {
		logger.Info("demo data already present; skipping seed")
		return
	}
	if err := seedDemoData(context.Background(), services, repos, cfg, logger); err != nil {
		logger.Warn("demo data seed failed; continuing with an empty store", "err", err)
		return
	}
	logger.Info("demo data seeded", "users", demoUserCount, "jobs", demoJobCount, "reports", demoReportN, "duration", time.Since(started).Round(time.Millisecond))
}

// SeedDemoDataIfMissing is the error-returning variant for cmd/seed-demo.
func SeedDemoDataIfMissing(services *Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
	if DemoDataExists(repos) {
		logger.Info("demo data already present; nothing to seed")
		return nil
	}
	return seedDemoData(context.Background(), services, repos, cfg, logger)
}

// DemoDataExists reports whether the marker user is present.
func DemoDataExists(repos Repos) bool {
	_, err := repos.Users.GetUser(context.Background(), demoMarkerID)
	return err == nil
}

// ResetAndSeedDemoData clears everything the seed created and reseeds; it
// backs `make seed-demo-reset` and cmd/seed-demo -reset.
func ResetAndSeedDemoData(services *Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
	ctx := context.Background()
	if err := resetDemoData(ctx, services, repos); err != nil {
		return fmt.Errorf("reset demo data: %w", err)
	}
	return seedDemoData(ctx, services, repos, cfg, logger)
}

// resetDemoData deletes demo users (cascading their credentials,
// identities, and sessions) plus demo jobs, reports, enrichments, and
// warehouse events. It works through an optional store extension so the
// service ports stay untouched.
func resetDemoData(ctx context.Context, services *Container, repos Repos) error {
	type demoResetter interface {
		ResetDemoData(ctx context.Context) error
	}
	if r, ok := repos.Users.(demoResetter); ok {
		if err := r.ResetDemoData(ctx); err != nil {
			return err
		}
	}
	if repos.Warehouse != nil {
		if err := repos.Warehouse.DeleteDemoEvents(ctx); err != nil {
			return err
		}
	}
	// Demo role assignments and org-scoped policies ride the rule store.
	type demoRuleCleaner interface {
		DeleteDemoGroupRules(ctx context.Context) error
		DeleteDemoOrgRules(ctx context.Context) error
	}
	if r, ok := repos.Rules.(demoRuleCleaner); ok {
		if err := r.DeleteDemoGroupRules(ctx); err != nil {
			return err
		}
		if err := r.DeleteDemoOrgRules(ctx); err != nil {
			return err
		}
	}
	// The policy engine cached the rules it loaded before the cleanup and
	// bypassed the deletes (they go through the raw rule store); without a
	// reload casbin would skip re-adding rules it still believes exist.
	if services != nil {
		if err := services.RBAC.ReloadPolicies(); err != nil {
			return err
		}
	}
	return nil
}

func seedDemoData(ctx context.Context, services *Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
	// A fixed seed keeps the dataset reproducible across runs.
	rng := rand.New(rand.NewSource(42))
	now := time.Now()

	if err := seedDemoUsers(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed users: %w", err)
	}
	if err := seedDemoIdentities(ctx, services, rng, now); err != nil {
		return fmt.Errorf("seed identities: %w", err)
	}
	jobs, err := seedDemoJobs(ctx, repos, rng, now)
	if err != nil {
		return fmt.Errorf("seed jobs: %w", err)
	}
	if err := seedDemoReports(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed reports: %w", err)
	}
	if err := seedDemoEnrichments(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed enrichments: %w", err)
	}
	if err := seedDemoEvents(ctx, repos, rng, now, jobs); err != nil {
		return fmt.Errorf("seed events: %w", err)
	}
	if err := seedDemoRBAC(ctx, services, repos, rng, now); err != nil {
		return fmt.Errorf("seed orgs and roles: %w", err)
	}
	orgs, err := demoOrgContexts(ctx, services)
	if err != nil {
		return fmt.Errorf("load demo orgs: %w", err)
	}
	if err := seedDemoTicketTypes(ctx, repos, now); err != nil {
		return fmt.Errorf("seed ticket types: %w", err)
	}
	if err := seedDemoTickets(ctx, repos, rng, now, orgs); err != nil {
		return fmt.Errorf("seed tickets: %w", err)
	}
	if err := seedDemoAudit(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed audit: %w", err)
	}
	if err := seedDemoNotifications(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed notifications: %w", err)
	}
	if err := seedDemoAPIKeys(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed api keys: %w", err)
	}
	if err := seedDemoWebhooks(ctx, repos, rng, now); err != nil {
		return fmt.Errorf("seed webhooks: %w", err)
	}
	return nil
}

func pick[T any](rng *rand.Rand, pool []T) T {
	return pool[rng.Intn(len(pool))]
}

func pickWeighted(rng *rand.Rand, pool []struct {
	name  string
	share int
}) string {
	total := 0
	for _, p := range pool {
		total += p.share
	}
	roll := rng.Intn(total)
	for _, p := range pool {
		if roll < p.share {
			return p.name
		}
		roll -= p.share
	}
	return pool[0].name
}

func pad4(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}
