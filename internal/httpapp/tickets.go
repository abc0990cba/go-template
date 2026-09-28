package httpapp

// Tickets HTTP surface: the requester's listings and lifecycle actions, the
// approver's inbox and decision history, comments, timeline events, and
// ticket-type administration.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// ticketActor identifies the caller for the ticket guards: user id plus the
// active Casbin domain.
func (a *App) ticketActor(c *gin.Context) service.Actor {
	return service.Actor{ID: a.currentUser(c).ID, Domain: a.requestDomain(c)}
}

// apiListTickets serves scope=my (the requester's own tickets, the default)
// and scope=all (org-wide, requires tickets:read_all).
func (a *App) apiListTickets(c *gin.Context) {
	user := a.currentUser(c)
	opts := repository.TicketOptions{
		ListOptions: listOptions(c),
		OrgID:       a.requestDomain(c),
		Priority:    c.Query("priority"),
	}
	switch c.DefaultQuery("scope", "my") {
	case "all":
		if !a.services.RBAC.Enforce(user.ID, a.requestDomain(c), "tickets", "read_all") {
			a.forbidden(c)
			return
		}
	default:
		opts.CreatedBy = user.ID
	}
	tickets, next, err := a.services.Tickets.List(c.Request.Context(), opts)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(tickets), "next_cursor": next})
}

// apiTicketCounts serves the per-status counts for the page's chips, scoped
// like the list: own tickets unless tickets:read_all allows org-wide.
func (a *App) apiTicketCounts(c *gin.Context) {
	user := a.currentUser(c)
	orgID := a.requestDomain(c)
	createdBy := user.ID
	if c.DefaultQuery("scope", "my") == "all" {
		if !a.services.RBAC.Enforce(user.ID, orgID, "tickets", "read_all") {
			a.forbidden(c)
			return
		}
		createdBy = ""
	}
	counts := a.services.Tickets.StatusCounts(c.Request.Context(), orgID, createdBy)
	c.JSON(http.StatusOK, counts)
}

func (a *App) apiCreateTicket(c *gin.Context) {
	var req struct {
		TypeID      string `json:"type_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	ticket, err := a.services.Tickets.Create(c.Request.Context(), a.ticketActor(c), req.TypeID, req.Title, req.Description, req.Priority)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket.created", "ticket", ticket.ID, nil, ticket)
	c.JSON(http.StatusCreated, ticket)
}

func (a *App) apiGetTicket(c *gin.Context) {
	ticket, approvals, err := a.services.Tickets.Get(c.Request.Context(), a.ticketActor(c), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticket, "approvals": approvals})
}

func (a *App) apiUpdateTicket(c *gin.Context) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	ticket, err := a.services.Tickets.Update(c.Request.Context(), a.ticketActor(c), c.Param("id"), req.Title, req.Description, req.Priority)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket.updated", "ticket", ticket.ID, nil, ticket)
	c.JSON(http.StatusOK, ticket)
}

func (a *App) apiDeleteTicket(c *gin.Context) {
	if err := a.services.Tickets.Delete(c.Request.Context(), a.ticketActor(c), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket.deleted", "ticket", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

// apiTicketAction applies one transition; the service enforces authorship
// and step-role guards, invalid transitions arrive as 409.
func (a *App) apiTicketAction(c *gin.Context) {
	var req struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	ticket, err := a.services.Tickets.Action(c.Request.Context(), a.ticketActor(c), c.Param("id"), req.Action, req.Comment)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket."+req.Action, "ticket", ticket.ID, nil, gin.H{"status": ticket.Status, "comment": req.Comment})
	c.JSON(http.StatusOK, ticket)
}

func (a *App) apiTicketComments(c *gin.Context) {
	comments, err := a.services.Tickets.Comments(c.Request.Context(), a.ticketActor(c), c.Param("id"), intQuery(c, "limit", 25), intQuery(c, "offset", 0))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(comments)})
}

func (a *App) apiAddTicketComment(c *gin.Context) {
	var req struct {
		Body string `json:"body"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	comment, err := a.services.Tickets.AddComment(c.Request.Context(), a.ticketActor(c), c.Param("id"), req.Body)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket.commented", "ticket", c.Param("id"), nil, gin.H{"comment_id": comment.ID})
	c.JSON(http.StatusCreated, comment)
}

func (a *App) apiTicketEvents(c *gin.Context) {
	events, err := a.services.Tickets.Events(c.Request.Context(), a.ticketActor(c), c.Param("id"), intQuery(c, "limit", 50), intQuery(c, "offset", 0))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(events)})
}

// apiApprovals serves the approver's two views: state=pending is the inbox
// of tickets waiting on the caller's roles, state=decided the decision
// history.
func (a *App) apiApprovals(c *gin.Context) {
	actor := a.ticketActor(c)
	if c.DefaultQuery("state", "pending") == "decided" {
		decisions, err := a.services.Tickets.Decisions(c.Request.Context(), actor, intQuery(c, "limit", 25), intQuery(c, "offset", 0))
		if err != nil {
			a.fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(decisions)})
		return
	}
	tickets, next, err := a.services.Tickets.Inbox(c.Request.Context(), actor, listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(tickets), "next_cursor": next})
}

// --- Ticket types -------------------------------------------------------

func (a *App) apiTicketTypes(c *gin.Context) {
	types, err := a.services.Tickets.TicketTypes(c.Request.Context(), a.requestDomain(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(types)})
}

func (a *App) apiCreateTicketType(c *gin.Context) {
	var req struct {
		Key         string             `json:"key"`
		Name        string             `json:"name"`
		Description string             `json:"description"`
		Steps       []model.TicketStep `json:"steps"`
		Active      *bool              `json:"active"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	typ := model.TicketType{
		Key: req.Key, Name: req.Name, Description: req.Description,
		Steps: req.Steps, Active: req.Active == nil || *req.Active,
	}
	created, err := a.services.Tickets.CreateTicketType(c.Request.Context(), a.ticketActor(c), typ)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket_type.created", "ticket_type", created.ID, nil, created)
	c.JSON(http.StatusCreated, created)
}

func (a *App) apiUpdateTicketType(c *gin.Context) {
	var req struct {
		Name        string             `json:"name"`
		Description string             `json:"description"`
		Steps       []model.TicketStep `json:"steps"`
		Active      *bool              `json:"active"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	typ, err := a.services.Tickets.UpdateTicketType(c.Request.Context(), c.Param("id"), req.Name, req.Description, req.Steps, req.Active)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket_type.updated", "ticket_type", typ.ID, nil, typ)
	c.JSON(http.StatusOK, typ)
}

func (a *App) apiDeleteTicketType(c *gin.Context) {
	if err := a.services.Tickets.DeleteTicketType(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "ticket_type.deleted", "ticket_type", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}
