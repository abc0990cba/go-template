// Reports and analytics HTTP surface: snapshot reports on one side, the
// dashboard summary and the event-trail slices on the other. Every route is
// guarded by a catalog permission (reports/analytics); the summary is ETag'd
// because browsers poll it.

package analytics

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
)

// HTTP carries the reports/analytics handlers' dependencies.
type HTTP struct {
	Reports   *ReportService
	Analytics *Service
	// Jobs enqueues the report-generation job and returns the queued job
	// record for the 202 body. A func value rather than a typed port: the
	// jobs feature imports analytics for report persistence, so a typed
	// signature here would cycle the import graph.
	Jobs func(ctx context.Context, typ, payload, createdBy string) (any, error)
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
	// ETag adds strong ETag validation to heavy GET endpoints.
	ETag func() gin.HandlerFunc
}

// RegisterHTTP mounts the reports and analytics routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/reports", h.RequirePermission("reports", "read"), h.listReports)
	authorized.POST("/reports", h.RequirePermission("reports", "create"), h.generateReport)
	authorized.GET("/reports/:id", h.RequirePermission("reports", "read"), h.getReport)
	authorized.GET("/analytics/summary", h.RequirePermission("analytics", "read"), h.ETag(), h.summary)
	authorized.GET("/analytics/timeseries", h.RequirePermission("analytics", "read"), h.timeseries)
	authorized.GET("/analytics/events", h.RequirePermission("analytics", "read"), h.recentEvents)
	authorized.GET("/analytics/breakdown", h.RequirePermission("analytics", "read"), h.breakdown)
	authorized.GET("/analytics/dau", h.RequirePermission("analytics", "read"), h.dau)
}

// generateReport queues the background report-generation job.
func (h *HTTP) generateReport(c *gin.Context) {
	job, err := h.Jobs(c.Request.Context(), "user.report_generate", "{}", httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// listReports returns one page of stored reports, newest first.
func (h *HTTP) listReports(c *gin.Context) {
	reports, next, err := h.Reports.List(c.Request.Context(), httpx.ListOptions(c))
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(reports), "next_cursor": next})
}

// getReport returns one report by id.
func (h *HTTP) getReport(c *gin.Context) {
	report, err := h.Reports.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// summary serves the dashboard counters.
func (h *HTTP) summary(c *gin.Context) {
	c.JSON(http.StatusOK, h.Analytics.Summary(c.Request.Context()))
}

// timeseries serves daily event counts for the chart window (1..90 days).
func (h *HTTP) timeseries(c *gin.Context) {
	days := httpx.IntQuery(c, "days", 30)
	days = max(min(days, 90), 1)
	c.JSON(http.StatusOK, gin.H{
		"days": days,
		"data": httpx.ListOrEmpty(h.Analytics.Timeseries(c.Request.Context(), days)),
	})
}

// recentEvents serves the newest slice of the event trail (1..200).
func (h *HTTP) recentEvents(c *gin.Context) {
	limit := httpx.IntQuery(c, "limit", 50)
	limit = max(min(limit, 200), 1)
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(h.Analytics.RecentEvents(c.Request.Context(), limit))})
}

// breakdown groups events by a properties dimension.
func (h *HTTP) breakdown(c *gin.Context) {
	days := max(min(httpx.IntQuery(c, "days", 30), 90), 1)
	rows, err := h.Analytics.Breakdown(c.Request.Context(), c.DefaultQuery("by", "feature"), days)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": httpx.ListOrEmpty(rows)})
}

// dau serves distinct active users per day.
func (h *HTTP) dau(c *gin.Context) {
	days := max(min(httpx.IntQuery(c, "days", 30), 90), 1)
	rows, err := h.Analytics.DAU(c.Request.Context(), days)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": httpx.ListOrEmpty(rows)})
}
