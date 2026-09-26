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

// securityHeaders sets hardening headers on every response. The SPA is
// served same-origin, so the CSP needs no external sources; inline styles
// stay allowed because component libraries set style attributes.
func (a *App) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; font-src 'self' data:")
		c.Next()
	}
}

// newRandomToken is shared by session tokens and OAuth state.
func newRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// originCheck guards state-changing /api requests against cross-site posts.
// Browsers always send Origin on cross-origin requests, and Sec-Fetch-Site
// on modern ones; requests without either are non-browser clients (curl, k6)
// and pass. The cookie stays SameSite=Lax as a second line of defense.
func (a *App) originCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if site := c.GetHeader("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "same-site" && site != "none" {
			a.problem(c, http.StatusForbidden, "cross-site request rejected", nil)
			c.Abort()
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" {
			host := c.Request.Host
			if !sameHost(origin, host) {
				a.problem(c, http.StatusForbidden, "cross-origin request rejected", nil)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// sameHost compares an Origin header (scheme://host[:port]) with the
// request's Host.
func sameHost(origin, host string) bool {
	origin = strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	return strings.EqualFold(strings.TrimSuffix(origin, "/"), host)
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
