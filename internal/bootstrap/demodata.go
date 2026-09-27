package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
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
func SeedDemoData(services *service.Container, repos Repos, cfg config.Config, logger *slog.Logger) {
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
func SeedDemoDataIfMissing(services *service.Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
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
func ResetAndSeedDemoData(services *service.Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
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
func resetDemoData(ctx context.Context, services *service.Container, repos Repos) error {
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

func seedDemoData(ctx context.Context, services *service.Container, repos Repos, cfg config.Config, logger *slog.Logger) error {
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

// demoAccountNames pin readable names onto the six platform demo accounts
// so the login-page table, the README, and the IdentityChip all agree.
var demoAccountNames = map[int]string{
	1: "Ada Admin",
	2: "Max Manager",
	3: "Vera Viewer",
	4: "Rita Requester",
	5: "Ari Approver",
	6: "Sam Staff",
}

// seedDemoUsers creates the marker plus the demo population; users 1..10
// carry a working password (password123), the rest are passwordless accounts
// as if provisioned by an external system.
func seedDemoUsers(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		first, last := pick(rng, demoFirst), pick(rng, demoLast)
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoUserSpan))))
		email := strings.ToLower(first) + "." + strings.ToLower(last) + "." + strconv.Itoa(i) + demoDataDomain
		// The loginable head of the dataset gets predictable addresses so
		// the quickstart can name them: demo-u-0001@demo.example, …
		if i <= demoLoginable {
			email = id + demoDataDomain
		}
		name := first + " " + last
		if fixed, ok := demoAccountNames[i]; ok {
			name = fixed
		}
		user := model.User{
			ID:        id,
			Email:     email,
			Name:      name,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}
		// Argon2id at ~20 ms per hash: ten hashes are startup-cheap and give
		// the quickstart ten real logins. The hash goes through the service
		// helper so logins verify exactly like Register-issued credentials.
		hash := ""
		if i <= demoLoginable {
			hash = service.HashPassword(demoPassword)
		}
		if err := repos.Users.CreateUser(ctx, user, hash); err != nil && !errors.Is(err, repository.ErrConflict) {
			return err
		}
	}
	return nil
}

// seedDemoIdentities links OAuth identities to a slice of the population.
// linked_at is set by the store to "now"; the backfilled oauth_linked
// events carry the historical timestamps instead.
func seedDemoIdentities(ctx context.Context, services *service.Container, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		user, err := services.Users.Get(ctx, id)
		if err != nil {
			continue
		}
		switch {
		case i%4 == 0:
			_ = services.Auth.LinkIdentity(ctx, user.ID, model.OAuthProfile{
				Provider: "google", ProviderSubject: "google-demo-" + strconv.Itoa(i),
				Email: user.Email, Name: user.Name,
			})
		case i%8 == 0:
			_ = services.Auth.LinkIdentity(ctx, user.ID, model.OAuthProfile{
				Provider: "github", ProviderSubject: strconv.Itoa(700000 + i),
				Email: user.Email, Name: user.Name,
			})
		}
	}
	_ = now
	return nil
}

// seedDemoJobs writes jobs across the full status spectrum with backdated
// timestamps and plausible errors; it returns them so the event backfill
// can mirror their lifecycle.
func seedDemoJobs(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) ([]model.Job, error) {
	jobs := make([]model.Job, 0, demoJobCount)
	for i := 1; i <= demoJobCount; i++ {
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan))))
		typ := "user.enrich_batch"
		if i%3 == 0 {
			typ = "user.report_generate"
		}
		status := pickWeighted(rng, demoStatuses)
		payload := "{}"
		if typ == "user.enrich_batch" {
			payload = `{"count":` + strconv.Itoa(1+rng.Intn(20)) + `}`
		}
		jobErr := ""
		if status == "failed" {
			jobErr = pick(rng, []string{
				"context deadline exceeded",
				"permanent upstream status 429",
				"transient upstream status 503",
				"decode payload: unexpected end of JSON input",
			})
		}
		job := model.Job{
			ID: demoIDPrefix + "j-" + pad4(i), Type: typ, Status: status, Payload: payload, Error: jobErr,
			CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Duration(rng.Int63n(int64(3 * time.Minute)))),
		}
		if status == "queued" {
			job.UpdatedAt = job.CreatedAt
		}
		if err := repos.Jobs.SaveJob(ctx, job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func seedDemoReports(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoReportN; i++ {
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan))))
		report := model.Report{
			ID:        demoIDPrefix + "r-" + pad4(i),
			Status:    "done",
			Result:    fmt.Sprintf("users=%d password=%d google=%d github=%d jobs_done=%d", 100+rng.Intn(60), demoLoginable, 25+rng.Intn(10), 12+rng.Intn(6), 40+rng.Intn(20)),
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}
		if i%9 == 0 {
			report.Status = "failed"
			report.Error = "analytics warehouse unavailable"
		}
		if err := repos.Reports.SaveReport(ctx, report); err != nil {
			return err
		}
	}
	return nil
}

func seedDemoEnrichments(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoEnrichN; i++ {
		userID := demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount))
		first, last := pick(rng, demoFirst), pick(rng, demoLast)
		payload := fmt.Sprintf(`{"results":[{"gender":"%s","name":{"first":%q,"last":%q},"email":%q,"location":{"country":"%s"}}]}`,
			pick(rng, []string{"male", "female"}), first, last,
			strings.ToLower(first)+"."+strings.ToLower(last)+"@randomuser.example",
			pick(rng, []string{"US", "DE", "NL", "FI", "IE", "CA"}))
		if err := repos.Jobs.AddEnrichment(ctx, model.EnrichmentProfile{
			UserID: userID, Source: "randomuser.me", Payload: payload,
			CreatedAt: now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan)))),
		}); err != nil {
			return err
		}
	}
	return nil
}

// demoEventProperties pools keep the event dimensions realistic: the
// breakdown charts slice by feature, acquisition source, and country.
var (
	demoFeatures  = []string{"dashboard", "users", "jobs", "analytics", "reports", "settings"}
	demoSources   = []string{"direct", "google", "github", "twitter", "newsletter"}
	demoCountries = []string{"US", "DE", "NL", "FI", "IE", "CA", "GB", "PL", "UA", "BR"}
)

// eventProps renders the properties JSON of one event.
func eventProps(rng *rand.Rand, withCountry bool) string {
	source := fmt.Sprintf(`"source":%q`, pick(rng, demoSources))
	country := ""
	if withCountry {
		country = `,"country":` + fmt.Sprintf("%q", pick(rng, demoCountries))
	}
	return "{" + source + country + "}"
}

// seedDemoEvents backfills the warehouse trail: registrations at user
// creation moments, daily logins with business-hours bias, page views and
// feature usage for the charts, and the job lifecycle mirrored from the
// seeded jobs. Roughly 20-30k events over the 90-day window.
func seedDemoEvents(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time, jobs []model.Job) error {
	if repos.Warehouse == nil {
		return nil
	}
	events := make([]model.EventRecord, 0, 4*demoEventDays*8+demos)

	// Registrations at the moment each demo user appeared.
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		user, err := repos.Users.GetUser(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, model.EventRecord{
			Name: "user_registered", OccurredAt: user.CreatedAt, UserID: user.ID,
			Properties: eventProps(rng, true),
		})
	}

	for d := demoEventDays; d >= 1; d-- {
		base := now.AddDate(0, 0, -d)
		at := func() time.Time {
			hour := 8 + rng.Intn(11) // business-hours bias
			return time.Date(base.Year(), base.Month(), base.Day(), hour, rng.Intn(60), rng.Intn(60), 0, base.Location())
		}
		// Logins: 8..24 per day.
		for n := 8 + rng.Intn(17); n > 0; n-- {
			events = append(events, model.EventRecord{
				Name: "user_logged_in", OccurredAt: at(),
				UserID:     demoIDPrefix + "u-" + pad4(1+rng.Intn(demoLoginable)),
				Properties: eventProps(rng, true),
			})
		}
		// Page views: the traffic curve the charts live on (weekday-heavy).
		views := 30 + rng.Intn(50)
		if base.Weekday() == time.Saturday || base.Weekday() == time.Sunday {
			views /= 3
		}
		for n := views; n > 0; n-- {
			events = append(events, model.EventRecord{
				Name: "page_view", OccurredAt: at(),
				UserID:     demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
				Properties: eventProps(rng, true),
			})
		}
		// Feature usage: the stacked-bar dimension.
		for n := 8 + rng.Intn(18); n > 0; n-- {
			events = append(events, model.EventRecord{
				Name: "feature_used", OccurredAt: at(),
				UserID:     demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
				Properties: fmt.Sprintf(`{"feature":%q,"source":%q}`, pick(rng, demoFeatures), pick(rng, demoSources)),
			})
		}
		// Operational pings keep the secondary series alive.
		for n := 1 + rng.Intn(4); n > 0; n-- {
			events = append(events, model.EventRecord{
				Name:       pick(rng, []string{"notification_sent", "api_key_created", "settings_updated", "report_generated"}),
				OccurredAt: at(), UserID: demoIDPrefix + "u-" + pad4(1+rng.Intn(demoLoginable)),
				Properties: eventProps(rng, false),
			})
		}
	}

	// OAuth links and the job lifecycle.
	for i := 1; i <= demoUserCount; i++ {
		if i%4 == 0 || i%8 == 0 {
			events = append(events, model.EventRecord{
				Name: "oauth_linked", OccurredAt: now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan)))),
				UserID:     demoIDPrefix + "u-" + pad4(i),
				Properties: eventProps(rng, false),
			})
		}
	}
	for _, job := range jobs {
		events = append(events, model.EventRecord{Name: "job_queued", OccurredAt: job.CreatedAt, Properties: "{}"})
		name := "job_done"
		if job.Status == "failed" {
			name = "job_failed"
		}
		if job.Status != "queued" {
			events = append(events, model.EventRecord{Name: name, OccurredAt: job.UpdatedAt, Properties: "{}"})
		}
	}
	return repos.Warehouse.InsertEvents(ctx, events)
}

// seedDemoRBAC creates three organizations, their memberships, and the role
// assignments that make the RBAC admin immediately explorable: the first
// demo user is a platform admin, org leads hold manager, the rest viewer.
func seedDemoRBAC(ctx context.Context, services *service.Container, repos Repos, rng *rand.Rand, now time.Time) error {
	orgNames := []struct{ name, slug string }{
		{"Acme Corp", "acme"}, {"Globex", "globex"}, {"Initech", "initech"},
	}
	for n, named := range orgNames[:demoOrgN] {
		org := model.Organization{
			ID:        fmt.Sprintf("%sorg-%d", demoIDPrefix, n+1),
			Name:      named.name,
			Slug:      named.slug,
			CreatedAt: now.Add(-time.Duration(rng.Int63n(int64(demoUserSpan)))),
		}
		if err := repos.Orgs.CreateOrg(ctx, org); err != nil && !errors.Is(err, repository.ErrConflict) {
			return err
		}
		// Members: the org lead plus a slice of the population.
		lead := demoIDPrefix + "u-" + pad4(10*(n+1))
		if err := repos.Orgs.AddOrgMember(ctx, model.OrgMember{OrgID: org.ID, UserID: lead, JoinedAt: now.Add(-720 * time.Hour)}); err != nil {
			return err
		}
		if err := services.RBAC.AssignRole(lead, "manager", org.ID); err != nil {
			return err
		}
		for m := 0; m < 6; m++ {
			member := demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount))
			if err := repos.Orgs.AddOrgMember(ctx, model.OrgMember{OrgID: org.ID, UserID: member, JoinedAt: now.Add(-time.Duration(rng.Int63n(int64(400 * time.Hour))))}); err != nil {
				continue // duplicates are fine
			}
			// Members file tickets; every second one can also approve, so
			// the approvals inbox is populated and some users see both
			// interfaces.
			_ = services.RBAC.AssignRole(member, "requester", org.ID)
			if m%2 == 0 {
				_ = services.RBAC.AssignRole(member, "approver", org.ID)
			}
		}
	}
	// Platform-level grants for the loginable head of the dataset. The six
	// named accounts cover every interface: admin, manager, viewer, a pure
	// requester, a pure approver, and a requester+approver hybrid — the
	// quickstart table and the login page walk through them.
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0001", "admin", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0002", "manager", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0003", "viewer", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0004", "requester", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0005", "approver", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0006", "requester", service.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0006", "approver", service.PlatformDomain); err != nil {
		return err
	}
	return nil
}

// seedDemoAudit writes a believable admin trail over the last 30 days.
func seedDemoAudit(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	actions := []struct {
		action, resource string
	}{
		{"user.created", "user"}, {"user.updated", "user"}, {"user.logged_in", "user"},
		{"role.permissions_updated", "role"}, {"org.created", "org"}, {"org.member_added", "org"},
		{"settings.updated", "setting"}, {"webhook.created", "webhook"},
	}
	for i := 0; i < demoAuditN; i++ {
		a := pick(rng, actions)
		actorN := 1 + rng.Intn(demoLoginable)
		entry := model.AuditEntry{
			ID:           demoIDPrefix + "a-" + pad4(i+1),
			ActorID:      demoIDPrefix + "u-" + pad4(actorN),
			ActorEmail:   demoIDPrefix + "u-" + pad4(actorN) + demoDataDomain,
			Action:       a.action,
			ResourceType: a.resource,
			ResourceID:   demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
			Domain:       "*",
			Diff:         "{}",
			IP:           fmt.Sprintf("203.0.113.%d", 1+rng.Intn(200)),
			RequestID:    demoIDPrefix + "r-" + pad4(1+rng.Intn(9999)),
			CreatedAt:    now.Add(-time.Duration(rng.Int63n(int64(30 * 24 * time.Hour)))),
		}
		if err := repos.Audit.RecordAudit(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoNotifications gives the bell something to show; a third stay
// unread so the badge is live at first login.
func seedDemoNotifications(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	titles := []struct{ typ, title, body, link string }{
		{"job.done", "Job user.enrich_batch finished", "Completed successfully.", "/jobs"},
		{"job.failed", "Job user.report_generate failed", "transient upstream status 503", "/jobs"},
		{"security.password_changed", "Password changed", "Your password was reset via email.", "/profile"},
		{"impersonation.started", "An administrator signed in to your account", "Contact support if this was not expected.", "/profile"},
	}
	// Address them to the loginable users so demo logins see a full bell.
	for i := 0; i < demoNotifN; i++ {
		userID := demoIDPrefix + "u-" + pad4(1+rng.Intn(demoLoginable))
		t := pick(rng, titles)
		notification := model.Notification{
			ID: demoIDPrefix + "n-" + pad4(i+1), UserID: userID,
			Type: t.typ, Title: t.title, Body: t.body, Link: t.link,
			CreatedAt: now.Add(-time.Duration(rng.Int63n(int64(14 * 24 * time.Hour)))),
		}
		if i%3 != 0 { // every third stays unread
			readAt := notification.CreatedAt.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
			if readAt.After(now) {
				readAt = now
			}
			notification.ReadAt = &readAt
		}
		if err := repos.Notifications.CreateNotification(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoAPIKeys issues two keys attributed to the demo admin; only
// deterministic hashes are stored, so nobody can authenticate with them.
func seedDemoAPIKeys(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoAPIKeyN; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("demo-key-%d", i)))
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(60 * 24 * time.Hour))))
		var expires *time.Time
		if i == 2 {
			t := createdAt.Add(720 * time.Hour)
			expires = &t
		}
		key := model.APIKey{
			ID: fmt.Sprintf("%skey-%d", demoIDPrefix, i), UserID: demoIDPrefix + "u-0001",
			Name:       pick(rng, []string{"CI integration", "Local scripts", "Analytics exporter"}),
			Prefix:     "gtpl_demo" + strconv.Itoa(i),
			Scopes:     []string{"users:read", "jobs:read"},
			LastUsedAt: &createdAt, ExpiresAt: expires, CreatedAt: createdAt,
		}
		if err := repos.APIKeys.CreateAPIKey(ctx, key, base64.RawURLEncoding.EncodeToString(sum[:])); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoWebhooks writes one inactive endpoint (no real external calls)
// plus a delivery history for the log UI.
func seedDemoWebhooks(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	endpoint := model.WebhookEndpoint{
		ID:          demoIDPrefix + "wh-0001",
		URL:         "https://hooks.demo.example/platform",
		Secret:      "demo-secret-not-used-for-delivery",
		Events:      []string{"user.logged_in", "job.done", "job.failed"},
		Description: "Sample subscription (inactive — flip it on with your own URL)",
		Active:      false, OrgID: "", CreatedBy: demoIDPrefix + "u-0001",
		CreatedAt: now.Add(-30 * 24 * time.Hour), UpdatedAt: now.Add(-30 * 24 * time.Hour),
	}
	if err := repos.Webhooks.InsertWebhookEndpoint(ctx, endpoint); err != nil {
		return err
	}
	for i := 0; i < demoWebhookN; i++ {
		deliveredAt := now.Add(-time.Duration(rng.Int63n(int64(14 * 24 * time.Hour))))
		status := "success"
		if i%5 == 0 {
			status = "failed"
		}
		attempts := 1
		lastError := ""
		responseStatus := 200
		if status == "failed" {
			responseStatus = 503
			lastError = "endpoint answered 503"
			attempts = 3
		}
		delivery := model.WebhookDelivery{
			ID: demoIDPrefix + "wd-" + pad4(i+1), EndpointID: endpoint.ID,
			Event:   pick(rng, []string{"user.logged_in", "job.done", "job.failed"}),
			Payload: "{}", Status: status, Attempts: attempts,
			LastResponseStatus: responseStatus, LastError: lastError,
			CreatedAt: deliveredAt,
		}
		if status == "success" {
			delivery.DeliveredAt = &deliveredAt
		}
		if err := repos.Webhooks.InsertWebhookDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	return nil
}

// demos is the rough pre-allocation size of the event backfill.
const demos = 90 * 70

// demoTicketTypeSpec is one seeded ticket type with its approval chain.
type demoTicketTypeSpec struct {
	key, name, description string
	steps                  []model.TicketStep
}

// demoTicketTypes exercise multi-step chains over both approving roles:
// the org lead holds manager, approver roles go to half the members.
var demoTicketTypes = []demoTicketTypeSpec{
	{"expense", "Expense report", "Reimburse travel, training, or equipment costs", []model.TicketStep{
		{Name: "Manager review", ApproverRole: "manager"},
		{Name: "Finance approval", ApproverRole: "approver"},
	}},
	{"access_request", "Access request", "Grant or extend access to a system", []model.TicketStep{
		{Name: "Access approval", ApproverRole: "approver"},
	}},
	{"vacation", "Vacation request", "Book paid time off", []model.TicketStep{
		{Name: "Manager review", ApproverRole: "manager"},
		{Name: "HR approval", ApproverRole: "approver"},
	}},
}

// demoTicketTitles pools realistic subjects per type.
var demoTicketTitles = map[string][]string{
	"expense": {
		"Team offsite travel reimbursement",
		"Kubernetes certification exam fee",
		"Home office monitor purchase",
		"Client visit mileage expenses",
		"Conference tickets for the platform team",
	},
	"access_request": {
		"Production database read access",
		"CI/CD admin rights for the release week",
		"Analytics warehouse access for the data team",
		"On-call PagerDuty schedule access",
		"VPN split-tunnel exception",
	},
	"vacation": {
		"Annual leave in late October",
		"Long weekend away",
		"Family trip abroad",
		"Study leave for the exam week",
		"Winter holiday block",
	},
}

var demoTicketPriorities = []struct {
	name  string
	share int
}{{"low", 25}, {"medium", 45}, {"high", 20}, {"urgent", 10}}

var demoTicketComments = []string{
	"Uploaded the receipts, please take a look.",
	"Could you clarify the cost center for this?",
	"Approved on my side — moving it forward.",
	"This exceeds the quarterly budget line; can we split it?",
	"Thanks for the quick turnaround!",
	"Blocking until the vendor invoice arrives.",
	"Details updated as discussed on the call.",
	"Reminding everyone this needs two approvals.",
}

var demoTicketRejections = []string{
	"Missing itemized receipts; please resubmit with attachments.",
	"Overlaps with the existing subscription — consolidate first.",
	"Not within the current budget cycle, retry next quarter.",
	"Policy requires the manager's sign-off before this step.",
}

// seedDemoTicketTypes writes the platform-wide ticket types.
func seedDemoTicketTypes(ctx context.Context, repos Repos, now time.Time) error {
	for _, spec := range demoTicketTypes {
		typ := model.TicketType{
			ID:          fmt.Sprintf("%stt-%s", demoIDPrefix, spec.key),
			Key:         spec.key,
			Name:        spec.name,
			Description: spec.description,
			Steps:       spec.steps,
			Active:      true,
			CreatedBy:   demoIDPrefix + "u-0001",
			CreatedAt:   now.Add(-60 * 24 * time.Hour),
			UpdatedAt:   now.Add(-60 * 24 * time.Hour),
		}
		if err := repos.Tickets.CreateTicketType(ctx, typ); err != nil && !errors.Is(err, repository.ErrConflict) {
			return err
		}
	}
	return nil
}

// demoOrgContext carries one seeded org's cast: who files tickets, who
// approves them, who manages them.
type demoOrgContext struct {
	id        string
	members   []string
	approvers []string
	managers  []string
}

// decideAs picks a plausible decider for the role inside the org, falling
// back to any member when the cast is thin.
func (c demoOrgContext) decideAs(rng *rand.Rand, role string) string {
	switch role {
	case "manager":
		if len(c.managers) > 0 {
			return pick(rng, c.managers)
		}
	case "approver":
		if len(c.approvers) > 0 {
			return pick(rng, c.approvers)
		}
	}
	return pick(rng, c.members)
}

// demoOrgContexts reloads the seeded organizations with their members and
// role holders, so tickets land in orgs their members can actually see.
func demoOrgContexts(ctx context.Context, services *service.Container) ([]demoOrgContext, error) {
	orgs, _, err := services.RBAC.ListOrgs(ctx, repository.ListOptions{Mode: "offset", Limit: 100})
	if err != nil {
		return nil, err
	}
	var out []demoOrgContext
	for _, org := range orgs {
		if !strings.HasPrefix(org.ID, demoIDPrefix) {
			continue
		}
		members, err := services.RBAC.OrgMembers(ctx, org.ID)
		if err != nil {
			return nil, err
		}
		c := demoOrgContext{id: org.ID}
		for _, m := range members {
			c.members = append(c.members, m.UserID)
			for _, role := range services.RBAC.RolesForUser(m.UserID, org.ID) {
				switch role {
				case "approver":
					c.approvers = append(c.approvers, m.UserID)
				case "manager":
					c.managers = append(c.managers, m.UserID)
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// seedDemoTickets writes tickets across the full state machine: consistent
// approval steps for the status (pending at the current step, skipped after
// a rejection, all approved on the happy path), a timeline per ticket, and
// a discussion on some of them. Most tickets land in the demo orgs; the
// rest live in the platform domain where the seeded admins act.
func seedDemoTickets(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time, orgs []demoOrgContext) error {
	// The platform cast mirrors the documented demo accounts: u-0004/u-0006
	// file tickets, u-0002 decides the manager steps, u-0005 (+ the admin
	// u-0001) the approver steps — every loginable account owns a slice of
	// the flow.
	platform := demoOrgContext{
		id:        service.PlatformDomain,
		members:   []string{demoIDPrefix + "u-0004", demoIDPrefix + "u-0005", demoIDPrefix + "u-0006", demoIDPrefix + "u-0001"},
		approvers: []string{demoIDPrefix + "u-0005", demoIDPrefix + "u-0001"},
		managers:  []string{demoIDPrefix + "u-0002"},
	}
	for i := 1; i <= demoTicketN; i++ {
		spec := demoTicketTypes[rng.Intn(len(demoTicketTypes))]
		status := pickWeighted(rng, demoTicketStatuses)
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan))))
		cast := platform
		if len(orgs) > 0 && rng.Intn(5) != 0 {
			cast = orgs[rng.Intn(len(orgs))]
		}

		ticket := model.Ticket{
			ID:          demoIDPrefix + "t-" + pad4(i),
			OrgID:       cast.id,
			TypeID:      fmt.Sprintf("%stt-%s", demoIDPrefix, spec.key),
			TypeKey:     spec.key,
			Title:       pick(rng, demoTicketTitles[spec.key]),
			Description: "Seeded demo ticket for the " + spec.name + " flow.",
			Priority:    pickWeighted(rng, demoTicketPriorities),
			Status:      model.TicketDraft,
			CreatedBy:   pick(rng, cast.members),
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}
		// Org leads occasionally file their own, so the manager logins have
		// a "my tickets" history too.
		if len(cast.managers) > 0 && rng.Intn(4) == 0 {
			ticket.CreatedBy = cast.managers[0]
		}
		number, err := repos.Tickets.CreateTicket(ctx, ticket)
		if err != nil {
			return err
		}
		ticket.Number = number

		submittedAt := createdAt.Add(time.Duration(rng.Int63n(int64(2 * time.Hour))))
		actor := func(role string) string { return cast.decideAs(rng, role) }
		steps := make([]model.TicketApproval, 0, len(spec.steps))
		for idx, step := range spec.steps {
			steps = append(steps, model.TicketApproval{
				ID:       fmt.Sprintf("%sta-%04d-%02d", demoIDPrefix, i, idx),
				TicketID: ticket.ID, StepIndex: idx, Name: step.Name,
				ApproverRole: step.ApproverRole, Status: model.ApprovalPending, CreatedAt: submittedAt,
			})
		}
		advance := func(idx int, status, comment string, at time.Time) {
			steps[idx].Status = status
			steps[idx].DecidedBy = actor(steps[idx].ApproverRole)
			steps[idx].DecidedAt = &at
			steps[idx].Comment = comment
		}

		events := []model.TicketEvent{
			{ID: fmt.Sprintf("%ste-%04d-00", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "created", Detail: ticket.Title, CreatedAt: createdAt},
		}

		switch status {
		case model.TicketDraft:
			ticket.UpdatedAt = createdAt
		case model.TicketInReview:
			ticket.Status = model.TicketInReview
			ticket.UpdatedAt = submittedAt
			events = append(events, model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt})
			if rng.Intn(2) == 1 && len(steps) > 1 {
				// Mid-chain: the first step already approved.
				decidedAt := submittedAt.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
				advance(0, model.ApprovalApproved, "Looks fine to me.", decidedAt)
				ticket.CurrentStep = 1
				events = append(events, model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-02", demoIDPrefix, i), TicketID: ticket.ID, ActorID: steps[0].DecidedBy, Action: "step_approved", Detail: steps[0].Name, CreatedAt: decidedAt})
			}
		case model.TicketApproved, model.TicketCompleted:
			ticket.Status = status
			at := submittedAt
			for idx := range steps {
				at = at.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
				advance(idx, model.ApprovalApproved, pick(rng, []string{"", "Approved.", "All good.", ""}), at)
				events = append(events, model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-%02d", demoIDPrefix, i, idx+1), TicketID: ticket.ID, ActorID: steps[idx].DecidedBy, Action: "approved", Detail: steps[idx].Name, CreatedAt: at})
			}
			ticket.DecidedBy = steps[len(steps)-1].DecidedBy
			ticket.DecidedAt = &at
			ticket.UpdatedAt = at
			if status == model.TicketCompleted {
				completedAt := at.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
				ticket.UpdatedAt = completedAt
				events = append(events, model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-99", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "completed", Detail: "result delivered by the requester", CreatedAt: completedAt})
			}
		case model.TicketRejected:
			ticket.Status = model.TicketRejected
			decidedAt := submittedAt.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
			stepIdx := rng.Intn(len(steps))
			for idx := 0; idx < stepIdx; idx++ {
				advance(idx, model.ApprovalApproved, "", submittedAt.Add(time.Duration(rng.Int63n(int64(24*time.Hour)))))
			}
			advance(stepIdx, model.ApprovalRejected, pick(rng, demoTicketRejections), decidedAt)
			for idx := stepIdx + 1; idx < len(steps); idx++ {
				steps[idx].Status = model.ApprovalSkipped
			}
			ticket.CurrentStep = stepIdx
			ticket.DecidedBy = steps[stepIdx].DecidedBy
			ticket.DecidedAt = &decidedAt
			ticket.UpdatedAt = decidedAt
			events = append(events,
				model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt},
				model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-90", demoIDPrefix, i), TicketID: ticket.ID, ActorID: steps[stepIdx].DecidedBy, Action: "rejected", Detail: steps[stepIdx].Comment, CreatedAt: decidedAt},
			)
		case model.TicketWithdrawn:
			ticket.Status = model.TicketWithdrawn
			withdrawnAt := submittedAt.Add(time.Duration(rng.Int63n(int64(24 * time.Hour))))
			for idx := range steps {
				steps[idx].Status = model.ApprovalSkipped
			}
			ticket.DecidedBy = ticket.CreatedBy
			ticket.DecidedAt = &withdrawnAt
			ticket.UpdatedAt = withdrawnAt
			events = append(events,
				model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt},
				model.TicketEvent{ID: fmt.Sprintf("%ste-%04d-95", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "withdrawn", Detail: "withdrawn by the requester", CreatedAt: withdrawnAt},
			)
		}

		if err := repos.Tickets.ReplaceApprovals(ctx, ticket.ID, steps); err != nil {
			return err
		}
		if err := repos.Tickets.UpdateTicket(ctx, ticket); err != nil {
			return err
		}
		for _, event := range events {
			if err := repos.Tickets.AddTicketEvent(ctx, event); err != nil {
				return err
			}
		}
		// A discussion on roughly half of the tickets.
		for n := rng.Intn(3); n > 0; n-- {
			author := ticket.CreatedBy
			if rng.Intn(2) == 1 {
				author = actor(pick(rng, []string{"manager", "approver"}))
			}
			commentAt := createdAt.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
			comment := model.TicketComment{
				ID:       fmt.Sprintf("%stc-%04d-%02d", demoIDPrefix, i, n),
				TicketID: ticket.ID, AuthorID: author,
				Body: pick(rng, demoTicketComments), CreatedAt: commentAt,
			}
			if err := repos.Tickets.AddTicketComment(ctx, comment); err != nil {
				return err
			}
		}
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
