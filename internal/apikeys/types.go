package apikeys

import "time"

// Key is one personal API key. Only the Prefix is safe to display; the raw
// secret is returned once at creation and only its hash is stored.
type Key struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	OrgID      string     `json:"org_id,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
