// Package model defines the domain types shared across layers. JSON tags are
// part of the API contract: response fields are snake_case.
package model

import "time"

// User is an account in the system.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthIdentity links one external identity (password, google, github) to a
// user; (provider, provider_subject) is globally unique.
type AuthIdentity struct {
	UserID          string    `json:"user_id"`
	Provider        string    `json:"provider"`
	ProviderSubject string    `json:"provider_subject"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	AvatarURL       string    `json:"avatar_url"`
	LinkedAt        time.Time `json:"linked_at"`
	LastLoginAt     time.Time `json:"last_login_at"`
}

// OAuthState is a single-use CSRF value for the OAuth redirect round trip.
type OAuthState struct {
	State         string    `json:"state"`
	Provider      string    `json:"provider"`
	UserID        string    `json:"user_id"`
	RedirectAfter string    `json:"redirect_after"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// OAuthProfile is the normalized profile returned by a provider after the
// code exchange.
type OAuthProfile struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatar_url"`
}

// EnrichmentProfile is one fetched external profile record.
type EnrichmentProfile struct {
	UserID    string    `json:"user_id"`
	Source    string    `json:"source"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// Job is a background task record with its lifecycle status.
type Job struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Payload   string    `json:"payload"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Report is a stored analytics snapshot.
type Report struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnalyticsSummary aggregates the counters shown on the dashboard.
type AnalyticsSummary struct {
	TotalUsers          int `json:"total_users"`
	PasswordIdentities  int `json:"password_identities"`
	GoogleIdentities    int `json:"google_identities"`
	GitHubIdentities    int `json:"github_identities"`
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
