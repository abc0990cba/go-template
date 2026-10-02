// Package settings serves the settings and feature flags HTTP surface.
package settings

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the settings handlers' dependencies.
type HTTP struct {
	Svc   *Service
	Audit *audit.Service
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the settings routes; the public flags live on the
// unauthenticated group, the catalog and updates on the authorized one.
func RegisterHTTP(public, authorized *gin.RouterGroup, h HTTP) {
	public.GET("/settings/public", h.publicSettings)
	authorized.GET("/settings", h.RequirePermission("settings", "read"), h.catalog)
	authorized.PUT("/settings/:key", h.RequirePermission("settings", "update"), h.update)
}

// catalog serves the settings catalog with current values.
func (h *HTTP) catalog(c *gin.Context) {
	catalog, err := h.Svc.Catalog(c.Request.Context())
	if err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(catalog)})
}

// update stores one setting; changes are audited.
func (h *HTTP) update(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	user := users.CurrentUser(c)
	if err := h.Svc.Set(c.Request.Context(), c.Param("key"), req.Value, user.Email); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	audit.Record(h.Audit, c, "settings.updated", "setting", c.Param("key"), nil, gin.H{"value": req.Value})
	c.Status(http.StatusNoContent)
}

// publicSettings exposes the safe subset (feature flags the login and
// register pages render against) without requiring a session.
func (h *HTTP) publicSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.Svc.PublicSettings(c.Request.Context()))
}
