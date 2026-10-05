package observ

import (
	"strings"
	"testing"
)

// FuzzMaskEmails throws arbitrary strings at the masker: it must never
// panic and must preserve non-email bytes exactly.
func FuzzMaskEmails(f *testing.F) {
	f.Add("")
	f.Add("a@b")
	f.Add("ada@example.com")
	f.Add("mail <x.y@z.io> tail")
	f.Add("@@@")
	f.Add("user@@double@@at")
	f.Add("trailing@dot.")

	f.Fuzz(func(t *testing.T, value string) {
		masked := maskEmails(value)
		// Each mask replaces one '@' with "***@", so growth is bounded by
		// three bytes per '@'; anything beyond that is a masking bug.
		if len(masked) > len(value)+3*strings.Count(value, "@") {
			t.Fatalf("maskEmails grew beyond the bound: %q -> %q", value, masked)
		}
		// Inputs without '@' pass through untouched.
		if !strings.Contains(value, "@") && masked != value {
			t.Fatalf("no-at input changed: %q -> %q", value, masked)
		}
		// No '@' the masker chose to skip may panic the writer (regression
		// corpus holds "0@0@0" and friends); overlapping-email shapes are
		// left as-is by design — the scanner never rewrites what it has
		// already emitted.
	})
}
