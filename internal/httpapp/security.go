package httpapp

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// securityHeaders sets hardening headers on every response. The CSP allows
// the CDN used by htmx and the docs page plus inline styles.
func (a *App) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		c.Next()
	}
}

// newRandomToken is shared by csrf token generation and OAuth state.
func newRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// csrfIssue runs globally: it gives every visitor a token cookie and puts
// the token into the request context so forms render the same value the
// cookie carries. Without the context value the double-submit check could
// never pass.
func (a *App) csrfIssue() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("app_csrf")
		if err != nil || token == "" {
			token = newRandomToken()
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("app_csrf", token, 3600*24, "/", "", a.cfg.CookieSecure, false)
		}
		c.Set("csrf_token", token)
		c.Next()
	}
}

// verifyCSRF enforces the double-submit pattern on HTML form posts: the
// form field must match the app_csrf cookie. Routes under /api/ are exempt
// because the JSON API does not rely on ambient cookie credentials.
func (a *App) verifyCSRF(c *gin.Context) bool {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		return true
	}
	cookie, _ := c.Cookie("app_csrf")
	if cookie == "" || c.PostForm("csrf_token") != cookie {
		a.problem(c, http.StatusForbidden, "csrf token mismatch", nil)
		return false
	}
	return true
}

type bucketEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipBuckets struct {
	mu         sync.Mutex
	buckets    map[string]*bucketEntry
	newLimiter func() *rate.Limiter
}

func newIPBuckets(rps float64, burst int) *ipBuckets {
	ib := &ipBuckets{buckets: map[string]*bucketEntry{}}
	go func() {
		for {
			time.Sleep(time.Minute)
			ib.mu.Lock()
			for ip, b := range ib.buckets {
				if time.Since(b.lastSeen) > 10*time.Minute {
					delete(ib.buckets, ip)
				}
			}
			ib.mu.Unlock()
		}
	}()
	ib.newLimiter = func() *rate.Limiter { return rate.NewLimiter(rate.Limit(rps), burst) }
	return ib
}

// authRateLimit throttles login and register attempts per client IP with a
// token bucket; idle buckets are dropped after ten minutes.
func (a *App) authRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		a.limits.mu.Lock()
		b, ok := a.limits.buckets[c.ClientIP()]
		if !ok {
			b = &bucketEntry{limiter: a.limits.newLimiter()}
			a.limits.buckets[c.ClientIP()] = b
		}
		b.lastSeen = time.Now()
		limiter := b.limiter
		a.limits.mu.Unlock()

		if !limiter.Allow() {
			a.problem(c, http.StatusTooManyRequests, "too many requests", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
