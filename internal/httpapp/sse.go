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
	Summary analytics.AnalyticsSummary `json:"summary"`
	Jobs    []jobs.Job                 `json:"jobs"`
}

// sseStream pushes analytics events, job transitions, and periodic
// snapshots to the browser. Auth happens in the apiAuth middleware; the
// stream itself is anonymous bytes on an authenticated connection.
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

	events, cancel := a.hub.Subscribe()
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
			writeSSE(c, flusher, a.buildSnapshot(ctx))
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
	payload, err := json.Marshal(ev)
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
