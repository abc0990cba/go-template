// In-app notification endpoints: every route is self-service — a user only
// ever sees and mutates their own records.

package notifications

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the notification handlers' dependencies.
type HTTP struct {
	Svc *Service
}

// RegisterHTTP mounts the notification routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/notifications", h.list)
	authorized.GET("/notifications/unread-count", h.unreadCount)
	authorized.POST("/notifications/:id/read", h.markRead)
	authorized.POST("/notifications/read-all", h.markAllRead)
}

func (h *HTTP) list(c *gin.Context) {
	user := users.CurrentUser(c)
	limit := httpx.IntQuery(c, "limit", 25)
	limit = max(min(limit, 100), 1)
	offset := max(httpx.IntQuery(c, "offset", 0), 0)
	notifications, err := h.Svc.List(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(notifications), "next_cursor": ""})
}

func (h *HTTP) unreadCount(c *gin.Context) {
	user := users.CurrentUser(c)
	count, err := h.Svc.UnreadCount(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *HTTP) markRead(c *gin.Context) {
	user := users.CurrentUser(c)
	if err := h.Svc.MarkRead(c.Request.Context(), user.ID, c.Param("id")); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTP) markAllRead(c *gin.Context) {
	user := users.CurrentUser(c)
	if err := h.Svc.MarkAllRead(c.Request.Context(), user.ID); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.Status(http.StatusNoContent)
}
