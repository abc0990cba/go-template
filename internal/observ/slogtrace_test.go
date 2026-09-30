package observ

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// newCapturingLogger returns a logger writing JSON lines into buf, wrapped
// with the trace enrichment under test.
func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewTraceHandler(inner))
}

func TestTraceHandlerStampsTraceIDs(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	buf := &bytes.Buffer{}
	newCapturingLogger(buf).InfoContext(ctx, "hello", "k", "v")

	line := buf.String()
	for _, want := range []string{`"trace_id"`, `"span_id"`, `"k":"v"`, `"msg":"hello"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line missing %s: %s", want, line)
		}
	}
}

func TestTraceHandlerPassThroughWithoutSpan(t *testing.T) {
	buf := &bytes.Buffer{}
	newCapturingLogger(buf).InfoContext(context.Background(), "plain")

	line := buf.String()
	if strings.Contains(line, "trace_id") || strings.Contains(line, "span_id") {
		t.Fatalf("unexpected trace attributes without a span: %s", line)
	}
	if !strings.Contains(line, `"msg":"plain"`) {
		t.Fatalf("message lost: %s", line)
	}
}

func TestTraceHandlerSurvivesWithAndGroup(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	buf := &bytes.Buffer{}
	logger := newCapturingLogger(buf).With("component", "test").WithGroup("g")
	logger.InfoContext(ctx, "nested")

	line := buf.String()
	if !strings.Contains(line, `"trace_id"`) {
		t.Fatalf("trace_id lost across With/WithGroup: %s", line)
	}
	if !strings.Contains(line, `"component":"test"`) {
		t.Fatalf("WithAttrs lost: %s", line)
	}
}

func TestSamplerRatio(t *testing.T) {
	cases := []struct {
		ratio float64
		want  sdktrace.Sampler
		desc  string
	}{
		{1.0, sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0)), "always sample"},
		{0.25, sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.25)), "quarter"},
		{0.0, sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0)), "out of range falls back to always-sample"},
		{7.5, sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0)), "above one falls back to always-sample"},
	}
	for _, tc := range cases {
		if got := samplerFor(tc.ratio); got.Description() != tc.want.Description() {
			t.Errorf("%s: got %q, want %q", tc.desc, got.Description(), tc.want.Description())
		}
	}
}
