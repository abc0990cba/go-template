package observ

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps a slog.Handler so every log line carries the trace
// context of the request it belongs to. This closes the loop between the
// two signals: a structured log line gains trace_id/span_id (jump to Jaeger),
// and spans gain request.id (jump back to the log line) via the middleware
// that sets the attribute.
//
// With tracing disabled (no-op tracer provider) the wrapper is a pass-through
// that adds nothing: logs keep their request_id correlation only.
type TraceHandler struct {
	inner slog.Handler
}

// NewTraceHandler wraps handler with trace-context enrichment.
func NewTraceHandler(handler slog.Handler) *TraceHandler {
	return &TraceHandler{inner: handler}
}

// Enabled delegates to the inner handler.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle appends the current span's IDs as attributes before the inner
// handler formats the record.
func (h *TraceHandler) Handle(ctx context.Context, record slog.Record) error {
	if ctx != nil {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			record.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
			record.AddAttrs(slog.String("span_id", sc.SpanID().String()))
		}
	}
	return h.inner.Handle(ctx, record)
}

// WithAttrs and WithGroup preserve the wrapper so enrichment survives
// logger.With(...) chains.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup preserves the wrapper.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{inner: h.inner.WithGroup(name)}
}
