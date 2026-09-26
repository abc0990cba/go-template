package httpapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
)

// oauthProvider is a gin path parameter; it must be one of the manager's
// configured providers.
func (a *App) oauthLogin(c *gin.Context) {
	provider := c.Param("provider")
	if !a.oauth.IsConfigured(provider) {
		a.render(c, loginPage(provider+" oauth is not configured", a.csrfToken(c)))
		return
	}
	// Linking requires an active session by policy; login via OAuth alone
	// is out of scope for this change.
	user, ok := a.currentUser(c)
	if !ok {
		return
	}
	state := model.OAuthState{
		State:    newRandomToken(),
		Provider: provider,
		UserID:   user.ID,
		// Redirect target is fixed; the field exists for future flows.
		ExpiresAt: time.Now().Add(a.cfg.OAuthStateTTL),
	}
	if err := a.services.Auth.SaveState(c.Request.Context(), state); err != nil {
		a.logger.Error("save oauth state failed", "err", err)
		a.problem(c, http.StatusInternalServerError, "oauth login failed", nil)
		return
	}
	url, err := a.oauth.AuthCodeURL(provider, state.State)
	if err != nil {
		a.logger.Error("build authorize url failed", "provider", provider, "err", err)
		a.problem(c, http.StatusInternalServerError, "oauth login failed", nil)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

func (a *App) oauthCallback(c *gin.Context) {
	provider := c.Param("provider")
	q := c.Request.URL.Query()
	if failure := q.Get("error"); failure != "" {
		a.renderProfileData(c, "provider rejected the request: "+failure)
		return
	}
	state, err := a.services.Auth.ConsumeState(c.Request.Context(), q.Get("state"))
	if err != nil || state.Provider != provider {
		a.problem(c, http.StatusBadRequest, "invalid oauth state", nil)
		return
	}
	if state.UserID == "" {
		a.problem(c, http.StatusBadRequest, "oauth state is not linked to a session", nil)
		return
	}
	// The state's user must still own the session that started the flow.
	user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
	if err != nil || user.ID != state.UserID {
		a.problem(c, http.StatusUnauthorized, "session expired during oauth", nil)
		return
	}
	profile, err := a.oauth.ExchangeProfile(c.Request.Context(), provider, q.Get("code"))
	if err != nil {
		a.logger.Error("oauth exchange failed", "provider", provider, "err", err)
		a.renderProfileData(c, "oauth exchange failed")
		return
	}
	if err := a.services.Auth.LinkIdentity(c.Request.Context(), user.ID, profile); err != nil {
		a.renderProfileData(c, err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/profile")
}
