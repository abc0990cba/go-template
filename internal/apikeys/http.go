// Package apikeys implements the personal API key HTTP surface: self-service
// issue/revoke; the raw secret is returned exactly once at creation.
package apikeys

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the API-key handlers' dependencies.
type HTTP struct {
	Svc   *Service
	Audit *audit.Service
}

// RegisterHTTP mounts the API-key routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/api-keys", h.list)
	authorized.POST("/api-keys", h.create)
	authorized.DELETE("/api-keys/:id", h.revoke)
}

func (h *HTTP) list(c *gin.Context) {
	user := users.CurrentUser(c)
	keys, err := h.Svc.List(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(keys)})
}

// create issues a key; the raw secret appears exactly once.
func (h *HTTP) create(c *gin.Context) {
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
		TTL    string   `json:"ttl"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	var ttl time.Duration
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			httpx.Problem(c, http.StatusBadRequest, "ttl must be a duration like 720h", err)
			return
		}
		ttl = parsed
	}
	user := users.CurrentUser(c)
	key, secret, err := h.Svc.Create(c.Request.Context(), user.ID, req.Name, req.Scopes, ttl)
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	audit.Record(h.Audit, c, "api_key.created", "api_key", key.ID, nil, gin.H{"name": key.Name, "scopes": key.Scopes})
	c.JSON(http.StatusCreated, gin.H{"key": key, "secret": secret})
}

func (h *HTTP) revoke(c *gin.Context) {
	user := users.CurrentUser(c)
	if err := h.Svc.Revoke(c.Request.Context(), c.Param("id"), user.ID); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	audit.Record(h.Audit, c, "api_key.revoked", "api_key", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}
