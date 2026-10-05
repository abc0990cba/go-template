package httpapp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	platformfilestore "github.com/your-org/go-template/internal/platform/filestore"
	platformrbac "github.com/your-org/go-template/internal/platform/rbac"
	"github.com/your-org/go-template/internal/repository"
)

// newTestContainer builds the service graph over the store with a real
// policy enforcer and the seeded system roles, mirroring production wiring.
func newTestContainer(t *testing.T, store *repository.MemoryStore, events analytics.Sink, logger *slog.Logger) *bootstrap.Container {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	enforcer, err := platformrbac.NewEnforcer(store)
	if err != nil {
		t.Fatalf("rbac enforcer: %v", err)
	}
	services := bootstrap.NewContainer(bootstrap.Deps{
		Auth: store, Users: store, Jobs: store, Reports: store,
		Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, EmailMessages: store, APIKeys: store, Webhooks: store, Tickets: store, Streams: store, MFA: store, Tokens: store, Search: store, Invitations: store, Files: store, Blobs: platformfilestore.NewMemory(), Mailer: nil, Events: events, EventStats: store, RBACEnforcer: enforcer,
	}, logger, cfg)
	if err := services.RBAC.EnsureSystemRoles(t.Context()); err != nil {
		t.Fatalf("seed system roles: %v", err)
	}
	return services
}

// discardLogger is the quiet logger the tests share.
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
