package observ

// RedactHandler wraps a slog.Handler and scrubs sensitive values before
// they reach any sink: attribute keys that name credentials (password,
// token, secret, authorization, cookie, and any key mentioning an email
// address's subject) have their values replaced, and bare email addresses
// are masked inside string attribute values. Messages are left alone —
// they are static strings in this codebase, and rewriting every line on
// the hot path buys nothing.

import (
	"context"
	"log/slog"
	"strings"
)

// sensitiveKeys are attribute keys whose values never belong in a log line,
// matched case-insensitively as substrings ("refresh_token", "SMTPPassword").
var sensitiveKeys = [...]string{
	"password", "token", "secret", "authorization", "cookie", "credential", "recovery",
}

// NewRedactHandler wraps handler with PII scrubbing.
func NewRedactHandler(handler slog.Handler) slog.Handler {
	return &redactHandler{inner: handler}
}

type redactHandler struct {
	inner slog.Handler
}

func (h *redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &redactHandler{inner: h.inner.WithAttrs(h.scrub(attrs))}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name)}
}

// Handle rewrites the record's attributes and delegates.
func (h *redactHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	scrubbed := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	scrubbed.AddAttrs(h.scrub(attrs)...)
	return h.inner.Handle(ctx, scrubbed)
}

// scrub masks values of sensitive keys and email addresses in string values.
func (h *redactHandler) scrub(attrs []slog.Attr) []slog.Attr {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if attr.Value.Kind() == slog.KindGroup {
			group := attr.Value.Group()
			attr = slog.Attr{Key: attr.Key, Value: slog.GroupValue(h.scrub(group)...)}
			out = append(out, attr)
			continue
		}
		if isSensitiveKey(attr.Key) {
			out = append(out, slog.String(attr.Key, "[REDACTED]"))
			continue
		}
		if attr.Value.Kind() == slog.KindString {
			if value := maskEmails(attr.Value.String()); value != attr.Value.String() {
				attr = slog.String(attr.Key, value)
			}
		}
		out = append(out, attr)
	}
	return out
}

// isSensitiveKey reports whether the key names something secret-like.
func isSensitiveKey(key string) bool {
	lowered := strings.ToLower(key)
	for _, fragment := range sensitiveKeys {
		if strings.Contains(lowered, fragment) {
			return true
		}
	}
	return false
}

// maskEmails replaces every email-looking substring with a masked form:
// the local part reduces to its first character, the domain stays — enough
// to correlate accounts across lines without publishing the address. The
// common case (no '@' at all) is a single pass with no allocation.
// Known limitation: overlapping email shapes ("0@0@0") mask only the
// leftmost match — the scanner never rewrites bytes it already emitted.
func maskEmails(value string) string {
	first := strings.IndexByte(value, '@')
	if first < 1 {
		return value
	}
	var b strings.Builder
	start := 0
	for i := first; i < len(value); {
		at := strings.IndexByte(value[i:], '@')
		if at < 0 {
			break
		}
		at += i
		localStart := localPartStart(value, at)
		if localStart < start {
			// The local part reaches back into a region this pass already
			// masked ("0@0@0": the second '@' sees the first domain's byte
			// as its local part); treat it as already-consumed.
			localStart = start
		}
		end := domainEnd(value, at)
		if localStart < at && end > at+1 {
			b.WriteString(value[start:localStart])
			b.WriteByte(value[localStart])
			b.WriteString("***@")
			b.WriteString(value[at+1 : end])
			start = end
			i = end
			continue
		}
		// Not an email-shaped region: leave it for the trailing copy.
		i = at + 1
	}
	b.WriteString(value[start:])
	return b.String()
}

// localPartStart walks back from '@' to the start of the local part.
func localPartStart(value string, at int) int {
	i := at
	for i > 0 && isEmailByte(value[i-1]) {
		i--
	}
	return i
}

// domainEnd walks forward past the domain (letters, digits, dots, dashes).
func domainEnd(value string, at int) int {
	i := at + 1
	for i < len(value) && (isEmailByte(value[i]) || value[i] == '.') {
		i++
	}
	return i
}

func isEmailByte(b byte) bool {
	return b == '-' || b == '_' || b == '+' || b == '.' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
