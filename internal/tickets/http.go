// Tickets HTTP surface: the requester's listings and lifecycle actions, the
// approver's inbox and decision history, comments, and timeline events.
// Ticket-type administration lives in http_types.go; the attachments list
// is served by the files feature (GET /tickets/:id/attachments).

package tickets

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the ticket handlers' dependencies.
type HTTP struct {
	// Svc runs the approval state machine.
	Svc *Service
	// RBAC answers the scope=all org-wide read questions (tickets:read_all).
	RBAC   *rbac.Service
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
	// ETag adds strong ETag validation to heavy GET endpoints.
	ETag func() gin.HandlerFunc
	// Idempotency turns client retries of POST /tickets into replays.
	Idempotency func() gin.HandlerFunc
}

// RegisterHTTP mounts the ticket routes on the authorized group. Ownership
// and step-role checks live in the service; these guards keep the catalog
// permissions. Filing a ticket accepts an Idempotency-Key so client retries
// replay instead of double-filing.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/tickets", h.RequirePermission("tickets", "read"), h.ETag(), h.list)
	authorized.GET("/tickets/counts", h.RequirePermission("tickets", "read"), h.counts)
	authorized.POST("/tickets", h.Idempotency(), h.RequirePermission("tickets", "create"), h.create)
	authorized.GET("/tickets/:id", h.RequirePermission("tickets", "read"), h.get)
	authorized.PATCH("/tickets/:id", h.RequirePermission("tickets", "update"), h.update)
	authorized.DELETE("/tickets/:id", h.RequirePermission("tickets", "delete"), h.remove)
	authorized.POST("/tickets/:id/actions", h.RequirePermission("tickets", "read"), h.action)
	authorized.GET("/tickets/:id/comments", h.RequirePermission("tickets", "read"), h.comments)
	authorized.POST("/tickets/:id/comments", h.RequirePermission("tickets", "read"), h.addComment)
	authorized.GET("/tickets/:id/events", h.RequirePermission("tickets", "read"), h.events)
	authorized.GET("/approvals", h.RequirePermission("tickets", "approve"), h.approvals)

	authorized.GET("/ticket-types", h.RequirePermission("tickets", "read"), h.listTypes)
	authorized.POST("/ticket-types", h.RequirePermission("ticket_types", "create"), h.createType)
	authorized.PATCH("/ticket-types/:id", h.RequirePermission("ticket_types", "update"), h.updateType)
	authorized.DELETE("/ticket-types/:id", h.RequirePermission("ticket_types", "delete"), h.deleteType)
}

// actor identifies the caller for the ticket guards: user id plus the
// active Casbin domain.
func (h *HTTP) actor(c *gin.Context) Actor {
	return Actor{ID: users.CurrentUser(c).ID, Domain: httpx.Domain(c)}
}

// fail maps this feature's transition sentinel before the shared mapping:
// an invalid transition is a conflict, not a server error.
func (h *HTTP) fail(c *gin.Context, err error) {
	if errors.Is(err, ErrInvalidTransition) {
		httpx.Problem(c, http.StatusConflict, "invalid transition", err)
		return
	}
	httpx.Fail(c, h.Logger, err)
}

// forbidden is 403 problem+json, never a redirect.
func forbidden(c *gin.Context) {
	httpx.Problem(c, http.StatusForbidden, "forbidden", nil)
	c.Abort()
}

// list serves scope=my (the requester's own tickets, the default) and
// scope=all (org-wide, requires tickets:read_all).
func (h *HTTP) list(c *gin.Context) {
	user := users.CurrentUser(c)
	opts := kernel.TicketOptions{
		ListOptions: httpx.ListOptions(c),
		OrgID:       httpx.Domain(c),
		Priority:    c.Query("priority"),
	}
	switch c.DefaultQuery("scope", "my") {
	case "all":
		if !h.RBAC.Enforce(user.ID, httpx.Domain(c), "tickets", "read_all") {
			forbidden(c)
			return
		}
	default:
		opts.CreatedBy = user.ID
	}
	tickets, next, err := h.Svc.List(c.Request.Context(), opts)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(tickets), "next_cursor": next})
}

// counts serves the per-status counts for the page's chips, scoped like the
// list: own tickets unless tickets:read_all allows org-wide.
func (h *HTTP) counts(c *gin.Context) {
	user := users.CurrentUser(c)
	orgID := httpx.Domain(c)
	createdBy := user.ID
	if c.DefaultQuery("scope", "my") == "all" {
		if !h.RBAC.Enforce(user.ID, orgID, "tickets", "read_all") {
			forbidden(c)
			return
		}
		createdBy = ""
	}
	counts := h.Svc.StatusCounts(c.Request.Context(), orgID, createdBy)
	c.JSON(http.StatusOK, counts)
}

func (h *HTTP) create(c *gin.Context) {
	var req struct {
		TypeID      string `json:"type_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	ticket, err := h.Svc.Create(c.Request.Context(), h.actor(c), req.TypeID, req.Title, req.Description, req.Priority)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket.created", "ticket", ticket.ID, nil, ticket)
	c.JSON(http.StatusCreated, ticket)
}

func (h *HTTP) get(c *gin.Context) {
	ticket, approvals, err := h.Svc.Get(c.Request.Context(), h.actor(c), c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticket, "approvals": approvals})
}

func (h *HTTP) update(c *gin.Context) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	ticket, err := h.Svc.Update(c.Request.Context(), h.actor(c), c.Param("id"), req.Title, req.Description, req.Priority)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket.updated", "ticket", ticket.ID, nil, ticket)
	c.JSON(http.StatusOK, ticket)
}

func (h *HTTP) remove(c *gin.Context) {
	if err := h.Svc.Delete(c.Request.Context(), h.actor(c), c.Param("id")); err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket.deleted", "ticket", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

// action applies one transition; the service enforces authorship and
// step-role guards, invalid transitions arrive as 409.
func (h *HTTP) action(c *gin.Context) {
	var req struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	ticket, err := h.Svc.Action(c.Request.Context(), h.actor(c), c.Param("id"), req.Action, req.Comment)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket."+req.Action, "ticket", ticket.ID, nil, gin.H{"status": ticket.Status, "comment": req.Comment})
	c.JSON(http.StatusOK, ticket)
}

func (h *HTTP) comments(c *gin.Context) {
	comments, err := h.Svc.Comments(c.Request.Context(), h.actor(c), c.Param("id"), httpx.IntQuery(c, "limit", 25), httpx.IntQuery(c, "offset", 0))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(comments)})
}

func (h *HTTP) addComment(c *gin.Context) {
	var req struct {
		Body string `json:"body"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	comment, err := h.Svc.AddComment(c.Request.Context(), h.actor(c), c.Param("id"), req.Body)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket.commented", "ticket", c.Param("id"), nil, gin.H{"comment_id": comment.ID})
	c.JSON(http.StatusCreated, comment)
}

func (h *HTTP) events(c *gin.Context) {
	events, err := h.Svc.Events(c.Request.Context(), h.actor(c), c.Param("id"), httpx.IntQuery(c, "limit", 50), httpx.IntQuery(c, "offset", 0))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(events)})
}

// approvals serves the approver's two views: state=pending is the inbox
// of tickets waiting on the caller's roles, state=decided the decision
// history.
func (h *HTTP) approvals(c *gin.Context) {
	actor := h.actor(c)
	if c.DefaultQuery("state", "pending") == "decided" {
		decisions, err := h.Svc.Decisions(c.Request.Context(), actor, httpx.IntQuery(c, "limit", 25), httpx.IntQuery(c, "offset", 0))
		if err != nil {
			h.fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(decisions)})
		return
	}
	tickets, next, err := h.Svc.Inbox(c.Request.Context(), actor, httpx.ListOptions(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(tickets), "next_cursor": next})
}
