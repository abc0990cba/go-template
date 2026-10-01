// Organization and membership HTTP handlers. The org invitations admin
// routes (invite/list/revoke) live with the invitations feature; the
// self-service routes (mine / accept) moved there too.
package rbac

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
)

func (h *HTTP) orgs(c *gin.Context) {
	orgs, next, err := h.Svc.ListOrgs(c.Request.Context(), httpx.ListOptions(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(orgs), "next_cursor": next})
}

func (h *HTTP) createOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	org, err := h.Svc.CreateOrg(c.Request.Context(), req.Name, req.Slug)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.created", "org", org.ID, nil, org)
	c.JSON(http.StatusCreated, org)
}

func (h *HTTP) getOrg(c *gin.Context) {
	org, err := h.Svc.GetOrg(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, org)
}

func (h *HTTP) updateOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.UpdateOrg(c.Request.Context(), c.Param("id"), req.Name, req.Slug); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.updated", "org", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) deleteOrg(c *gin.Context) {
	if err := h.Svc.DeleteOrg(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.deleted", "org", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) orgMembers(c *gin.Context) {
	members, err := h.Svc.OrgMembers(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(members)})
}

func (h *HTTP) addOrgMember(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.AddOrgMember(c.Request.Context(), c.Param("id"), req.UserID); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.member_added", "org", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) removeOrgMember(c *gin.Context) {
	if err := h.Svc.RemoveOrgMember(c.Request.Context(), c.Param("id"), c.Param("userId")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.member_removed", "org", c.Param("id"), nil, gin.H{"user_id": c.Param("userId")})
	c.Status(http.StatusNoContent)
}
