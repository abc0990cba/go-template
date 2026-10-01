// Streams admin API: the transactional outbox and the dead-letter queue.
// Operators inspect pending relay events, inspect parked messages, and
// replay them back through the live handler path.
package streams

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the streams admin handlers' dependencies.
type HTTP struct {
	Svc *Service
	// Users resolves the demo trigger's target user; the direct pointer is
	// cycle-free (users imports kernel/cache only).
	Users *users.Service
	// Demo publishes the admin console's test enrichment events; nil
	// disables the buttons (503).
	Demo   DemoPublisher
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the streams admin routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/streams/outbox", h.RequirePermission("streams", "read"), h.listOutbox)
	authorized.GET("/streams/dead-letters", h.RequirePermission("streams", "read"), h.listDeadLetters)
	authorized.POST("/streams/dead-letters/:id/replay", h.RequirePermission("streams", "replay"), h.replayDeadLetter)
	authorized.POST("/users/:id/enrich-demo", h.RequirePermission("users", "update"), h.triggerEnrichDemo)
}

// listOutbox pages relay events; status=pending filters to unpublished.
func (h *HTTP) listOutbox(c *gin.Context) {
	opts := httpx.ListOptions(c)
	if c.DefaultQuery("status", "") == "pending" {
		opts.Status = "pending"
	}
	events, next, err := h.Svc.Outbox(c.Request.Context(), opts)
	if err != nil {
		h.fail(c, err)
		return
	}
	pending, err := h.Svc.OutboxPending(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(events), "next_cursor": next, "pending": pending})
}

// listDeadLetters pages parked messages; pending=true filters to ones
// not yet replayed.
func (h *HTTP) listDeadLetters(c *gin.Context) {
	opts := kernel.DeadLetterOptions{ListOptions: httpx.ListOptions(c), Pending: c.Query("pending") == "true"}
	letters, next, err := h.Svc.DeadLetters(c.Request.Context(), opts)
	if err != nil {
		h.fail(c, err)
		return
	}
	pending, err := h.Svc.DeadLettersPending(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(letters), "next_cursor": next, "pending": pending})
}

// replayDeadLetter re-runs a parked message through its live handler.
// Every attempt — successful or not — is audit-logged with the outcome.
func (h *HTTP) replayDeadLetter(c *gin.Context) {
	actor := users.CurrentUser(c)
	letter, err := h.Svc.ReplayDeadLetter(c.Request.Context(), c.Param("id"))
	if err != nil {
		audit.Record(h.Audit, c, "streams.replay_failed", "dead_letter", c.Param("id"), nil, gin.H{"err": err.Error(), "by": actor.ID})
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "streams.replayed", "dead_letter", letter.ID, nil, gin.H{"source": letter.Source, "reason": letter.Reason, "by": actor.ID})
	c.JSON(http.StatusOK, letter)
}

// enrichDemoRequest/enrichDemoResult mirror the generated API shapes
// (features do not import internal/api); field order keeps the response
// byte-identical.
type enrichDemoRequest struct {
	Source string `json:"source"`
}

type enrichDemoResult struct {
	EventId   string `json:"event_id"`
	Published bool   `json:"published"`
	Seq       int64  `json:"seq"`
	Source    string `json:"source"`
}

// triggerEnrichDemo publishes one test enrichment event for a user into
// the requested stream (Kafka profile or NATS contact). The worker applies
// it through the normal pipeline, so the admin can watch the exact
// production path work with a single click.
func (h *HTTP) triggerEnrichDemo(c *gin.Context) {
	if h.Demo == nil {
		httpx.Problem(c, http.StatusServiceUnavailable, "demo enrichment publisher is not configured (brokers are off)", nil)
		return
	}
	var req enrichDemoRequest
	if !httpx.Bind(c, &req) {
		return
	}
	source := req.Source
	if source != SourceKafka && source != SourceNats {
		httpx.Problem(c, http.StatusBadRequest, "source must be kafka or nats", nil)
		return
	}
	ctx := c.Request.Context()
	user, err := h.Users.Get(ctx, c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	if user.ExternalRef == "" {
		httpx.Problem(c, http.StatusUnprocessableEntity, "this user has no external_ref; edit the user and set one first", nil)
		return
	}
	profileSeq, contactSeq, err := h.Svc.StreamSeqs(ctx, user.ID)
	if err != nil {
		h.fail(c, err)
		return
	}
	var (
		eventID string
		seq     int64
	)
	switch source {
	case SourceKafka:
		seq = profileSeq + 1
		eventID, err = h.Demo.PublishProfile(ctx, user.ExternalRef, seq)
	case SourceNats:
		seq = contactSeq + 1
		eventID, err = h.Demo.PublishContact(ctx, user.ExternalRef, seq)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	actor := users.CurrentUser(c)
	audit.Record(h.Audit, c, "streams.demo_published", "user", user.ID, nil, gin.H{
		"source": source, "event_id": eventID, "seq": seq, "by": actor.ID,
	})
	c.JSON(http.StatusOK, enrichDemoResult{
		Published: true, Source: source, EventId: eventID, Seq: seq,
	})
}

// fail maps the streams sentinels, then falls through to the shared
// problem+json mapping.
func (h *HTTP) fail(c *gin.Context, err error) {
	if errors.Is(err, ErrAlreadyReplayed) {
		httpx.Problem(c, http.StatusConflict, "already replayed", err)
		return
	}
	httpx.Fail(c, h.Logger, err)
}
