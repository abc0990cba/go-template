package tickets

// Ticket-type administration HTTP surface (split out of http.go to keep the
// files within budget).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
)

func (h *HTTP) listTypes(c *gin.Context) {
	types, err := h.Svc.TicketTypes(c.Request.Context(), httpx.Domain(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(types)})
}

func (h *HTTP) createType(c *gin.Context) {
	var req struct {
		Key         string       `json:"key"`
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Steps       []TicketStep `json:"steps"`
		Active      *bool        `json:"active"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	typ := TicketType{
		Key: req.Key, Name: req.Name, Description: req.Description,
		Steps: req.Steps, Active: req.Active == nil || *req.Active,
	}
	created, err := h.Svc.CreateTicketType(c.Request.Context(), h.actor(c), typ)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket_type.created", "ticket_type", created.ID, nil, created)
	c.JSON(http.StatusCreated, created)
}

func (h *HTTP) updateType(c *gin.Context) {
	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Steps       []TicketStep `json:"steps"`
		Active      *bool        `json:"active"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	typ, err := h.Svc.UpdateTicketType(c.Request.Context(), c.Param("id"), req.Name, req.Description, req.Steps, req.Active)
	if err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket_type.updated", "ticket_type", typ.ID, nil, typ)
	c.JSON(http.StatusOK, typ)
}

func (h *HTTP) deleteType(c *gin.Context) {
	if err := h.Svc.DeleteTicketType(c.Request.Context(), c.Param("id")); err != nil {
		h.fail(c, err)
		return
	}
	audit.Record(h.Audit, c, "ticket_type.deleted", "ticket_type", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}
