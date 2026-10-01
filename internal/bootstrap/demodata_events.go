package bootstrap

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/jobs"
)

// demos is the rough pre-allocation size of the event backfill.
const demos = 90 * 70

// seedDemoJobs writes jobs across the full status spectrum with backdated
// timestamps and plausible errors; it returns them so the event backfill
// can mirror their lifecycle.
func seedDemoJobs(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) ([]jobs.Job, error) {
	seeded := make([]jobs.Job, 0, demoJobCount)
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
		job := jobs.Job{
			ID: demoIDPrefix + "j-" + pad4(i), Type: typ, Status: status, Payload: payload, Error: jobErr,
			CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Duration(rng.Int63n(int64(3 * time.Minute)))),
		}
		if status == "queued" {
			job.UpdatedAt = job.CreatedAt
		}
		if err := repos.Jobs.SaveJob(ctx, job); err != nil {
			return nil, err
		}
		seeded = append(seeded, job)
	}
	return seeded, nil
}

func seedDemoReports(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoReportN; i++ {
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan))))
		report := analytics.Report{
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
		if err := repos.Jobs.AddEnrichment(ctx, jobs.EnrichmentProfile{
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
func seedDemoEvents(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time, seeded []jobs.Job) error {
	if repos.Warehouse == nil {
		return nil
	}
	events := make([]analytics.EventRecord, 0, 4*demoEventDays*8+demos)

	// Registrations at the moment each demo user appeared.
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		user, err := repos.Users.GetUser(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, analytics.EventRecord{
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
			events = append(events, analytics.EventRecord{
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
			events = append(events, analytics.EventRecord{
				Name: "page_view", OccurredAt: at(),
				UserID:     demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
				Properties: eventProps(rng, true),
			})
		}
		// Feature usage: the stacked-bar dimension.
		for n := 8 + rng.Intn(18); n > 0; n-- {
			events = append(events, analytics.EventRecord{
				Name: "feature_used", OccurredAt: at(),
				UserID:     demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
				Properties: fmt.Sprintf(`{"feature":%q,"source":%q}`, pick(rng, demoFeatures), pick(rng, demoSources)),
			})
		}
		// Operational pings keep the secondary series alive.
		for n := 1 + rng.Intn(4); n > 0; n-- {
			events = append(events, analytics.EventRecord{
				Name:       pick(rng, []string{"notification_sent", "api_key_created", "settings_updated", "report_generated"}),
				OccurredAt: at(), UserID: demoIDPrefix + "u-" + pad4(1+rng.Intn(demoLoginable)),
				Properties: eventProps(rng, false),
			})
		}
	}

	// OAuth links and the job lifecycle.
	for i := 1; i <= demoUserCount; i++ {
		if i%4 == 0 || i%8 == 0 {
			events = append(events, analytics.EventRecord{
				Name: "oauth_linked", OccurredAt: now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan)))),
				UserID:     demoIDPrefix + "u-" + pad4(i),
				Properties: eventProps(rng, false),
			})
		}
	}
	for _, job := range seeded {
		events = append(events, analytics.EventRecord{Name: "job_queued", OccurredAt: job.CreatedAt, Properties: "{}"})
		name := "job_done"
		if job.Status == "failed" {
			name = "job_failed"
		}
		if job.Status != "queued" {
			events = append(events, analytics.EventRecord{Name: name, OccurredAt: job.UpdatedAt, Properties: "{}"})
		}
	}
	return repos.Warehouse.InsertEvents(ctx, events)
}
