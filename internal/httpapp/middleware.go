package httpapp

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxBodyBytes = 1 << 20

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// requestID tags every request with a collision-resistant id, honoring an
// upstream X-Request-ID so the id survives proxy hops.
func (a *App) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Next()
	}
}

// bodyLimit rejects oversized request bodies before handlers read them.
func (a *App) bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBodyBytes {
			a.problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
