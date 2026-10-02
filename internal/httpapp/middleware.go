package httpapp

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

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

// bodyLimit caps request bodies before handlers read them. The
// ContentLength check rejects pre-declared oversize bodies up front; the
// MaxBytesReader wrapper catches chunked requests whose size only becomes
// known while streaming — the mid-read overflow surfaces in a.bind as 413.
// The upload route earns a higher cap (the file limit plus multipart
// framing) when the files service is wired.
func (a *App) bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := int64(maxBodyBytes)
		if a.services != nil && a.services.Files != nil && strings.HasPrefix(c.Request.URL.Path, "/api/files") {
			limit = a.services.Files.MaxSize() + 1<<20
		}
		if c.Request.ContentLength > limit {
			a.problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
			c.Abort()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
