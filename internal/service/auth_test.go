package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/repository"
)

func testContainer(t *testing.T) *Container {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := repository.NewMemoryStore()
	return NewContainer(store, store, store, store, store, nil, logger, cfg)
}

func TestRegisterLoginAndLinkProviders(t *testing.T) {
	services := testContainer(t)
	ctx := context.Background()

	user, _, err := services.Auth.Register(ctx, "ada@example.com", "Ada", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, _, err := services.Auth.Login(ctx, "ada@example.com", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := services.Auth.LinkProvider(ctx, user.ID, "google"); err != nil {
		t.Fatalf("link google: %v", err)
	}
	if err := services.Auth.LinkProvider(ctx, user.ID, "github"); err != nil {
		t.Fatalf("link github: %v", err)
	}
	identities, err := services.Auth.Identities(ctx, user.ID)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	if len(identities) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(identities))
	}
}
