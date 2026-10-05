package httpapp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// BenchmarkOpenAPIValidate measures the runtime contract gate on a request
// that carries a body: spec routing plus full schema validation.
func BenchmarkOpenAPIValidate(b *testing.B) {
	v := newSpecValidator(slog.New(slog.NewTextHandler(io.Discard, nil)), "requests")
	if v == nil {
		b.Fatal("validator disabled")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/auth/register", v.middleware, func(c *gin.Context) { c.Status(http.StatusOK) })

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register",
			strings.NewReader(`{"email":"bench@example.com","name":"Ada","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("bench request rejected: %d %s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkLoadShedAdmitRelease measures the uncontended admission path.
func BenchmarkLoadShedAdmitRelease(b *testing.B) {
	shed := newLoadShed(512)
	b.ReportAllocs()
	for b.Loop() {
		if !shed.admit(laneNormal) {
			b.Fatal("uncontended admit must succeed")
		}
		shed.release()
	}
}
