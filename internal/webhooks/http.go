// Webhook HTTP surface: endpoint CRUD plus the delivery log. Events are
// matched and fanned out inside the analytics event path, not here.
package webhooks

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the webhook handlers' dependencies.
type HTTP struct {
	Svc    *Service
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the webhook routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/webhooks", h.RequirePermission("webhooks", "read"), h.list)
	authorized.POST("/webhooks", h.RequirePermission("webhooks", "create"), h.create)
	authorized.PUT("/webhooks/:id", h.RequirePermission("webhooks", "update"), h.update)
	authorized.DELETE("/webhooks/:id", h.RequirePermission("webhooks", "delete"), h.remove)
	authorized.GET("/webhooks/:id/deliveries", h.RequirePermission("webhooks", "read"), h.deliveries)
	authorized.POST("/webhooks/:id/test", h.RequirePermission("webhooks", "execute"), h.test)
}

// list returns every endpoint.
func (h *HTTP) list(c *gin.Context) {
	endpoints, err := h.Svc.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(endpoints), "next_cursor": ""})
}

// create adds an endpoint; the generated secret is revealed once here and
// never appears in listings.
func (h *HTTP) create(c *gin.Context) {
	var req Endpoint
	if !httpx.Bind(c, &req) {
		return
	}
	req.ID = ""
	req.CreatedBy = users.CurrentUser(c).ID
	endpoint, err := h.Svc.Create(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "webhook.created", "webhook", endpoint.ID, nil, gin.H{"url": endpoint.URL, "events": endpoint.Events})
	c.JSON(http.StatusCreated, gin.H{"id": endpoint.ID, "url": endpoint.URL, "secret": endpoint.Secret, "events": endpoint.Events, "active": endpoint.Active})
}

// update rewrites url/events/description/active.
func (h *HTTP) update(c *gin.Context) {
	var req Endpoint
	if !httpx.Bind(c, &req) {
		return
	}
	req.ID = c.Param("id")
	if err := h.Svc.Update(c.Request.Context(), req); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "webhook.updated", "webhook", req.ID, nil, gin.H{"url": req.URL, "events": req.Events, "active": req.Active})
	c.Status(http.StatusNoContent)
}

// remove deletes the endpoint; deliveries cascade.
func (h *HTTP) remove(c *gin.Context) {
	if err := h.Svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "webhook.deleted", "webhook", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

// deliveries pages one endpoint's delivery log.
func (h *HTTP) deliveries(c *gin.Context) {
	rows, err := h.Svc.Deliveries(c.Request.Context(), c.Param("id"), httpx.IntQuery(c, "limit", 25), 0)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(rows), "next_cursor": ""})
}

// test fires one synthetic event through the normal fan-out so operators
// can verify a subscription without waiting for real traffic.
func (h *HTTP) test(c *gin.Context) {
	if err := h.Svc.SendTest(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}
