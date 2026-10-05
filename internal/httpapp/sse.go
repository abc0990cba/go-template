package httpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/realtime"
)

// SSE stream tuning: heartbeats keep proxies from idling the connection out,
// snapshots bound cross-process staleness when no Redis bridge relays worker
// activity (the worker updates the shared store either way).
const (
	sseHeartbeatEvery  = 15 * time.Second
	sseSnapshotEvery   = 5 * time.Second
	sseSnapshotJobPage = 10
)

// snapshot is the periodic state push; it lets the UI converge even when it
// missed live events.
type snapshot struct {
	Summary analytics.Summary `json:"summary"`
	Jobs    []jobs.Job        `json:"jobs"`
}

// realtimeSubscriber resolves the connection's identity once: org
// memberships, platform-admin flag, and the jobs permission that gates the
// periodic snapshot — everything Publish needs to scope delivery.
func (a *App) realtimeSubscriber(c *gin.Context) realtime.Subscriber {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	memberships, err := a.services.RBAC.OrgsForUser(c.Request.Context(), user.ID)
	orgs := make([]string, 0, len(memberships))
	for _, org := range memberships {
		orgs = append(orgs, org.ID)
	}
	if err != nil {
		orgs = nil
	}
	sub := realtime.Subscriber{
		UserID:      user.ID,
		Orgs:        orgs,
		Admin:       a.services.RBAC.Enforce(user.ID, rbac.PlatformDomain, "*", "*"),
		CanReadJobs: a.services.RBAC.Enforce(user.ID, domain, "jobs", "read"),
	}
	return sub
}

// sseStream pushes analytics events, job transitions, and periodic
// snapshots to the browser. Auth happens in the apiAuth middleware; the
// stream itself is anonymous bytes on an authenticated connection, scoped
// to the subscriber's orgs (or personal events only).
func (a *App) sseStream(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		a.problem(c, http.StatusInternalServerError, "streaming unsupported", nil)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sub := a.realtimeSubscriber(c)
	events, cancel := a.hub.Subscribe(sub)
	defer cancel()

	writeSSE(c, flusher, realtime.NewEvent(realtime.TypeConnected, gin.H{"at": time.Now().UTC()}))

	heartbeat := time.NewTicker(sseHeartbeatEvery)
	defer heartbeat.Stop()
	snapshots := time.NewTicker(sseSnapshotEvery)
	defer snapshots.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			writeSSE(c, flusher, ev)
		case <-heartbeat.C:
			if _, err := c.Writer.WriteString(": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-snapshots.C:
			// The snapshot carries job records and platform aggregates —
			// exactly what the jobs list REST permission guards, so the
			// stream honors the same gate.
			if sub.CanReadJobs {
				writeSSE(c, flusher, a.buildSnapshot(ctx))
			}
		}
	}
}

// buildSnapshot merges the analytics summary and the newest jobs into one
// event scoped to this connection.
func (a *App) buildSnapshot(ctx context.Context) realtime.Event {
	jobs, _, err := a.services.Jobs.List(ctx, kernel.ListOptions{Mode: "offset", Limit: sseSnapshotJobPage})
	if err != nil {
		a.logger.Warn("snapshot jobs unavailable", "err", err)
		jobs = nil
	}
	return realtime.NewEvent(realtime.TypeSnapshot, snapshot{
		Summary: a.services.Analytics.Summary(ctx),
		Jobs:    jobs,
	})
}

// writeSSE emits one event in text/event-stream framing.
func writeSSE(c *gin.Context, flusher http.Flusher, ev realtime.Event) {
	// The wire view strips the routing scope; clients never see it.
	payload, err := json.Marshal(ev.Wire())
	if err != nil {
		return
	}
	// The whole event rides the data line as one JSON object carrying its
	// type, so EventSource.onmessage sees a single dispatchable message.
	if _, err := c.Writer.WriteString("data: " + string(payload) + "\n\n"); err != nil {
		return
	}
	flusher.Flush()
}
