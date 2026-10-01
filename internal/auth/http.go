// Auth HTTP surface: registration, login/logout, the SPA session probe,
// the profile self-service listings, and the OAuth provider probe.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the auth handlers' dependencies. The direct service pointers
// are cycle-free: rbac imports audit/users only, and none of them imports
// auth.
type HTTP struct {
	Svc           *Service
	RBAC          *rbac.Service
	Users         *users.Service
	MFA           *mfa.Service
	Notifications *notifications.Service
	Audit         *audit.Service
	Settings      *settings.Service
	OAuth         *oauth.Manager
	Logger        *slog.Logger
	// AuthRateLimit is the assembly's tighter per-IP budget for login and
	// register.
	AuthRateLimit gin.HandlerFunc
	// RequirePermission is the assembly's catalog-permission guard
	// (impersonation).
	RequirePermission func(obj, act string) gin.HandlerFunc
	// SessionTTL and CookieSecure mirror the login cookie flags.
	SessionTTL   time.Duration
	CookieSecure bool
	// OAuthStateTTL bounds the single-use CSRF state of the link flow.
	OAuthStateTTL time.Duration
}

// RegisterHTTP mounts the auth routes: login/logout on the public group
// under the tight auth budget, the session probe and profile self-service
// on the authorized group, and impersonation behind its permission guard.
func RegisterHTTP(public, authorized *gin.RouterGroup, h HTTP) {
	public.POST("/auth/register", h.AuthRateLimit, h.register)
	public.POST("/auth/login", h.AuthRateLimit, h.login)
	public.POST("/auth/logout", h.logout)
	public.GET("/oauth/providers", h.providers)

	authorized.GET("/me", h.me)
	authorized.GET("/me/identities", h.identities)
	authorized.GET("/sessions", h.sessions)
	authorized.POST("/profile/logout-all", h.logoutAll)
	authorized.POST("/profile/unlink/:provider", h.unlinkProvider)
	authorized.POST("/impersonation", h.RequirePermission("impersonation", "execute"), h.impersonate)
	authorized.POST("/impersonation/stop", h.stopImpersonation)
}

// RegisterRoot mounts the server-side OAuth redirect routes on the root
// engine — they live outside /api by contract with the SPA.
func RegisterRoot(r *gin.Engine, h HTTP) {
	r.GET("/auth/:provider/login", h.authRequired(h.oauthLogin))
	r.GET("/auth/:provider/callback", h.oauthCallback)
}

// registerRequest mirrors the generated RegisterRequest: email required,
// name optional.
type registerRequest struct {
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Password string  `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// derefString normalizes the optional request fields.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// register creates an account and opens the first session. The first
// account becomes superadmin; later ones get the viewer role.
func (h *HTTP) register(c *gin.Context) {
	var req registerRequest
	if !httpx.Bind(c, &req) {
		return
	}
	if !h.Settings.Bool(c.Request.Context(), "registration.enabled") {
		httpx.Problem(c, http.StatusForbidden, "registration is closed", nil)
		return
	}
	user, token, err := h.Svc.Register(c.Request.Context(), req.Email, derefString(req.Name), req.Password)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	h.grantRegistrationRole(c.Request.Context(), user.ID)
	httpx.SetSession(c, token, h.SessionTTL, h.CookieSecure)
	audit.Record(h.Audit, c, "user.registered", "user", user.ID, nil, gin.H{"email": user.Email})
	c.JSON(http.StatusCreated, gin.H{"user": user, "token": token})
}

// login verifies credentials and opens a session. Accounts with a
// confirmed TOTP factor instead receive a short-lived challenge token and
// complete the login at /api/auth/mfa/challenge.
func (h *HTTP) login(c *gin.Context) {
	var req loginRequest
	if !httpx.Bind(c, &req) {
		return
	}
	user, token, challenge, err := h.MFA.Authenticate(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, mfa.ErrEnrollmentRequired) {
			c.JSON(http.StatusForbidden, gin.H{
				"type": "about:blank", "title": "two-factor enrollment required", "status": http.StatusForbidden,
				"mfa_enrollment_required": true,
			})
			return
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	if challenge != "" {
		c.JSON(http.StatusUnauthorized, gin.H{"mfa_required": true, "challenge": challenge})
		return
	}
	httpx.SetSession(c, token, h.SessionTTL, h.CookieSecure)
	audit.Record(h.Audit, c, "user.logged_in", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"user": user, "token": token})
}

// logout revokes the presented session and clears the cookie.
func (h *HTTP) logout(c *gin.Context) {
	// A missing session is the normal case for logout; nothing to report.
	actor, _ := users.CurrentUserOK(c)
	_ = h.Svc.Logout(c.Request.Context(), httpx.SessionToken(c))
	httpx.ClearSession(c, h.CookieSecure)
	if actor.ID != "" {
		audit.Record(h.Audit, c, "user.logged_out", "user", actor.ID, nil, nil)
	}
	c.Status(http.StatusNoContent)
}

// meResponse is the SPA session probe: the account plus the RBAC context
// (active organization, roles, expanded catalog permissions) the UI needs
// to gate navigation and actions, plus the impersonation state.
type meResponse struct {
	users.User
	Orgs             []rbac.Organization `json:"orgs"`
	ActiveOrg        string              `json:"active_org"`
	Roles            []string            `json:"roles"`
	Permissions      []string            `json:"permissions"`
	Impersonated     bool                `json:"impersonated"`
	ImpersonatorID   string              `json:"impersonator_id,omitempty"`
	ImpersonatorName string              `json:"impersonator_name,omitempty"`
}

// me resolves the caller's account; the SPA uses it as its session probe.
func (h *HTTP) me(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	resp := meResponse{
		User: user, ActiveOrg: domain,
		Orgs: []rbac.Organization{}, Roles: []string{}, Permissions: []string{},
	}
	// Nil slices from the stores must stay [] on the wire: the SPA reads
	// me.orgs unconditionally, and JSON null breaks that contract.
	if orgs, err := h.RBAC.OrgsForUser(c.Request.Context(), user.ID); err == nil && len(orgs) > 0 {
		resp.Orgs = orgs
	}
	if roleRules, err := h.RBAC.UserRoles(user.ID, domain); err == nil {
		for _, rule := range roleRules {
			resp.Roles = append(resp.Roles, rule.Object)
		}
	}
	resp.Permissions = h.RBAC.EffectivePermissions(user.ID, domain)
	if resp.Permissions == nil {
		resp.Permissions = []string{}
	}
	if session, err := h.Svc.CurrentSession(c.Request.Context(), httpx.SessionToken(c)); err == nil && session.ImpersonatedBy != "" {
		resp.Impersonated = true
		resp.ImpersonatorID = session.ImpersonatedBy
		if admin, err := h.Users.Get(c.Request.Context(), session.ImpersonatedBy); err == nil {
			resp.ImpersonatorName = admin.Email
		}
	}
	c.JSON(http.StatusOK, resp)
}

// providers reports which identity providers have credentials, so
// the profile page renders disabled buttons instead of dead ends.
func (h *HTTP) providers(c *gin.Context) {
	providers := []ProviderInfo{
		{Name: "google", Configured: h.OAuth.IsConfigured("google")},
		{Name: "github", Configured: h.OAuth.IsConfigured("github")},
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(providers)})
}

// identities lists the OAuth identities linked to the caller.
func (h *HTTP) identities(c *gin.Context) {
	user := users.CurrentUser(c)
	identities, err := h.Svc.Identities(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(identities)})
}

// sessions lists the caller's active sessions; the current session is
// flagged by comparing the opaque token prefixes.
func (h *HTTP) sessions(c *gin.Context) {
	user := users.CurrentUser(c)
	sessions, err := h.Svc.Sessions(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	current := sessionTokenPrefix(httpx.SessionToken(c))
	flagged := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		flagged = append(flagged, gin.H{
			"id": s.ID, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt, "current": s.ID == current,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(flagged)})
}

// logoutAll revokes every session of the caller, including this one.
func (h *HTTP) logoutAll(c *gin.Context) {
	user := users.CurrentUser(c)
	if err := h.Svc.LogoutAll(c.Request.Context(), user.ID); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	httpx.ClearSession(c, h.CookieSecure)
	c.Status(http.StatusNoContent)
}

// unlinkProvider detaches an OAuth identity from the caller.
func (h *HTTP) unlinkProvider(c *gin.Context) {
	user := users.CurrentUser(c)
	if err := h.Svc.UnlinkProvider(c.Request.Context(), user.ID, c.Param("provider")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// sessionTokenPrefix mirrors the store's public session id derivation.
func sessionTokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

// grantRegistrationRole gives the first account superadmin and everyone
// after it the viewer role in the platform domain, so a fresh install is
// administrable and self-registered users land with least privilege. The
// account already exists when this runs, so "first" means the store holds
// exactly one user.
func (h *HTTP) grantRegistrationRole(ctx context.Context, userID string) {
	domain := rbac.PlatformDomain
	if h.Users.Count(ctx) == 1 {
		_ = h.RBAC.AssignRole(userID, "superadmin", domain)
		return
	}
	_ = h.RBAC.AssignRole(userID, "viewer", domain)
}
