// Audit HTTP helpers: capture the request context into immutable records
// and serve the admin trail listing.
package httpapp

import (
	"encoding/json"
	"net/http"

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
	diff := ""
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
	}
	entries, err := a.services.Audit.List(c.Request.Context(), opts)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries, "next_cursor": ""})
}
