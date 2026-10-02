// RBAC assembly glue: the domain middleware and the permission guard. The
// role/org/policy handlers live in the rbac feature package, impersonation
// moved to the auth feature.

package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/rbac"
)

// orgDomain resolves the active Casbin domain for the request: the X-Org-Id
// header when present, else the user's single organization, else the
// platform domain "*". Non-members (without full access) get 403; users
// with fewer rights still pass here and fail the permission guard.
func (a *App) orgDomain(c *gin.Context) {
	user := a.currentUser(c)
	domain := c.GetHeader("X-Org-Id")
	switch {
	case domain == rbac.PlatformDomain:
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
		domain = rbac.PlatformDomain
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
	return rbac.PlatformDomain
}

// requirePermission guards a route with a catalog permission; denials are
// 403 problem+json, never redirects. A request authenticated by API key is
// additionally narrowed to the key's own scopes: the key can reduce what
// its owner may do, never widen it.
func (a *App) requirePermission(obj, act string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := a.currentUser(c)
		if !a.services.RBAC.Enforce(user.ID, a.requestDomain(c), obj, act) {
			a.forbidden(c)
			return
		}
		if scopes, ok := a.requestScopes(c); ok && !apikeys.ScopesAllow(scopes, obj, act) {
			a.forbidden(c)
			return
		}
		c.Next()
	}
}

// requestScopes reports the API key's scopes when the request was
// authenticated by key rather than by session or JWT.
func (a *App) requestScopes(c *gin.Context) ([]string, bool) {
	raw, ok := c.Get("auth_scopes")
	if !ok {
		return nil, false
	}
	scopes, ok := raw.([]string)
	return scopes, ok
}

func (a *App) forbidden(c *gin.Context) {
	a.problem(c, http.StatusForbidden, "forbidden", nil)
	// Without the abort the rest of the chain still ran: the response said
	// 403 while the handler executed its side effects.
	c.Abort()
}
