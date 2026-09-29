package httpapp

import (
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/your-org/go-template/internal/observ"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "app_http_requests_total",
		Help: "HTTP requests by method, route, and status.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "app_http_request_duration_seconds",
		Help:    "HTTP request duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "app_http_requests_in_flight",
		Help: "Requests currently being served.",
	})
)

// tracing starts a server span per request, continuing inbound W3C
// tracecontext from the headers. The span is named after the method when it
// starts and renamed to the matched route pattern once routing has run, so
// the backend groups by endpoint instead of raw URL. With the no-op provider
// (APP_OTLP_ENDPOINT unset) spans are cheap empty objects with no exporter.
func (a *App) tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := observ.Propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := observ.Tracer.Start(ctx, "HTTP "+c.Request.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", c.Request.Method),
				attribute.String("url.path", c.Request.URL.Path),
			))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		status := c.Writer.Status()
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if route := c.FullPath(); route != "" {
			span.SetName(route)
			span.SetAttributes(attribute.String("http.route", route))
		}
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, strconv.Itoa(status))
		}
	}
}

// metrics records RED numbers for every request. The route label is the Gin
// path pattern, not the raw URL, so cardinality stays bounded.
func (a *App) metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		httpInFlight.Inc()
		c.Next()
		httpInFlight.Dec()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		httpRequestsTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// accessLog writes one structured line per request with the request id,
// final status, and duration; slow requests are escalated to warn.
func (a *App) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		attrs := []any{
			"method", c.Request.Method,
			"path", route,
			"status", c.Writer.Status(),
			"duration_ms", duration.Milliseconds(),
			"request_id", c.GetString("request_id"),
		}
		if userID := c.GetString("user_id"); userID != "" {
			attrs = append(attrs, "user_id", userID)
		}
		if duration > 1*time.Second {
			a.logger.Warn("slow request", attrs...)
			return
		}
		a.logger.Info("request", attrs...)
	}
}

// metricsHandler exposes the Prometheus registry.
func metricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}

// versionHandler reports build metadata; release builds stamp it via
// ldflags.
func versionHandler(build observ.Build) gin.HandlerFunc {
	return func(c *gin.Context) {
		version := build.Version
		if version == "" {
			version = "dev"
		}
		c.JSON(http.StatusOK, gin.H{
			"version": version,
			"commit":  build.Commit,
			"date":    build.Date,
		})
	}
}

// pprofHandler mounts net/http/pprof behind the APP_PPROF flag; it must
// never be reachable from untrusted networks.
func pprofHandler() gin.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return gin.WrapH(mux)
}

// openapi serves the raw contract; Scalar on /docs renders it.
func (a *App) openapi(c *gin.Context) {
	c.File("api/openapi/openapi.yaml")
}

// docs renders the Scalar API reference over the contract.
func (a *App) docs(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!doctype html><html><head><title>API Docs</title><script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script></head><body><script id="api-reference" data-url="/openapi.yaml"></script></body></html>`)
}
