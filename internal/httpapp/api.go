package httpapp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oapi-codegen/runtime/types"
	api "github.com/your-org/go-template/internal/api"
	"github.com/your-org/go-template/internal/model"
)

// derefString normalizes the generated optional fields; Email is a string
// alias so the same helper covers both after conversion at call sites.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefEmail(e *types.Email) string {
	return derefString((*string)(e))
}

// apiRegister creates an account and opens the first session. The first
// account becomes superadmin; later ones get the viewer role.
func (a *App) apiRegister(c *gin.Context) {
	var req api.RegisterRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if !a.services.Settings.Bool(c.Request.Context(), "registration.enabled") {
		a.problem(c, http.StatusForbidden, "registration is closed", nil)
		return
	}
	user, token, err := a.services.Auth.Register(c.Request.Context(), string(req.Email), derefString(req.Name), req.Password)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.grantRegistrationRole(c.Request.Context(), user.ID)
	a.setSession(c, token)
	a.audit(c, "user.registered", "user", user.ID, nil, gin.H{"email": user.Email})
	c.JSON(http.StatusCreated, gin.H{"user": user, "token": token})
}

// apiLogin verifies credentials, opens a session, and sets the cookie.
func (a *App) apiLogin(c *gin.Context) {
	var req api.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, token, err := a.services.Auth.Login(c.Request.Context(), string(req.Email), req.Password)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.setSession(c, token)
	a.audit(c, "user.logged_in", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"user": user, "token": token})
}

// apiLogout revokes the presented session and clears the cookie.
func (a *App) apiLogout(c *gin.Context) {
	// A missing session is the normal case for logout; nothing to report.
	actor := a.auditActor(c)
	_ = a.services.Auth.Logout(c.Request.Context(), a.sessionToken(c))
	a.clearSession(c)
	if actor.ID != "" {
		a.audit(c, "user.logged_out", "user", actor.ID, nil, nil)
	}
	c.Status(http.StatusNoContent)
}

// meResponse is the SPA session probe: the account plus the RBAC context
// (active organization, roles, expanded catalog permissions) the UI needs
// to gate navigation and actions, plus the impersonation state.
type meResponse struct {
	model.User
	Orgs             []model.Organization `json:"orgs"`
	ActiveOrg        string               `json:"active_org"`
	Roles            []string             `json:"roles"`
	Permissions      []string             `json:"permissions"`
	Impersonated     bool                 `json:"impersonated"`
	ImpersonatorID   string               `json:"impersonator_id,omitempty"`
	ImpersonatorName string               `json:"impersonator_name,omitempty"`
}

// apiMe resolves the caller's account; the SPA uses it as its session probe.
func (a *App) apiMe(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	resp := meResponse{
		User: user, ActiveOrg: domain,
		Orgs: []model.Organization{}, Roles: []string{}, Permissions: []string{},
	}
	if orgs, err := a.services.RBAC.OrgsForUser(c.Request.Context(), user.ID); err == nil {
		resp.Orgs = orgs
	}
	if roleRules, err := a.services.RBAC.UserRoles(user.ID, domain); err == nil {
		for _, rule := range roleRules {
			resp.Roles = append(resp.Roles, rule.Object)
		}
	}
	resp.Permissions = a.services.RBAC.EffectivePermissions(user.ID, domain)
	if session, err := a.services.Auth.CurrentSession(c.Request.Context(), a.sessionToken(c)); err == nil && session.ImpersonatedBy != "" {
		resp.Impersonated = true
		resp.ImpersonatorID = session.ImpersonatedBy
		if admin, err := a.services.Users.Get(c.Request.Context(), session.ImpersonatedBy); err == nil {
			resp.ImpersonatorName = admin.Email
		}
	}
	c.JSON(http.StatusOK, resp)
}

// apiOAuthProviders reports which identity providers have credentials, so
// the profile page renders disabled buttons instead of dead ends.
func (a *App) apiOAuthProviders(c *gin.Context) {
	providers := []model.OAuthProviderInfo{
		{Name: "google", Configured: a.oauth.IsConfigured("google")},
		{Name: "github", Configured: a.oauth.IsConfigured("github")},
	}
	c.JSON(http.StatusOK, gin.H{"data": providers})
}

// apiUsers pages through accounts in the requested pagination mode.
func (a *App) apiUsers(c *gin.Context) {
	users, next, err := a.services.Users.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users, "next_cursor": next})
}

func (a *App) apiCreateUser(c *gin.Context) {
	var req api.CreateUserRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, err := a.services.Users.Create(c.Request.Context(), derefEmail(&req.Email), derefString(req.Name), derefString(req.ExternalRef))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.created", "user", user.ID, nil, user)
	c.JSON(http.StatusCreated, user)
}

func (a *App) apiGetUser(c *gin.Context) {
	user, err := a.services.Users.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (a *App) apiUpdateUser(c *gin.Context) {
	var req api.UpdateUserRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	before, _ := a.services.Users.Get(c.Request.Context(), c.Param("id"))
	user, err := a.services.Users.Update(c.Request.Context(), c.Param("id"), derefEmail(&req.Email), req.Name, derefString(req.ExternalRef))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.updated", "user", user.ID, before, user)
	c.JSON(http.StatusOK, user)
}

func (a *App) apiDeleteUser(c *gin.Context) {
	before, _ := a.services.Users.Get(c.Request.Context(), c.Param("id"))
	if err := a.services.Users.Delete(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.deleted", "user", c.Param("id"), before, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiJobs(c *gin.Context) {
	jobs, next, err := a.services.Jobs.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs, "next_cursor": next})
}

func (a *App) apiGetJob(c *gin.Context) {
	job, err := a.services.Jobs.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

// apiProcessJob runs the oldest queued job in-process. Without a distributed
// queue this is how the demo moves jobs forward; with one, it is a no-op
// because the worker owns execution.
func (a *App) apiProcessJob(c *gin.Context) {
	processed, err := a.services.Jobs.ProcessOne(c.Request.Context())
	if err != nil {
		a.logger.Error("job processing failed", "err", err)
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"processed": processed})
}

func (a *App) apiEnrichEnrichment(c *gin.Context) {
	var req api.EnqueueEnrichmentJSONBody
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	payload, err := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: req.Count})
	if err != nil {
		a.fail(c, err)
		return
	}
	job, err := a.services.Jobs.Enqueue(c.Request.Context(), "user.enrich_batch", string(payload), a.currentUser(c).ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (a *App) apiEnrichmentProfiles(c *gin.Context) {
	profiles, next, err := a.services.Jobs.EnrichmentProfiles(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": profiles, "next_cursor": next})
}

func (a *App) apiGenerateReport(c *gin.Context) {
	job, err := a.services.Jobs.Enqueue(c.Request.Context(), "user.report_generate", "{}", a.currentUser(c).ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (a *App) apiReports(c *gin.Context) {
	reports, next, err := a.services.Reports.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reports, "next_cursor": next})
}

func (a *App) apiGetReport(c *gin.Context) {
	report, err := a.services.Reports.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// apiAnalytics serves the dashboard counters.
func (a *App) apiAnalytics(c *gin.Context) {
	c.JSON(http.StatusOK, a.services.Analytics.Summary(c.Request.Context()))
}

// apiTimeseries serves daily event counts for the chart window (1..90 days).
func (a *App) apiTimeseries(c *gin.Context) {
	days := intQuery(c, "days", 30)
	days = max(min(days, 90), 1)
	c.JSON(http.StatusOK, gin.H{
		"days": days,
		"data": a.services.Analytics.Timeseries(c.Request.Context(), days),
	})
}

// apiRecentEvents serves the newest slice of the event trail (1..200).
func (a *App) apiRecentEvents(c *gin.Context) {
	limit := intQuery(c, "limit", 50)
	limit = max(min(limit, 200), 1)
	c.JSON(http.StatusOK, gin.H{"data": a.services.Analytics.RecentEvents(c.Request.Context(), limit)})
}

// apiAnalyticsBreakdown groups events by a properties dimension.
func (a *App) apiAnalyticsBreakdown(c *gin.Context) {
	days := max(min(intQuery(c, "days", 30), 90), 1)
	rows, err := a.services.Analytics.Breakdown(c.Request.Context(), c.DefaultQuery("by", "feature"), days)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": rows})
}

// apiAnalyticsDAU serves distinct active users per day.
func (a *App) apiAnalyticsDAU(c *gin.Context) {
	days := max(min(intQuery(c, "days", 30), 90), 1)
	rows, err := a.services.Analytics.DAU(c.Request.Context(), days)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": rows})
}

// apiIdentities lists the OAuth identities linked to the caller.
func (a *App) apiIdentities(c *gin.Context) {
	user := a.currentUser(c)
	identities, err := a.services.Auth.Identities(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": identities})
}

// apiSessions lists the caller's active sessions; the current session is
// flagged by comparing the opaque token prefixes.
func (a *App) apiSessions(c *gin.Context) {
	user := a.currentUser(c)
	sessions, err := a.services.Auth.Sessions(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	current := sessionTokenPrefix(a.sessionToken(c))
	flagged := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		flagged = append(flagged, gin.H{
			"id": s.ID, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt, "current": s.ID == current,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": flagged})
}

// apiLogoutAll revokes every session of the caller, including this one.
func (a *App) apiLogoutAll(c *gin.Context) {
	user := a.currentUser(c)
	if err := a.services.Auth.LogoutAll(c.Request.Context(), user.ID); err != nil {
		a.fail(c, err)
		return
	}
	a.clearSession(c)
	c.Status(http.StatusNoContent)
}

// apiUnlinkProvider detaches an OAuth identity from the caller.
func (a *App) apiUnlinkProvider(c *gin.Context) {
	user := a.currentUser(c)
	if err := a.services.Auth.UnlinkProvider(c.Request.Context(), user.ID, c.Param("provider")); err != nil {
		a.fail(c, err)
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
