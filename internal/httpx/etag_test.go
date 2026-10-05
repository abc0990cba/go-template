package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestComputeETagIsStableAndDistinct(t *testing.T) {
	first := gin.H{"id": "u1", "name": "Ada <admin>"}
	second := gin.H{"id": "u1", "name": "Ada <root>"}

	tag1, err := ComputeETag(first)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	again, err := ComputeETag(first)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if tag1 != again {
		t.Fatalf("tag not stable: %s vs %s", tag1, again)
	}
	other, err := ComputeETag(second)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if tag1 == other {
		t.Fatal("different payloads must hash differently")
	}
	if len(tag1) < 2 || tag1[0] != '"' || tag1[len(tag1)-1] != '"' {
		t.Fatalf("tag must be a quoted string, got %s", tag1)
	}
}

func TestRequireIfMatchMissingHeaderKeepsLastWriteWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("PATCH", "/api/users/u1", nil)

	if !RequireIfMatch(c, gin.H{"id": "u1"}) {
		t.Fatal("absent If-Match must not block the handler")
	}
}

func TestRequireIfMatchAcceptsCurrentTag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := gin.H{"id": "u1", "name": "Ada"}
	tag, err := ComputeETag(current)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	for _, header := range []string{tag, `W/` + tag, "*", tag + `, "other"`} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("PATCH", "/api/users/u1", nil)
		c.Request.Header.Set("If-Match", header)
		if !RequireIfMatch(c, current) {
			t.Fatalf("header %q must match the current state", header)
		}
	}
}

func TestRequireIfMatchRejectsStaleTag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("PATCH", "/api/users/u1", nil)
	c.Request.Header.Set("If-Match", `"deadbeef000000000000000000000000"`)

	if RequireIfMatch(c, gin.H{"id": "u1", "name": "Ada"}) {
		t.Fatal("stale tag must not pass")
	}
	if rec.Code != 412 {
		t.Fatalf("stale tag: want 412, got %d", rec.Code)
	}
}
