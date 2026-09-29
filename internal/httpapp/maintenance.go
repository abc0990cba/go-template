package httpapp

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const maintenanceSetting = "maintenance.enabled"

// maintenanceMemoTTL bounds the cost of the gate: the flag is read from the
// settings store at most once per interval, so flipping it takes effect on
// every replica within that window without hammering the store on the hot
// path. A var so tests can collapse the window.
var maintenanceMemoTTL = 5 * time.Second

// maintenanceGate sheds all API traffic while the maintenance.enabled flag
// is on. Login and logout stay reachable (operators must be able to sign
// in), and the settings API stays reachable because it is permission-gated —
// that is the switch an admin flips to end maintenance. Health, metrics,
// version, OpenAPI, and the SPA live outside /api and pass untouched, so the
// UI can render its banner and the orchestrator keeps probing.
func (a *App) maintenanceGate() gin.HandlerFunc {
	var mu sync.Mutex
	var until time.Time
	var on bool

	// on reads the flag through the memo; the store is hit at most once per
	// interval per process.
	onNow := func() bool {
		mu.Lock()
		defer mu.Unlock()
		if time.Now().Before(until) {
			return on
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		on = a.services.Settings.Bool(ctx, maintenanceSetting)
		until = time.Now().Add(maintenanceMemoTTL)
		return on
	}

	return func(c *gin.Context) {
		if !onNow() {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api/") || maintenanceAllowed(path) {
			c.Next()
			return
		}
		c.Header("Retry-After", "30")
		a.problem(c, http.StatusServiceUnavailable, "service under maintenance", nil)
		c.Abort()
	}
}

// maintenanceAllowed lists the API paths that answer during maintenance.
// Everything listed is additionally guarded by its own route middleware:
// letting the request through the gate never bypasses authentication.
func maintenanceAllowed(path string) bool {
	if path == "/api/auth/login" || path == "/api/auth/logout" {
		return true
	}
	return path == "/api/settings/public" || path == "/api/settings" || strings.HasPrefix(path, "/api/settings/")
}
