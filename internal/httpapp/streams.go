// Streams admin API: the transactional outbox and the dead-letter queue.
// Operators inspect pending relay events, inspect parked messages, and
// replay them back through the live handler path.
package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	api "github.com/your-org/go-template/internal/api"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// apiListOutbox pages relay events; status=pending filters to unpublished.
func (a *App) apiListOutbox(c *gin.Context) {
	opts := listOptions(c)
	if c.DefaultQuery("status", "") == "pending" {
		opts.Status = "pending"
	}
	events, next, err := a.services.Streams.Outbox(c.Request.Context(), opts)
	if err != nil {
		a.fail(c, err)
		return
	}
	pending, err := a.services.Streams.OutboxPending(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(events), "next_cursor": next, "pending": pending})
}

// apiListDeadLetters pages parked messages; pending=true filters to ones
// not yet replayed.
func (a *App) apiListDeadLetters(c *gin.Context) {
	opts := repository.DeadLetterOptions{ListOptions: listOptions(c), Pending: c.Query("pending") == "true"}
	letters, next, err := a.services.Streams.DeadLetters(c.Request.Context(), opts)
	if err != nil {
		a.fail(c, err)
		return
	}
	pending, err := a.services.Streams.DeadLettersPending(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(letters), "next_cursor": next, "pending": pending})
}

// apiReplayDeadLetter re-runs a parked message through its live handler.
// Every attempt — successful or not — is audit-logged with the outcome.
func (a *App) apiReplayDeadLetter(c *gin.Context) {
	actor := a.currentUser(c)
	letter, err := a.services.Streams.ReplayDeadLetter(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.audit(c, "streams.replay_failed", "dead_letter", c.Param("id"), nil, gin.H{"err": err.Error(), "by": actor.ID})
		a.fail(c, err)
		return
	}
	a.audit(c, "streams.replayed", "dead_letter", letter.ID, nil, gin.H{"source": letter.Source, "reason": letter.Reason, "by": actor.ID})
	c.JSON(http.StatusOK, letter)
}

// apiTriggerEnrichDemo publishes one test enrichment event for a user into
// the requested stream (Kafka profile or NATS contact). The worker applies
// it through the normal pipeline, so the admin can watch the exact
// production path work with a single click.
func (a *App) apiTriggerEnrichDemo(c *gin.Context) {
	if a.demoEnricher == nil {
		a.problem(c, http.StatusServiceUnavailable, "demo enrichment publisher is not configured (brokers are off)", nil)
		return
	}
	var req api.EnrichDemoRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	source := string(req.Source)
	if source != model.SourceKafka && source != model.SourceNats {
		a.problem(c, http.StatusBadRequest, "source must be kafka or nats", nil)
		return
	}
	ctx := c.Request.Context()
	user, err := a.services.Users.Get(ctx, c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	if user.ExternalRef == "" {
		a.problem(c, http.StatusUnprocessableEntity, "this user has no external_ref; edit the user and set one first", nil)
		return
	}
	profileSeq, contactSeq, err := a.services.Streams.StreamSeqs(ctx, user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	var (
		eventID string
		seq     int64
	)
	switch source {
	case model.SourceKafka:
		seq = profileSeq + 1
		eventID, err = a.demoEnricher.PublishProfile(ctx, user.ExternalRef, seq)
	case model.SourceNats:
		seq = contactSeq + 1
		eventID, err = a.demoEnricher.PublishContact(ctx, user.ExternalRef, seq)
	}
	if err != nil {
		a.fail(c, err)
		return
	}
	actor := a.currentUser(c)
	a.audit(c, "streams.demo_published", "user", user.ID, nil, gin.H{
		"source": source, "event_id": eventID, "seq": seq, "by": actor.ID,
	})
	c.JSON(http.StatusOK, api.EnrichDemoResult{
		Published: true, Source: source, EventId: eventID, Seq: seq,
	})
}
