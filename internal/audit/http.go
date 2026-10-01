// Audit HTTP surface: the request-context capture helper every mutating
// handler uses, and the admin trail listing.
package audit

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the audit handlers' dependencies.
type HTTP struct {
	Svc *Service
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the audit routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/audit", h.RequirePermission("audit", "read"), h.list)
	authorized.GET("/users/:id/activity", h.RequirePermission("audit", "read"), h.userActivity)
}

// Record captures the request context (caller, active domain, client IP,
// request id) into one immutable entry. before/after (either may be nil)
// are marshalled into the diff column. Write failures never fail the
// request.
func Record(s *Service, c *gin.Context, action, resourceType, resourceID string, before, after any) {
	user, _ := users.CurrentUserOK(c)
	diff := "{}" // jsonb column: an empty string is not valid JSON
	if before != nil || after != nil {
		if raw, err := json.Marshal(map[string]any{"before": before, "after": after}); err == nil {
			diff = string(raw)
		}
	}
	s.Record(c.Request.Context(), Entry{
		ActorID: user.ID, ActorEmail: user.Email,
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		Domain: httpx.Domain(c), Diff: diff,
		IP: c.ClientIP(), RequestID: c.GetString("request_id"),
	})
}

// list pages the audit trail newest first with optional filters.
func (h *HTTP) list(c *gin.Context) {
	opts := kernel.AuditOptions{
		ListOptions: httpx.ListOptions(c),
		Actor:       c.Query("actor"),
		Action:      c.Query("action"),
		Resource:    c.Query("resource"),
		ResourceID:  c.Query("resource_id"),
	}
	entries, err := h.Svc.List(c.Request.Context(), opts)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(entries), "next_cursor": ""})
}

// userActivity serves one account's history card: audit records where the
// user is the actor or the subject, newest first. next_cursor carries the
// next offset, so the card's feed can keep loading while more records exist.
func (h *HTTP) userActivity(c *gin.Context) {
	limit := httpx.IntQuery(c, "limit", 25)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	offset := max(httpx.IntQuery(c, "offset", 0), 0)
	entries, err := h.Svc.List(c.Request.Context(), kernel.AuditOptions{
		ListOptions: kernel.ListOptions{Limit: limit, Offset: offset},
		Subject:     c.Param("id"),
	})
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	next := ""
	if len(entries) == limit {
		next = strconv.Itoa(offset + len(entries))
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(entries), "next_cursor": next})
}
