// Export HTTP surface: a user requests their GDPR data bundle, polls the
// generating job, and downloads the finished ZIP. All three routes are
// me-scoped — a caller can only ever reach their own bundle.
package export

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the export handlers' dependencies. Jobs is the job service:
// requesting an export enqueues a job, and status/download poll it.
type HTTP struct {
	Svc    *Service
	Jobs   *jobs.Service
	Audit  *audit.Service
	Logger *slog.Logger
}

// RegisterHTTP mounts the export routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.POST("/users/me/export", h.request)
	authorized.GET("/users/me/export", h.status)
	authorized.GET("/users/me/export/download", h.download)
}

// request queues the caller's GDPR data export (202 with the job;
// 409 when one is already in flight).
func (h *HTTP) request(c *gin.Context) {
	user := users.CurrentUser(c)
	job, err := h.Jobs.RequestExport(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "export.requested", "user", user.ID, nil, gin.H{"job_id": job.ID})
	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": job.Status})
}

// status reports the caller's latest export job.
func (h *HTTP) status(c *gin.Context) {
	user := users.CurrentUser(c)
	job, err := h.Jobs.LatestExport(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"job_id": job.ID, "status": job.Status, "error": job.Error, "updated_at": job.UpdatedAt})
}

// download streams the latest finished bundle. While the export is
// still running it answers 202 with Retry-After so clients can poll.
func (h *HTTP) download(c *gin.Context) {
	user := users.CurrentUser(c)
	file, stream, err := h.Svc.Open(c.Request.Context(), user.ID)
	if err != nil {
		// Distinguish "not ready yet" from "nothing requested".
		job, jobErr := h.Jobs.LatestExport(c.Request.Context(), user.ID)
		if jobErr == nil && (job.Status == "queued" || job.Status == "running") {
			c.Header("Retry-After", "3")
			c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": job.Status})
			return
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	defer func() { _ = stream.Close() }()
	audit.Record(h.Audit, c, "export.downloaded", "user", user.ID, nil, gin.H{"file": file.ID})
	c.Header("Content-Disposition", "attachment; filename=\""+file.Filename+"\"")
	c.DataFromReader(http.StatusOK, file.SizeBytes, "application/zip", stream, nil)
}
