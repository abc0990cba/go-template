package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// FuzzProblem throws arbitrary strings through the problem+json writer:
// any status, title, and detail must render without panicking.
func FuzzProblem(f *testing.F) {
	f.Add(400, "title", "detail")
	f.Add(500, "internal error", "stack\nwith\nnewlines")
	f.Add(200, "<script>alert(1)</script>", "\x00\x01binary")

	f.Fuzz(func(_ *testing.T, status int, title, detail string) {
		// Production statuses come from the HTTPError map (100..599);
		// httptest's recorder rejects anything outside that range.
		code := 100 + ((status%500)+500)%500 // abs-modulo: Go keeps the sign of %
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("GET", "/api/fuzz", nil)
		Problem(c, code, title, errorString(detail))
	})
}

type errorString string

func (e errorString) Error() string { return string(e) }

// FuzzIfMatchParsing throws arbitrary If-Match headers at the comparator:
// it must classify without panicking, and "*" matches everything.
func FuzzIfMatchParsing(f *testing.F) {
	f.Add(`"deadbeef"`)
	f.Add("*")
	f.Add(`W/"weak"`)
	f.Add(`"a", "b" , "c"`)
	f.Add("")
	f.Add(`",,,"`)

	f.Fuzz(func(_ *testing.T, header string) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("PATCH", "/api/x", nil)
		c.Request.Header.Set("If-Match", header)
		// A quoted-empty tag never matches a real one; "*" always does.
		matched := RequireIfMatch(c, struct{}{})
		if header == "*" && !matched {
			panic("If-Match: * must always match, got header " + header)
		}
	})
}
