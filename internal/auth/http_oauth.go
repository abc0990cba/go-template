// OAuth link flows stay on the server: redirects and callbacks. They mount
// on the root engine (outside /api) and answer redirects, not problem+json.
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// authRequired guards the server-side OAuth redirect routes, which still
// answer redirects rather than problem+json.
func (h *HTTP) authRequired(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := h.Svc.CurrentUser(c.Request.Context(), httpx.SessionToken(c))
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.Set("user_id", user.ID)
		c.Set("user", user)
		next(c)
	}
}

// oauthProvider is a gin path parameter; it must be one of the manager's
// configured providers.
func (h *HTTP) oauthLogin(c *gin.Context) {
	provider := c.Param("provider")
	if !h.OAuth.IsConfigured(provider) {
		c.Redirect(http.StatusSeeOther, "/profile?oauth_error="+provider+"+oauth+is+not+configured")
		return
	}
	// Linking requires an active session by policy; login via OAuth alone
	// is out of scope for this change. authRequired resolved the user.
	user := users.CurrentUser(c)
	// Providers that mandate PKCE (Twitter) get a verifier minted here and
	// stored with the state; the challenge rides in the authorize URL.
	verifier, challenge := h.OAuth.NewCodeVerifier(provider)
	state := OAuthState{
		State:        kernel.NewID(),
		Provider:     provider,
		UserID:       user.ID,
		CodeVerifier: verifier,
		// Redirect target is fixed; the field exists for future flows.
		ExpiresAt: h.Now().Add(h.OAuthStateTTL),
	}
	if err := h.Svc.SaveState(c.Request.Context(), state); err != nil {
		h.Logger.Error("save oauth state failed", "err", err)
		httpx.Problem(c, http.StatusInternalServerError, "oauth login failed", nil)
		return
	}
	url, err := h.OAuth.AuthCodeURL(provider, state.State, challenge)
	if err != nil {
		h.Logger.Error("build authorize url failed", "provider", provider, "err", err)
		httpx.Problem(c, http.StatusInternalServerError, "oauth login failed", nil)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

func (h *HTTP) oauthCallback(c *gin.Context) {
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
	state, err := h.Svc.ConsumeState(c.Request.Context(), q.Get("state"))
	if err != nil || state.Provider != provider {
		back("invalid+oauth+state")
		return
	}
	if state.UserID == "" {
		back("oauth+state+is+not+linked+to+a+session")
		return
	}
	// The state's user must still own the session that started the flow.
	user, err := h.Svc.CurrentUser(c.Request.Context(), httpx.SessionToken(c))
	if err != nil || user.ID != state.UserID {
		back("session+expired+during+oauth")
		return
	}
	profile, err := h.OAuth.ExchangeProfile(c.Request.Context(), provider, q.Get("code"), state.CodeVerifier)
	if err != nil {
		h.Logger.Error("oauth exchange failed", "provider", provider, "err", err)
		back("oauth+exchange+failed")
		return
	}
	if err := h.Svc.LinkIdentity(c.Request.Context(), user.ID, profile); err != nil {
		back("linking+identity+failed")
		return
	}
	c.Redirect(http.StatusSeeOther, "/profile")
}
