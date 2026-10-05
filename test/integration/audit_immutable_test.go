//go:build integration

package integration

// The audit trail's immutability is enforced by the database, not just by
// code paths: UPDATE and DELETE on audit_log answer an exception for the
// application role (migration 000022), retention runs exclusively through
// the audit_prune function, and demo rows keep their reset path.

import (
	"context"
	"os"
	"testing"
	"time"

	"strings"

	"github.com/jackc/pgx/v5"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/platform/postgres"
)

func TestAuditLogIsImmutableAtDBLevel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		msg := err.Error()
		for _, marker := range []string{"pull access denied", "manifest unknown", "No such image", "not found"} {
			if strings.Contains(msg, marker) {
				t.Skipf("postgres image unavailable in this environment: %v", err)
			}
		}
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	store, err := postgres.Open(ctx, dsn, os.DirFS("../../migrations/postgres"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.RecordAudit(ctx, audit.Entry{ActorID: "actor-1", ActorEmail: "a@example.com", Action: "ticket.created", ResourceType: "ticket", ResourceID: "ticket-1", Diff: "{}", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("record: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("raw connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// UPDATE is refused.
	if _, err := conn.Exec(ctx, `UPDATE audit_log SET action = 'tampered' WHERE actor_id = 'actor-1'`); err == nil {
		t.Fatal("audit_log UPDATE must be trigger-blocked")
	}
	// DELETE is refused.
	if _, err := conn.Exec(ctx, `DELETE FROM audit_log WHERE actor_id = 'actor-1'`); err == nil {
		t.Fatal("audit_log DELETE must be trigger-blocked")
	}

	// Retention works only through the sanctioned function.
	pruned, err := store.PruneAuditLog(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("prune function: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("prune: want 1 row, got %d", pruned)
	}

	// Demo rows keep their reset path.
	if err := store.RecordAudit(ctx, audit.Entry{ActorID: "demo-seed", Action: "demo.seeded", Diff: "{}", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("record demo: %v", err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM audit_log WHERE actor_id LIKE 'demo-%'`); err != nil {
		t.Fatalf("demo rows must stay deletable: %v", err)
	}
}
