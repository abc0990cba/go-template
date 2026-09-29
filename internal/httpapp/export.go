package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// apiRequestExport queues the caller's GDPR data export (202 with the job;
// 409 when one is already in flight).
func (a *App) apiRequestExport(c *gin.Context) {
	user := a.currentUser(c)
	job, err := a.services.Jobs.RequestExport(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "export.requested", "user", user.ID, nil, gin.H{"job_id": job.ID})
	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": job.Status})
}

// apiExportStatus reports the caller's latest export job.
func (a *App) apiExportStatus(c *gin.Context) {
	user := a.currentUser(c)
	job, err := a.services.Jobs.LatestExport(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"job_id": job.ID, "status": job.Status, "error": job.Error, "updated_at": job.UpdatedAt})
}

// apiExportDownload streams the latest finished bundle. While the export is
// still running it answers 202 with Retry-After so clients can poll.
func (a *App) apiExportDownload(c *gin.Context) {
	user := a.currentUser(c)
	file, stream, err := a.services.Exports.Open(c.Request.Context(), user.ID)
	if err != nil {
		// Distinguish "not ready yet" from "nothing requested".
		job, jobErr := a.services.Jobs.LatestExport(c.Request.Context(), user.ID)
		if jobErr == nil && (job.Status == "queued" || job.Status == "running") {
			c.Header("Retry-After", "3")
			c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": job.Status})
			return
		}
		a.fail(c, err)
		return
	}
	defer func() { _ = stream.Close() }()
	a.audit(c, "export.downloaded", "user", user.ID, nil, gin.H{"file": file.ID})
	c.Header("Content-Disposition", "attachment; filename=\""+file.Filename+"\"")
	c.DataFromReader(http.StatusOK, file.SizeBytes, "application/zip", stream, nil)
}
