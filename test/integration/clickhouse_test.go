//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	tcch "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	ch "github.com/your-org/go-template/internal/platform/clickhouse"
)

// startClickHouse starts a throwaway warehouse and applies the migrations.
// It skips the test when the image cannot be started (for example, a slow
// pull on a constrained network); CI pulls the image beforehand.
func startClickHouse(t *testing.T, ctx context.Context) (*ch.Client, string, func()) {
	t.Helper()
	container, err := tcch.Run(ctx,
		"clickhouse/clickhouse-server:24.8-alpine",
		tcch.WithDatabase("app"),
		tcch.WithUsername("app"),
		tcch.WithPassword("app"),
	)
	if err != nil {
		t.Skipf("start clickhouse container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("clickhouse host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("clickhouse port: %v", err)
	}

	addr := host + ":" + port.Port()
	client, err := ch.Open(ctx, addr, "app", "app", "app", os.DirFS("../../migrations/clickhouse"))
	if err != nil {
		t.Fatalf("open clickhouse: %v", err)
	}
	return client, addr, func() { _ = client.Close(); _ = container.Terminate(ctx) }
}

// TestClickHouseEventPipeline checks the sink→flush→aggregate round trip.
func TestClickHouseEventPipeline(t *testing.T) {
	ctx := context.Background()
	client, _, cleanup := startClickHouse(t, ctx)
	defer cleanup()

	// Buffer with a small size so AddEvent flushes synchronously.
	buffer := client.NewEventBuffer(testLogger(), 2, time.Hour)
	buffer.AddEvent(context.Background(), "user_registered", "")
	buffer.AddEvent(context.Background(), "user_registered", "") // fills the batch → flush
	buffer.AddEvent(context.Background(), "user_logged_in", "")  // stays buffered
	if err := buffer.Close(); err != nil {                   // flushes the remainder
		t.Fatalf("close buffer: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		counts, err := client.EventCounts(ctx)
		if err == nil {
			if counts["user_registered"] == 2 && counts["user_logged_in"] == 1 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("event counts: want registered=2 logged_in=1, got %v", counts)
			}
		} else if time.Now().After(deadline) {
			t.Fatalf("event counts query: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestClickHouseMigrationIsIdempotent reopens against the same warehouse.
func TestClickHouseMigrationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	_, addr, cleanup := startClickHouse(t, ctx)
	defer cleanup()

	// Opening a second client against the same warehouse must not fail on
	// the CREATE TABLE migration.
	migrations := os.DirFS("../../migrations/clickhouse")
	second, err := ch.Open(ctx, addr, "app", "app", "app", migrations)
	if err != nil {
		t.Fatalf("reopen clickhouse: %v", err)
	}
	_ = second.Close()
}
