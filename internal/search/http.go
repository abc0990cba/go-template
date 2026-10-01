package search

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the search handler's dependencies.
type HTTP struct {
	Svc    *Service
	Logger *slog.Logger
}

// RegisterHTTP mounts the search route on the authorized group. There is no
// permission guard: the service scopes the sections per caller.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/search", h.search)
}

// search runs the combined, permission-scoped search across users and
// tickets in the active domain. The service decides which sections the
// caller may see; an empty query answers empty sections.
func (h *HTTP) search(c *gin.Context) {
	user := users.CurrentUser(c)
	results, err := h.Svc.Search(c.Request.Context(), user.ID, httpx.Domain(c), c.Query("q"), httpx.IntQuery(c, "limit", DefaultLimit))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, results)
}
