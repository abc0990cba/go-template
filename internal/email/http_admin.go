package email

// The delivery-log admin surface: read access for answering "did the mail
// go out?", and a retry for messages the queue gave up on. Split from
// http.go (the public flows) to keep both within the size budget.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"log/slog"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
)

// AdminHTTP carries the delivery-log handlers' dependencies.
type AdminHTTP struct {
	Svc    *Service
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterAdminHTTP mounts the delivery-log routes on the authorized group.
func RegisterAdminHTTP(authorized *gin.RouterGroup, h AdminHTTP) {
	authorized.GET("/emails", h.RequirePermission("emails", "read"), h.listMessages)
	authorized.GET("/emails/:id", h.RequirePermission("emails", "read"), h.getMessage)
	authorized.POST("/emails/:id/retry", h.RequirePermission("emails", "execute"), h.retryMessage)
}

func (h *AdminHTTP) listMessages(c *gin.Context) {
	limit := httpx.IntQuery(c, "limit", 25)
	offset := httpx.IntQuery(c, "offset", 0)
	messages, err := h.Svc.Messages(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":        httpx.ListOrEmpty(messages),
		"next_cursor": httpx.NextOffsetCursor(offset, len(messages)),
	})
}

func (h *AdminHTTP) getMessage(c *gin.Context) {
	message, err := h.Svc.GetMessage(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

// retryMessage re-queues a failed or dead message for delivery. An inline
// attempt's send failure is the row's business (status + last_error on the
// delivery log), not this request's: the answer is 200 with the row either
// way.
func (h *AdminHTTP) retryMessage(c *gin.Context) {
	message, retryErr := h.Svc.RetryMessage(c.Request.Context(), c.Param("id"))
	if retryErr != nil {
		if errors.Is(retryErr, kernel.ErrConflict) {
			httpx.Problem(c, http.StatusConflict, "message was already delivered", retryErr)
			return
		}
		httpx.Fail(c, h.Logger, retryErr)
		return
	}
	audit.Record(h.Audit, c, "email.retried", "email_message", message.ID, nil, gin.H{
		"to": message.To, "kind": message.Kind,
	})
	c.JSON(http.StatusOK, message)
}
