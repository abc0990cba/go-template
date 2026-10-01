package httpapp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/observ"
)

// securityHeaders sets hardening headers on every response. The SPA is
// served same-origin, so the CSP needs no external sources; inline styles
// stay allowed because component libraries set style attributes. HSTS is
// emitted only in secure-cookie mode: the flag is the deployment's claim
// that TLS terminates in front of us.
func (a *App) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if a.cfg.CookieSecure {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; font-src 'self' data:")
		c.Next()
	}
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

// rateLimitScope builds the per-scope limiter middleware, keyed by the
// callback (client IP for the shared scopes, principal id for the identity
// scope). With the factory wired the budget is shared by every api process
// (Redis sliding window); without one, per-process token buckets do the job.
// Limiter failures fail open: shedding traffic because the limiter is down
// would turn one outage into two.
func (a *App) rateLimitScope(scope string, key func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter := a.limiters[scope]
		if limiter == nil {
			c.Next()
			return
		}
		ok, retryAfter, err := limiter.Allow(c.Request.Context(), scope+":"+key(c))
		if err != nil {
			observ.RateLimitErrors.Inc()
			c.Next()
			return
		}
		if !ok {
			observ.RateLimitRejected.WithLabelValues(scope).Inc()
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			}
			a.problem(c, http.StatusTooManyRequests, "too many requests", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// apiRateLimit sheds excess traffic across the whole /api surface, per
// client IP, with the general API budget.
func (a *App) apiRateLimit() gin.HandlerFunc {
	return a.rateLimitScope("api", func(c *gin.Context) string { return c.ClientIP() })
}

// authRateLimit throttles login, register, and password-reset attempts per
// client IP with a much tighter budget than the general API limit.
func (a *App) authRateLimit() gin.HandlerFunc {
	return a.rateLimitScope("auth", func(c *gin.Context) string { return c.ClientIP() })
}

// identityRateLimit runs after authentication and throttles each principal
// (user id, or the API-key owner) with its own budget, so one hot account
// cannot exhaust an IP budget shared with everyone behind the same NAT.
func (a *App) identityRateLimit() gin.HandlerFunc {
	return a.rateLimitScope("user", func(c *gin.Context) string {
		if id := c.GetString("user_id"); id != "" {
			return id
		}
		return c.ClientIP()
	})
}
