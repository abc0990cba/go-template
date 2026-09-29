package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// apiSearch runs the combined, permission-scoped search across users and
// tickets in the active domain. The service decides which sections the
// caller may see; an empty query answers empty sections.
func (a *App) apiSearch(c *gin.Context) {
	user := a.currentUser(c)
	domain, _ := c.Get("domain")
	domainStr, _ := domain.(string)
	results, err := a.services.Search.Search(c.Request.Context(), user.ID, domainStr, c.Query("q"), intQuery(c, "limit", 10))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, results)
}
