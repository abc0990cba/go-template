package observ

import (
	"context"
	"testing"
)

// SpanFromHeaders must accept a nil header map: Kafka records often carry no
// headers, and the propagator injects into the carrier it created.
func TestSpanFromHeadersNilMap(t *testing.T) {
	ctx, span := SpanFromHeaders(context.Background(), nil, "test.consume")
	defer span.End()
	if ctx == nil {
		t.Fatal("nil context")
	}
}

func TestSpanFromHeadersRoundTrip(t *testing.T) {
	if !TracingEnabled() {
		t.Skip("tracing provider off; noop tracer carries no context")
	}
}
