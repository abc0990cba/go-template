// Personal API key endpoints: self-service issue/revoke; the raw secret is
// returned exactly once at creation.
package httpapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *App) apiListAPIKeys(c *gin.Context) {
	user := a.currentUser(c)
	keys, err := a.services.APIKeys.List(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(keys)})
}

// apiCreateAPIKey issues a key; the raw secret appears exactly once.
func (a *App) apiCreateAPIKey(c *gin.Context) {
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
		TTL    string   `json:"ttl"`
	}
	if !a.bind(c, &req) {
		return
	}
	var ttl time.Duration
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			a.problem(c, http.StatusBadRequest, "ttl must be a duration like 720h", err)
			return
		}
		ttl = parsed
	}
	user := a.currentUser(c)
	key, secret, err := a.services.APIKeys.Create(c.Request.Context(), user.ID, req.Name, req.Scopes, ttl)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "api_key.created", "api_key", key.ID, nil, gin.H{"name": key.Name, "scopes": key.Scopes})
	c.JSON(http.StatusCreated, gin.H{"key": key, "secret": secret})
}

func (a *App) apiRevokeAPIKey(c *gin.Context) {
	user := a.currentUser(c)
	if err := a.services.APIKeys.Revoke(c.Request.Context(), c.Param("id"), user.ID); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "api_key.revoked", "api_key", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}
