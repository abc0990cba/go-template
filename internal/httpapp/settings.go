// Settings and feature flags HTTP surface.
package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// apiSettings serves the settings catalog with current values.
func (a *App) apiSettings(c *gin.Context) {
	catalog, err := a.services.Settings.Catalog(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": catalog})
}

// apiUpdateSetting stores one setting; changes are audited.
func (a *App) apiUpdateSetting(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
	}
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user := a.currentUser(c)
	if err := a.services.Settings.Set(c.Request.Context(), c.Param("key"), req.Value, user.Email); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "settings.updated", "setting", c.Param("key"), nil, gin.H{"value": req.Value})
	c.Status(http.StatusNoContent)
}

// apiPublicSettings exposes the safe subset (feature flags the login and
// register pages render against) without requiring a session.
func (a *App) apiPublicSettings(c *gin.Context) {
	c.JSON(http.StatusOK, a.services.Settings.PublicSettings(c.Request.Context()))
}
