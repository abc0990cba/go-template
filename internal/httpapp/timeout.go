package httpapp

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

// timeoutExemptPrefixes outlive an interactive deadline: the SSE hub streams
// until the client leaves, and file and export transfers are bounded by the
// body/upload caps rather than a request budget.
var timeoutExemptPrefixes = []string{"/api/events", "/api/files", "/api/export"}

// requestTimeout bounds every /api request's context. Handlers and the
// database driver honor the deadline, so a wedged backend surfaces as a fast
// 504 (context.DeadlineExceeded, mapped in httpx) instead of a connection
// held until the client gives up. The middleware never writes a racing
// response: a handler that ignores its context still completes normally.
func (a *App) requestTimeout() gin.HandlerFunc {
	timeout := a.cfg.RequestTimeout
	if timeout <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		for _, prefix := range timeoutExemptPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
