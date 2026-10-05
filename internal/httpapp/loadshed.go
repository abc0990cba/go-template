package httpapp

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/observ"
)

// Load-shed lanes. Bulk surfaces (reports, analytics, exports) share a
// low-priority budget so a dashboard refresh stampede can never evict the
// interactive traffic a support team needs during an incident.
const (
	laneNormal = "normal"
	laneLow    = "low"
)

// lowPriorityPrefixes route into the low lane; everything else is interactive.
var lowPriorityPrefixes = []string{"/api/reports", "/api/analytics", "/api/export"}

// loadShed admits requests while capacity lasts and rejects the rest with
// 503 + Retry-After. Admission is a single atomic add per lane — no locks on
// the hot path. A low-lane request must stay under lowMax (a fraction of the
// total), so bulk work hits the reserve before interactive traffic does.
type loadShed struct {
	max    int
	lowMax int
	total  atomic.Int64
}

func newLoadShed(capacity int) *loadShed {
	if capacity <= 0 {
		return nil
	}
	return &loadShed{max: capacity, lowMax: capacity * 3 / 4}
}

func (l *loadShed) admit(lane string) bool {
	capacity := int64(l.max)
	if lane == laneLow {
		capacity = int64(l.lowMax)
	}
	// CAS loop: a rejected attempt must not consume budget.
	for {
		current := l.total.Load()
		if current >= capacity {
			return false
		}
		if l.total.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (l *loadShed) release() { l.total.Add(-1) }

// loadShedMiddleware caps concurrent /api requests; nil (cap 0) passes
// everything through.
func (a *App) loadShedMiddleware() gin.HandlerFunc {
	shed := a.shed
	if shed == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		lane := laneNormal
		for _, prefix := range lowPriorityPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				lane = laneLow
				break
			}
		}
		if !shed.admit(lane) {
			observ.LoadShedRejected.WithLabelValues(lane).Inc()
			c.Header("Retry-After", "1")
			a.problem(c, http.StatusServiceUnavailable, "server busy, retry shortly", nil)
			c.Abort()
			return
		}
		observ.LoadShedInFlight.WithLabelValues(lane).Inc()
		defer func() {
			observ.LoadShedInFlight.WithLabelValues(lane).Dec()
			shed.release()
		}()
		c.Next()
	}
}
