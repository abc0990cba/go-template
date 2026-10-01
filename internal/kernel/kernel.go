// Package kernel holds the shared domain vocabulary: sentinel errors, ID
// generation, pagination options, and the idempotency record contract. It
// imports nothing else from this module, so every layer may depend on it.
package kernel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when a requested entity does not exist. Handlers
// map it to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an insert would violate a uniqueness
// constraint, such as a duplicate email or an identity owned by another user.
// Handlers map it to HTTP 409.
var ErrConflict = errors.New("conflict")

// ErrForbidden is returned when the caller is authenticated but not allowed
// to perform the action. Handlers map it to HTTP 403.
var ErrForbidden = errors.New("forbidden")

// ErrInvalidCredentials is returned when an email/password pair does not
// match. Handlers map it to HTTP 401 without leaking which half failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ValidationError marks a client-fixable problem; handlers map it to 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Invalid returns a client-fixable validation error.
func Invalid(msg string) error { return &ValidationError{Msg: msg} }

// IsValidationError reports whether err or anything it wraps is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// NewID returns a 16-byte random identifier, base64url-encoded. It is the
// single ID generator for every entity and opaque token in the application.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ListOptions controls every paginated listing. Mode selects the pagination
// strategy: "offset" pages by Offset, "cursor" and "keyset" continue after
// the entity id (or enrichment sequence) in After. Status and Type
// optionally narrow job listings; other entities ignore them.
type ListOptions struct {
	Query  string
	Mode   string
	Limit  int
	Offset int
	After  string
	Status string // jobs only: filter by status
	Type   string // jobs only: filter by job type
	UserID string // enrichments only: filter by user id
}

// AuditOptions filters audit listings: ListOptions carries pagination, the
// extra fields narrow by actor, action, resource type, or exact resource id.
// Subject (set by the per-user activity listing) matches everything the user
// did as actor plus every 'user' record about them.
type AuditOptions struct {
	ListOptions
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	Subject    string
}

// TicketOptions narrows ticket listings: ListOptions carries pagination and
// the shared query/status/type filters, the extra fields scope by creator
// (the requester's "my" view), organization, and priority.
type TicketOptions struct {
	ListOptions
	OrgID     string
	CreatedBy string
	Priority  string
}

// DeadLetterOptions narrows dead-letter listings: ListOptions carries
// pagination, Pending filters to not-yet-replayed letters.
type DeadLetterOptions struct {
	ListOptions
	Pending bool
}

// SplitEnrichmentCursor splits an enrichment "after" cursor back into its
// (created_at, sequence) components; the enrichment table has no id column,
// so its keyset cursor carries both.
func SplitEnrichmentCursor(cursor string) (time.Time, int64, bool) {
	before, after, ok := strings.Cut(cursor, ":")
	if !ok {
		return time.Time{}, 0, false
	}
	nanos, err := strconv.ParseInt(before, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	seq, err := strconv.ParseInt(after, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	return time.Unix(0, nanos), seq, true
}

// IdempotencyRecord stores one API idempotency-key reservation and, once the
// request finishes, the response to replay for client retries. Status 0
// means the request is still in flight.
type IdempotencyRecord struct {
	Key          string    `json:"key"`
	UserID       string    `json:"user_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	RequestHash  string    `json:"request_hash"`
	Status       int       `json:"status"`
	ResponseBody []byte    `json:"response_body"`
	ContentType  string    `json:"content_type"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IdempotencyRepo stores API idempotency-key reservations and responses.
// Reserve inserts atomically: it reports already=true when the (user, key)
// pair exists, and the caller compares the stored record to decide between
// replay, rejection, and conflict.
type IdempotencyRepo interface {
	ReserveIdempotency(ctx context.Context, record IdempotencyRecord) (stored IdempotencyRecord, already bool, err error)
	CompleteIdempotency(ctx context.Context, key, userID string, status int, body []byte, contentType string) error
	PruneIdempotency(ctx context.Context) (int64, error)
}

// Pinger reports whether the backing dependencies are ready; health
// endpoints surface it.
type Pinger interface {
	Ping(ctx context.Context) error
}
