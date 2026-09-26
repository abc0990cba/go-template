// Webhook HTTP surface: endpoint CRUD plus the delivery log. Events are
// matched and fanned out inside the analytics event path, not here.
package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
)

func (a *App) apiListWebhooks(c *gin.Context) {
	endpoints, err := a.services.Webhooks.List(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": endpoints, "next_cursor": ""})
}

func (a *App) apiCreateWebhook(c *gin.Context) {
	var req model.WebhookEndpoint
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	req.ID = ""
	req.CreatedBy = a.currentUser(c).ID
	endpoint, err := a.services.Webhooks.Create(c.Request.Context(), req)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "webhook.created", "webhook", endpoint.ID, nil, gin.H{"url": endpoint.URL, "events": endpoint.Events})
	// The secret is revealed once here; it never appears in listings.
	c.JSON(http.StatusCreated, gin.H{"id": endpoint.ID, "url": endpoint.URL, "secret": endpoint.Secret, "events": endpoint.Events, "active": endpoint.Active})
}

func (a *App) apiUpdateWebhook(c *gin.Context) {
	var req model.WebhookEndpoint
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	req.ID = c.Param("id")
	if err := a.services.Webhooks.Update(c.Request.Context(), req); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "webhook.updated", "webhook", req.ID, nil, gin.H{"url": req.URL, "events": req.Events, "active": req.Active})
	c.Status(http.StatusNoContent)
}

func (a *App) apiDeleteWebhook(c *gin.Context) {
	if err := a.services.Webhooks.Delete(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "webhook.deleted", "webhook", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiWebhookDeliveries(c *gin.Context) {
	deliveries, err := a.services.Webhooks.Deliveries(c.Request.Context(), c.Param("id"), intQuery(c, "limit", 25), 0)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": deliveries, "next_cursor": ""})
}

// apiTestWebhook fires one synthetic event through the normal fan-out so
// operators can verify a subscription without waiting for real traffic.
func (a *App) apiTestWebhook(c *gin.Context) {
	if err := a.services.Webhooks.SendTest(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}
