// Analytics reports and the event trail on the memory fake: the report
// pages, the warehouse-stats reads, and the sink the fan-out writes into.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/kernel"
)

// eventRecord is one analytics event with the moment it happened.
type eventRecord struct {
	name       string
	userID     string
	at         time.Time
	properties string
}

// topNameCounts sorts a count map and keeps the largest n entries.
func topNameCounts(counts map[string]int64, n int) []analytics.NameCount {
	out := make([]analytics.NameCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, analytics.NameCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// listReportsLocked mirrors listJobsLocked for reports. Caller holds the lock.
func (s *MemoryStore) listReportsLocked(opts kernel.ListOptions) ([]analytics.Report, string) {
	reports := make([]analytics.Report, 0, len(s.reports))
	query := strings.ToLower(opts.Query)
	for _, report := range s.reports {
		if query != "" && !strings.Contains(strings.ToLower(report.Status+" "+report.Result), query) {
			continue
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].CreatedAt.Equal(reports[j].CreatedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})
	rows, next := pageSlice(reports, func(report analytics.Report) string { return report.ID }, opts)
	return rows, next
}

// ListReports returns one page of reports plus the next-page cursor.
func (s *MemoryStore) ListReports(ctx context.Context, opts kernel.ListOptions) ([]analytics.Report, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, next := s.listReportsLocked(opts)
	return rows, next, nil
}

// SaveReport upserts a report by id.
func (s *MemoryStore) SaveReport(ctx context.Context, report analytics.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = report
	return nil
}

// GetReport returns a report by id.
func (s *MemoryStore) GetReport(ctx context.Context, id string) (analytics.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[id]
	if !ok {
		return analytics.Report{}, kernel.ErrNotFound
	}
	return report, nil
}

// RecentEvents returns the most recent analytics events, newest first.
func (s *MemoryStore) RecentEvents(ctx context.Context, limit int) ([]analytics.EventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]analytics.EventRecord, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, analytics.EventRecord{Name: s.events[i].name, OccurredAt: s.events[i].at, UserID: s.events[i].userID, Properties: s.events[i].properties})
	}
	return out, nil
}

// EventCounts aggregates the local event trail by name.
func (s *MemoryStore) EventCounts(ctx context.Context) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int64{}
	for _, e := range s.events {
		counts[e.name]++
	}
	return counts, nil
}

// InsertEvents bulk-loads historical events (demo seed backfill); the fake
// just appends them in the given order.
func (s *MemoryStore) InsertEvents(ctx context.Context, events []analytics.EventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		s.events = append(s.events, eventRecord{name: e.Name, userID: e.UserID, at: e.OccurredAt, properties: e.Properties})
	}
	return nil
}

// DeleteDemoEvents drops events attributed to demo users (demo reset).
func (s *MemoryStore) DeleteDemoEvents(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.events[:0]
	for _, e := range s.events {
		if strings.HasPrefix(e.userID, "demo-") {
			continue
		}
		next = append(next, e)
	}
	s.events = next
	return nil
}

// EventTimeseries aggregates the local event trail by calendar day.
func (s *MemoryStore) EventTimeseries(ctx context.Context, days int) ([]analytics.EventDayPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)
	byDay := map[string]map[string]int64{}
	for _, event := range s.events {
		if event.at.Before(since) {
			continue
		}
		day := event.at.Format("2006-01-02")
		counts, ok := byDay[day]
		if !ok {
			counts = map[string]int64{}
			byDay[day] = counts
		}
		counts[event.name]++
	}
	return dayPoints(byDay), nil
}

// EventBreakdown groups the window's events by one JSON property dimension.
func (s *MemoryStore) EventBreakdown(ctx context.Context, dimension string, days int) ([]analytics.NameCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days)
	key := fmt.Sprintf(`"%s":`, dimension)
	counts := map[string]int64{}
	for _, event := range s.events {
		if event.at.Before(since) {
			continue
		}
		var properties map[string]any
		_ = json.Unmarshal([]byte(event.properties), &properties)
		if value, ok := properties[dimension].(string); ok {
			counts[value]++
			continue
		}
		_ = key
		counts[""]++
	}
	delete(counts, "")
	return topNameCounts(counts, 10), nil
}

// DailyActiveUsers counts distinct event-attributed users per day.
func (s *MemoryStore) DailyActiveUsers(ctx context.Context, days int) ([]analytics.DayCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days)
	byDay := map[string]map[string]bool{}
	for _, event := range s.events {
		if event.at.Before(since) || event.userID == "" {
			continue
		}
		day := event.at.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = map[string]bool{}
		}
		byDay[day][event.userID] = true
	}
	days2 := make([]string, 0, len(byDay))
	for day := range byDay {
		days2 = append(days2, day)
	}
	sort.Strings(days2)
	out := make([]analytics.DayCount, 0, len(days2))
	for _, day := range days2 {
		out = append(out, analytics.DayCount{Day: day, Count: int64(len(byDay[day]))})
	}
	return out, nil
}

// AddEvent records a named analytics event tied to the acting user.
func (s *MemoryStore) AddEvent(ctx context.Context, event, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, eventRecord{name: event, userID: userID, at: time.Now()})
}

// Analytics computes the dashboard summary from the current state.
func (s *MemoryStore) Analytics(ctx context.Context) analytics.AnalyticsSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var summary analytics.AnalyticsSummary
	summary.TotalUsers = len(s.users)
	for _, identity := range s.identities {
		switch identity.Provider {
		case "password":
			summary.PasswordIdentities++
		case "google":
			summary.GoogleIdentities++
		case "github":
			summary.GitHubIdentities++
		}
	}
	for _, job := range s.jobs {
		switch job.Status {
		case "queued":
			summary.JobsQueued++
		case "done":
			summary.JobsDone++
		case "failed":
			summary.JobsFailed++
		}
	}
	summary.EnrichmentSucceeded = len(s.enrichments)
	for _, event := range s.events {
		switch event.name {
		case "user_registered":
			summary.EventsRegistered++
		case "user_logged_in":
			summary.EventsLoggedIn++
		case "oauth_linked":
			summary.EventsOAuthLinked++
		case "job_queued":
			summary.EventsJobsQueued++
		case "job_done":
			summary.EventsJobsDone++
		case "job_failed":
			summary.EventsJobsFailed++
		}
	}
	return summary
}

// dayPoints converts a day→event→count map into the sorted response shape.
func dayPoints(byDay map[string]map[string]int64) []analytics.EventDayPoint {
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	points := make([]analytics.EventDayPoint, 0, len(days))
	for _, day := range days {
		counts := byDay[day]
		var total int64
		for _, n := range counts {
			total += n
		}
		points = append(points, analytics.EventDayPoint{Day: day, Events: counts, Total: total})
	}
	return points
}
