// Package analytics carries the dashboard summary, the stored snapshot
// reports, and the event trail: its sinks (the producers), its warehouse
// stats port (the readers), and the HTTP surface for both.
package analytics

import "time"

// EventRecord is a single analytics event from the trail, as delivered by
// the SSE stream and the recent-events endpoint. Properties carries a JSON
// object of dimensions (feature, source, country) for breakdowns.
type EventRecord struct {
	Name       string    `json:"name"`
	OccurredAt time.Time `json:"occurred_at"`
	UserID     string    `json:"user_id,omitempty"`
	Properties string    `json:"properties,omitempty"`
}

// NameCount is one labeled aggregate: a breakdown row or a counter.
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// DayCount is one day of a daily aggregate such as DAU.
type DayCount struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// EventDayPoint aggregates one calendar day of the event trail by name;
// charts render it as a stacked series.
type EventDayPoint struct {
	Day    string           `json:"day"` // YYYY-MM-DD
	Events map[string]int64 `json:"events"`
	Total  int64            `json:"total"`
}

// Report is a stored analytics snapshot.
type Report struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	OrgID     string    `json:"org_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnalyticsSummary aggregates the counters shown on the dashboard.
type AnalyticsSummary struct {
	TotalUsers          int `json:"total_users"`
	PasswordIdentities  int `json:"password_identities"`
	GoogleIdentities    int `json:"google_identities"`
	GitHubIdentities    int `json:"github_identities"`
	FacebookIdentities  int `json:"facebook_identities"`
	TwitterIdentities   int `json:"twitter_identities"`
	EnrichmentSucceeded int `json:"enrichment_succeeded"`
	EnrichmentFailed    int `json:"enrichment_failed"`
	JobsQueued          int `json:"jobs_queued"`
	JobsDone            int `json:"jobs_done"`
	JobsFailed          int `json:"jobs_failed"`
	EventsRegistered    int `json:"events_registered,omitempty"`
	EventsLoggedIn      int `json:"events_logged_in,omitempty"`
	EventsOAuthLinked   int `json:"events_oauth_linked,omitempty"`
	EventsJobsQueued    int `json:"events_jobs_queued,omitempty"`
	EventsJobsDone      int `json:"events_jobs_done,omitempty"`
	EventsJobsFailed    int `json:"events_jobs_failed,omitempty"`
}
