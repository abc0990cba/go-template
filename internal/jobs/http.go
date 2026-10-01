// Jobs HTTP surface: the operations dashboard for background jobs plus the
// enrichment trigger and the stored enrichment trail. Listing requires the
// jobs:read permission (and is ETag'd because browsers poll it); running
// jobs requires jobs:execute; reading enrichments requires enrichments:read.
package jobs

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
)

// HTTP carries the jobs handlers' dependencies.
type HTTP struct {
	// Svc is the job service.
	Svc *Service
	// Logger records background-processing failures; nil logging is fine.
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
	// ETag adds strong ETag validation to heavy GET endpoints.
	ETag func() gin.HandlerFunc
}

// RegisterHTTP mounts the jobs routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/jobs", h.RequirePermission("jobs", "read"), h.ETag(), h.list)
	authorized.GET("/jobs/:id", h.RequirePermission("jobs", "read"), h.get)
	authorized.POST("/jobs/process", h.RequirePermission("jobs", "execute"), h.process)
	authorized.POST("/jobs/enrichment", h.RequirePermission("jobs", "execute"), h.enqueueEnrichment)
	authorized.GET("/enrichment-profiles", h.RequirePermission("enrichments", "read"), h.enrichmentProfiles)
}

// list returns one page of jobs, newest first.
func (h *HTTP) list(c *gin.Context) {
	listed, next, err := h.Svc.List(c.Request.Context(), httpx.ListOptions(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(listed), "next_cursor": next})
}

// get returns a job by id.
func (h *HTTP) get(c *gin.Context) {
	job, err := h.Svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

// process runs the oldest queued job in-process. Without a distributed
// queue this is how the demo moves jobs forward; with one, it is a no-op
// because the worker owns execution.
func (h *HTTP) process(c *gin.Context) {
	processed, err := h.Svc.ProcessOne(c.Request.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("job processing failed", "err", err)
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"processed": processed})
}

// enqueueEnrichment queues a user.enrich_batch job with the requested
// profile count.
func (h *HTTP) enqueueEnrichment(c *gin.Context) {
	var req struct {
		Count int `json:"count"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	payload, err := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: req.Count})
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	job, err := h.Svc.Enqueue(c.Request.Context(), TypeEnrichBatch, string(payload), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// enrichmentProfiles returns one page of stored enrichment profiles.
func (h *HTTP) enrichmentProfiles(c *gin.Context) {
	profiles, next, err := h.Svc.EnrichmentProfiles(c.Request.Context(), httpx.ListOptions(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(profiles), "next_cursor": next})
}
