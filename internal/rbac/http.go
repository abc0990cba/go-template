// RBAC HTTP surface: the role, policy, catalog, and user-role assignment
// handlers the admin console talks to. The domain middleware and the
// permission guard stay in the assembly (httpapp) — they are shared glue,
// not feature surface.

package rbac

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
)

// HTTP carries the RBAC handlers' dependencies.
type HTTP struct {
	Svc    *Service
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the RBAC routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/rbac/catalog", h.catalog)
	authorized.GET("/roles", h.RequirePermission("roles", "read"), h.roles)
	authorized.POST("/roles", h.RequirePermission("roles", "create"), h.createRole)
	authorized.PUT("/roles/:key", h.RequirePermission("roles", "update"), h.updateRole)
	authorized.DELETE("/roles/:key", h.RequirePermission("roles", "delete"), h.deleteRole)
	authorized.GET("/roles/:key/permissions", h.RequirePermission("roles", "read"), h.rolePermissions)
	authorized.PUT("/roles/:key/permissions", h.RequirePermission("roles", "update"), h.setRolePermissions)
	authorized.GET("/policies", h.RequirePermission("policies", "read"), h.policies)
	authorized.POST("/policies", h.RequirePermission("policies", "create"), h.addPolicy)
	authorized.DELETE("/policies", h.RequirePermission("policies", "delete"), h.removePolicy)
	authorized.GET("/users/:id/roles", h.RequirePermission("users", "read"), h.userRoles)
	authorized.POST("/users/:id/roles", h.RequirePermission("users", "update"), h.assignUserRole)
	authorized.DELETE("/users/:id/roles", h.RequirePermission("users", "update"), h.unassignUserRole)
	authorized.GET("/orgs", h.RequirePermission("orgs", "read"), h.orgs)
	authorized.POST("/orgs", h.RequirePermission("orgs", "create"), h.createOrg)
	authorized.GET("/orgs/:id", h.RequirePermission("orgs", "read"), h.getOrg)
	authorized.PUT("/orgs/:id", h.RequirePermission("orgs", "update"), h.updateOrg)
	authorized.DELETE("/orgs/:id", h.RequirePermission("orgs", "delete"), h.deleteOrg)
	authorized.GET("/orgs/:id/members", h.RequirePermission("orgs", "read"), h.orgMembers)
	authorized.POST("/orgs/:id/members", h.RequirePermission("orgs", "update"), h.addOrgMember)
	authorized.DELETE("/orgs/:id/members/:userId", h.RequirePermission("orgs", "update"), h.removeOrgMember)
}

// catalog serves the resource/action matrix for the roles page.
func (h *HTTP) catalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"resources": h.Svc.Catalog()})
}

func (h *HTTP) roles(c *gin.Context) {
	roles, err := h.Svc.Roles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(roles)})
}

func (h *HTTP) createRole(c *gin.Context) {
	var req struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	role, err := h.Svc.CreateRole(c.Request.Context(), req.Key, req.Name, req.Description)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "role.created", "role", role.Key, nil, role)
	c.JSON(http.StatusCreated, role)
}

func (h *HTTP) updateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.UpdateRole(c.Request.Context(), c.Param("key"), req.Name, req.Description); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "role.updated", "role", c.Param("key"), nil, req)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) deleteRole(c *gin.Context) {
	if err := h.Svc.DeleteRole(c.Request.Context(), c.Param("key")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "role.deleted", "role", c.Param("key"), nil, nil)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) rolePermissions(c *gin.Context) {
	rules, err := h.Svc.RolePermissions(c.Param("key"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(rules)})
}

func (h *HTTP) setRolePermissions(c *gin.Context) {
	var req struct {
		Domain      string       `json:"domain"`
		Permissions []Permission `json:"permissions"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.SetRolePermissions(c.Param("key"), req.Domain, req.Permissions); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "role.permissions_updated", "role", c.Param("key"), nil, req)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) policies(c *gin.Context) {
	ptype := c.DefaultQuery("ptype", "p")
	rules, err := h.Svc.ListPolicies(c.Request.Context(), ptype)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(rules)})
}

func (h *HTTP) addPolicy(c *gin.Context) {
	var req PolicyRule
	if !httpx.Bind(c, &req) {
		return
	}
	req.PType = c.DefaultQuery("ptype", req.PType)
	if err := h.Svc.AddPolicyRule(req); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "policy.added", "policy", req.Subject, nil, req)
	c.Status(http.StatusCreated)
}

func (h *HTTP) removePolicy(c *gin.Context) {
	var req PolicyRule
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.RemovePolicyRule(req); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "policy.removed", "policy", req.Subject, req, nil)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) userRoles(c *gin.Context) {
	rules, err := h.Svc.UserRoles(c.Param("id"), httpx.Domain(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(rules)})
}

func (h *HTTP) assignUserRole(c *gin.Context) {
	var req struct {
		Role   string `json:"role"`
		Domain string `json:"domain"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	domain := req.Domain
	if domain == "" {
		domain = httpx.Domain(c)
	}
	if err := h.Svc.AssignRole(c.Param("id"), req.Role, domain); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "user.role_assigned", "user", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) unassignUserRole(c *gin.Context) {
	var req struct {
		Role   string `json:"role"`
		Domain string `json:"domain"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	domain := req.Domain
	if domain == "" {
		domain = httpx.Domain(c)
	}
	if err := h.Svc.UnassignRole(c.Param("id"), req.Role, domain); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "user.role_revoked", "user", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}
