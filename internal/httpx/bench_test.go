package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

var benchPayload = gin.H{
	"id": "usr_01HQX2Y3Z4", "email": "ada@example.com", "name": "Ada Lovelace",
	"external_ref": "ext-4242", "created_at": "2026-01-01T00:00:00Z",
}

func BenchmarkComputeETag(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ComputeETag(benchPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProblem(b *testing.B) {
	gin.SetMode(gin.TestMode)
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("GET", "/api/x", nil)
		Problem(c, 400, "validation failed", nil)
	}
}
