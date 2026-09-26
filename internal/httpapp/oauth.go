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
		c.Redirect(http.StatusSeeOther, "/profile?oauth_error="+provider+"+oauth+is+not+configured")
		return
	}
	// Linking requires an active session by policy; login via OAuth alone
	// is out of scope for this change. apiAuthHandler resolved the user.
	user := a.currentUser(c)
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
	// Every failure funnels back to the profile page with a readable query
	// message; the SPA surfaces it as a dismissible alert.
	back := func(msg string) {
		c.Redirect(http.StatusSeeOther, "/profile?oauth_error="+msg)
	}
	if failure := q.Get("error"); failure != "" {
		back("provider+rejected+the+request")
		return
	}
	state, err := a.services.Auth.ConsumeState(c.Request.Context(), q.Get("state"))
	if err != nil || state.Provider != provider {
		back("invalid+oauth+state")
		return
	}
	if state.UserID == "" {
		back("oauth+state+is+not+linked+to+a+session")
		return
	}
	// The state's user must still own the session that started the flow.
	user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
	if err != nil || user.ID != state.UserID {
		back("session+expired+during+oauth")
		return
	}
	profile, err := a.oauth.ExchangeProfile(c.Request.Context(), provider, q.Get("code"))
	if err != nil {
		a.logger.Error("oauth exchange failed", "provider", provider, "err", err)
		back("oauth+exchange+failed")
		return
	}
	if err := a.services.Auth.LinkIdentity(c.Request.Context(), user.ID, profile); err != nil {
		back("linking+identity+failed")
		return
	}
	c.Redirect(http.StatusSeeOther, "/profile")
}
