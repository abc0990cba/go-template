// The kernel is the shared domain vocabulary; these tests pin the ID
// format, the validation-error contract, and the enrichment cursor format
// that both store adapters must agree on.
package kernel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewIDFormatAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := NewID()
		// 16 random bytes, base64url without padding: 22 characters of
		// the URL-safe alphabet only.
		if len(id) != 22 {
			t.Fatalf("id length: want 22, got %d (%q)", len(id), id)
		}
		if _, err := base64.RawURLEncoding.DecodeString(id); err != nil {
			t.Fatalf("id %q is not base64url: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("id collision after %d draws: %q", i+1, id)
		}
		seen[id] = true
	}
}

func TestInvalidAndIsValidationError(t *testing.T) {
	err := Invalid("count must be positive")
	if err.Error() != "count must be positive" {
		t.Errorf("message: got %q", err.Error())
	}
	if !IsValidationError(err) {
		t.Error("direct validation error: want true")
	}
	if !IsValidationError(fmt.Errorf("enqueue: %w", err)) {
		t.Error("wrapped validation error: want true")
	}
	if IsValidationError(ErrNotFound) {
		t.Error("sentinel: want false")
	}
	if IsValidationError(nil) {
		t.Error("nil: want false")
	}
}

func TestSplitEnrichmentCursor(t *testing.T) {
	// Round trip: (created_at nanos, sequence).
	at := time.Unix(0, 1767225600123456789)
	cursor := fmt.Sprintf("%d:%d", at.UnixNano(), int64(42))
	got, seq, ok := SplitEnrichmentCursor(cursor)
	if !ok {
		t.Fatalf("valid cursor %q: want ok", cursor)
	}
	if !got.Equal(at) || seq != 42 {
		t.Errorf("round trip: want (%v, 42), got (%v, %d)", at, got, seq)
	}

	for _, bad := range []string{"", "noseparator", "abc:1", "1:abc", "1:", ":2", "1:2:3"} {
		if _, _, ok := SplitEnrichmentCursor(bad); ok {
			t.Errorf("cursor %q: want not ok", bad)
		}
	}
}

func TestSentinelsAreDistinct(t *testing.T) {
	sentinels := []error{ErrNotFound, ErrConflict, ErrForbidden, ErrInvalidCredentials}
	seen := map[string]bool{}
	for _, sentinel := range sentinels {
		if seen[sentinel.Error()] {
			t.Errorf("duplicate sentinel message: %q", sentinel.Error())
		}
		seen[sentinel.Error()] = true
	}
	// Wrapping preserves the identity the HTTP mapping relies on.
	if !errors.Is(fmt.Errorf("x: %w", ErrConflict), ErrConflict) {
		t.Error("wrapped ErrConflict: want errors.Is match")
	}
}
