// In-app notification endpoints: every route is self-service — a user only
// ever sees and mutates their own records.
package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) apiNotifications(c *gin.Context) {
	user := a.currentUser(c)
	limit := intQuery(c, "limit", 25)
	limit = max(min(limit, 100), 1)
	offset := max(intQuery(c, "offset", 0), 0)
	notifications, err := a.services.Notifications.List(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(notifications), "next_cursor": ""})
}

func (a *App) apiUnreadCount(c *gin.Context) {
	user := a.currentUser(c)
	count, err := a.services.Notifications.UnreadCount(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (a *App) apiMarkNotificationRead(c *gin.Context) {
	user := a.currentUser(c)
	if err := a.services.Notifications.MarkRead(c.Request.Context(), user.ID, c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *App) apiMarkAllNotificationsRead(c *gin.Context) {
	user := a.currentUser(c)
	if err := a.services.Notifications.MarkAllRead(c.Request.Context(), user.ID); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
