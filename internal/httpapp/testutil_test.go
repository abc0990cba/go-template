package httpapp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/platform/rbac"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// newTestContainer builds the service graph over the store with a real
// policy enforcer and the seeded system roles, mirroring production wiring.
func newTestContainer(t *testing.T, store *repository.MemoryStore, events service.EventSink, logger *slog.Logger) *service.Container {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	enforcer, err := rbac.NewEnforcer(store)
	if err != nil {
		t.Fatalf("rbac enforcer: %v", err)
	}
	services := service.NewContainer(service.Deps{
		Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
		Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Tickets: store, Mailer: nil, Events: events, EventStats: store, RBACEnforcer: enforcer,
	}, logger, cfg)
	if err := services.RBAC.EnsureSystemRoles(t.Context()); err != nil {
		t.Fatalf("seed system roles: %v", err)
	}
	return services
}

// discardLogger is the quiet logger the tests share.
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
