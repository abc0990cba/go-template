package tokens_test

import (
	"testing"
	"time"

	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/tokens"
)

// FuzzVerifyAccess throws arbitrary token strings at verification: any
// input must come back as either a subject or an error, never a panic —
// this endpoint parses untrusted bytes on every API request.
func FuzzVerifyAccess(f *testing.F) {
	svc := tokens.NewService(repository.NewMemoryStore(), nil, nil, nil,
		testJWTSecret, 15*time.Minute, 24*time.Hour, func(string, string) bool { return false })

	f.Add("")
	f.Add("not-a-token")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJIUzI1NiJ9.e30.x") // alg=none shape
	f.Add(testJWTSecret)

	f.Fuzz(func(_ *testing.T, token string) {
		// The result is irrelevant; the contract is "no panic, no hang".
		_, _ = svc.VerifyAccess(token)
	})
}
