// Audit HTTP helpers: capture the request context into immutable records
// and serve the admin trail listing.
package httpapp

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// auditActor returns the resolved caller, or the zero user on public
// routes (register/login) where no one is authenticated yet.
func (a *App) auditActor(c *gin.Context) model.User {
	if v, ok := c.Get("user"); ok {
		if user, ok := v.(model.User); ok {
			return user
		}
	}
	return model.User{}
}

// audit records an audit entry stamped with the caller, the active domain,
// the client IP, and the request id. before/after (either may be nil) are
// marshalled into the diff column. Write failures never fail the request.
func (a *App) audit(c *gin.Context, action, resourceType, resourceID string, before, after any) {
	user := a.auditActor(c)
	diff := "{}" // jsonb column: an empty string is not valid JSON
	if before != nil || after != nil {
		if raw, err := json.Marshal(map[string]any{"before": before, "after": after}); err == nil {
			diff = string(raw)
		}
	}
	a.services.Audit.Record(c.Request.Context(), model.AuditEntry{
		ActorID: user.ID, ActorEmail: user.Email,
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		Domain: a.requestDomain(c), Diff: diff,
		IP: c.ClientIP(), RequestID: c.GetString("request_id"),
	})
}

// apiAudit pages the audit trail newest first with optional filters.
func (a *App) apiAudit(c *gin.Context) {
	opts := repository.AuditOptions{
		ListOptions: listOptions(c),
		Actor:       c.Query("actor"),
		Action:      c.Query("action"),
		Resource:    c.Query("resource"),
		ResourceID:  c.Query("resource_id"),
	}
	entries, err := a.services.Audit.List(c.Request.Context(), opts)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(entries), "next_cursor": ""})
}

// apiUserActivity serves one account's history card: audit records where the
// user is the actor or the subject, newest first. next_cursor carries the
// next offset, so the card's feed can keep loading while more records exist.
func (a *App) apiUserActivity(c *gin.Context) {
	limit := intQuery(c, "limit", 25)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	offset := max(intQuery(c, "offset", 0), 0)
	entries, err := a.services.Audit.List(c.Request.Context(), repository.AuditOptions{
		ListOptions: repository.ListOptions{Limit: limit, Offset: offset},
		Subject:     c.Param("id"),
	})
	if err != nil {
		a.fail(c, err)
		return
	}
	next := ""
	if len(entries) == limit {
		next = strconv.Itoa(offset + len(entries))
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(entries), "next_cursor": next})
}
