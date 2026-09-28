// RBAC HTTP surface: the domain middleware, the permission guard, and the
// org/role/policy handlers the admin console talks to.
package httpapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/service"
)

// orgDomain resolves the active Casbin domain for the request: the X-Org-Id
// header when present, else the user's single organization, else the
// platform domain "*". Non-members (without full access) get 403; users
// with fewer rights still pass here and fail the permission guard.
func (a *App) orgDomain(c *gin.Context) {
	user := a.currentUser(c)
	domain := c.GetHeader("X-Org-Id")
	switch {
	case domain == service.PlatformDomain:
		// Explicit platform scope; the permission guard decides.
	case domain != "":
		member, err := a.services.RBAC.OrgMember(c.Request.Context(), domain, user.ID)
		if err != nil {
			a.problem(c, http.StatusBadRequest, "unknown organization", nil)
			c.Abort()
			return
		}
		if !member && !a.services.RBAC.IsAdmin(user.ID, domain) {
			a.forbidden(c)
			return
		}
	default:
		domain = service.PlatformDomain
		if orgs, err := a.services.RBAC.OrgsForUser(c.Request.Context(), user.ID); err == nil && len(orgs) == 1 {
			domain = orgs[0].ID
		}
	}
	c.Set("domain", domain)
}

// requestDomain returns the domain resolved by the orgDomain middleware.
func (a *App) requestDomain(c *gin.Context) string {
	if domain, ok := c.Get("domain"); ok {
		if s, ok := domain.(string); ok && s != "" {
			return s
		}
	}
	return service.PlatformDomain
}

// requirePermission guards a route with a catalog permission; denials are
// 403 problem+json, never redirects.
func (a *App) requirePermission(obj, act string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := a.currentUser(c)
		if !a.services.RBAC.Enforce(user.ID, a.requestDomain(c), obj, act) {
			a.forbidden(c)
			return
		}
		c.Next()
	}
}

func (a *App) forbidden(c *gin.Context) {
	a.problem(c, http.StatusForbidden, "forbidden", nil)
}

// grantRegistrationRole gives the first account superadmin and everyone
// after it the viewer role in the platform domain, so a fresh install is
// administrable and self-registered users land with least privilege. The
// account already exists when this runs, so "first" means the store holds
// exactly one user.
func (a *App) grantRegistrationRole(ctx context.Context, userID string) {
	domain := service.PlatformDomain
	if a.services.Users.Count(ctx) == 1 {
		_ = a.services.RBAC.AssignRole(userID, "superadmin", domain)
		return
	}
	_ = a.services.RBAC.AssignRole(userID, "viewer", domain)
}

// apiRbacCatalog serves the resource/action matrix for the roles page.
func (a *App) apiRbacCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"resources": a.services.RBAC.Catalog()})
}

func (a *App) apiRoles(c *gin.Context) {
	roles, err := a.services.RBAC.Roles(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(roles)})
}

func (a *App) apiCreateRole(c *gin.Context) {
	var req struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	role, err := a.services.RBAC.CreateRole(c.Request.Context(), req.Key, req.Name, req.Description)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "role.created", "role", role.Key, nil, role)
	c.JSON(http.StatusCreated, role)
}

func (a *App) apiUpdateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := a.services.RBAC.UpdateRole(c.Request.Context(), c.Param("key"), req.Name, req.Description); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "role.updated", "role", c.Param("key"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiDeleteRole(c *gin.Context) {
	if err := a.services.RBAC.DeleteRole(c.Request.Context(), c.Param("key")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "role.deleted", "role", c.Param("key"), nil, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiRolePermissions(c *gin.Context) {
	rules, err := a.services.RBAC.RolePermissions(c.Param("key"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(rules)})
}

func (a *App) apiSetRolePermissions(c *gin.Context) {
	var req struct {
		Domain      string               `json:"domain"`
		Permissions []service.Permission `json:"permissions"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := a.services.RBAC.SetRolePermissions(c.Param("key"), req.Domain, req.Permissions); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "role.permissions_updated", "role", c.Param("key"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiPolicies(c *gin.Context) {
	ptype := c.DefaultQuery("ptype", "p")
	rules, err := a.services.RBAC.ListPolicies(c.Request.Context(), ptype)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(rules)})
}

func (a *App) apiAddPolicy(c *gin.Context) {
	var req model.PolicyRule
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	req.PType = c.DefaultQuery("ptype", req.PType)
	if err := a.services.RBAC.AddPolicyRule(req); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "policy.added", "policy", req.Subject, nil, req)
	c.Status(http.StatusCreated)
}

func (a *App) apiRemovePolicy(c *gin.Context) {
	var req model.PolicyRule
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := a.services.RBAC.RemovePolicyRule(req); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "policy.removed", "policy", req.Subject, req, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiUserRoles(c *gin.Context) {
	rules, err := a.services.RBAC.UserRoles(c.Param("id"), a.requestDomain(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(rules)})
}

func (a *App) apiAssignUserRole(c *gin.Context) {
	var req struct {
		Role   string `json:"role"`
		Domain string `json:"domain"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	domain := req.Domain
	if domain == "" {
		domain = a.requestDomain(c)
	}
	if err := a.services.RBAC.AssignRole(c.Param("id"), req.Role, domain); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.role_assigned", "user", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiUnassignUserRole(c *gin.Context) {
	var req struct {
		Role   string `json:"role"`
		Domain string `json:"domain"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	domain := req.Domain
	if domain == "" {
		domain = a.requestDomain(c)
	}
	if err := a.services.RBAC.UnassignRole(c.Param("id"), req.Role, domain); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.role_revoked", "user", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiOrgs(c *gin.Context) {
	orgs, next, err := a.services.RBAC.ListOrgs(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(orgs), "next_cursor": next})
}

func (a *App) apiCreateOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	org, err := a.services.RBAC.CreateOrg(c.Request.Context(), req.Name, req.Slug)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.created", "org", org.ID, nil, org)
	c.JSON(http.StatusCreated, org)
}

func (a *App) apiGetOrg(c *gin.Context) {
	org, err := a.services.RBAC.GetOrg(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, org)
}

func (a *App) apiUpdateOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := a.services.RBAC.UpdateOrg(c.Request.Context(), c.Param("id"), req.Name, req.Slug); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.updated", "org", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiDeleteOrg(c *gin.Context) {
	if err := a.services.RBAC.DeleteOrg(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.deleted", "org", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiOrgMembers(c *gin.Context) {
	members, err := a.services.RBAC.OrgMembers(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(members)})
}

func (a *App) apiAddOrgMember(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := a.services.RBAC.AddOrgMember(c.Request.Context(), c.Param("id"), req.UserID); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.member_added", "org", c.Param("id"), nil, req)
	c.Status(http.StatusNoContent)
}

func (a *App) apiRemoveOrgMember(c *gin.Context) {
	if err := a.services.RBAC.RemoveOrgMember(c.Request.Context(), c.Param("id"), c.Param("userId")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.member_removed", "org", c.Param("id"), nil, gin.H{"user_id": c.Param("userId")})
	c.Status(http.StatusNoContent)
}

// apiImpersonate starts an impersonation session for the target user.
func (a *App) apiImpersonate(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	admin := a.currentUser(c)
	if admin.ID == req.UserID {
		a.problem(c, http.StatusBadRequest, "cannot impersonate yourself", nil)
		return
	}
	token, err := a.services.Auth.Impersonate(c.Request.Context(), admin.ID, req.UserID)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.setSession(c, token)
	a.audit(c, "impersonation.started", "user", req.UserID, nil, gin.H{"admin_id": admin.ID})
	if err := a.services.Notifications.Create(c.Request.Context(), req.UserID, "impersonation.started", "An administrator signed in to your account", "Contact support if this was not expected.", "/profile"); err != nil {
		a.logger.Error("impersonation notification failed", "err", err)
	}
	c.JSON(http.StatusOK, gin.H{"status": "impersonating", "user_id": req.UserID})
}

// apiStopImpersonation ends the impersonated session and restores the admin.
func (a *App) apiStopImpersonation(c *gin.Context) {
	session, err := a.services.Auth.CurrentSession(c.Request.Context(), a.sessionToken(c))
	if err != nil || session.ImpersonatedBy == "" {
		a.problem(c, http.StatusBadRequest, "not impersonating", nil)
		return
	}
	adminToken, err := a.services.Auth.StopImpersonation(c.Request.Context(), a.sessionToken(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.setSession(c, adminToken)
	a.audit(c, "impersonation.stopped", "user", session.UserID, nil, gin.H{"admin_id": session.ImpersonatedBy})
	c.Status(http.StatusNoContent)
}
