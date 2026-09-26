package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
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
	return NewContainer(store, store, store, store, store, nil, nil, nil, nil, logger, cfg)
}

func TestRegisterLoginAndLinkProviders(t *testing.T) {
	services := testContainer(t)
	ctx := context.Background()

	user, token, err := services.Auth.Register(ctx, "ada@example.com", "Ada", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, _, err := services.Auth.Login(ctx, "ada@example.com", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	profiles := []model.OAuthProfile{
		{Provider: "google", ProviderSubject: "sub-google-1", Email: "ada@example.com", Name: "Ada"},
		{Provider: "github", ProviderSubject: "42", Email: "ada@example.com", Name: "Ada"},
	}
	for _, profile := range profiles {
		if err := services.Auth.LinkIdentity(ctx, user.ID, profile); err != nil {
			t.Fatalf("link %s: %v", profile.Provider, err)
		}
	}
	identities, err := services.Auth.Identities(ctx, user.ID)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	if len(identities) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(identities))
	}

	// Sessions: the returned token still resolves; logout-all revokes both.
	if _, err := services.Auth.CurrentUser(ctx, token); err != nil {
		t.Fatalf("current user after login: %v", err)
	}
	if err := services.Auth.LogoutAll(ctx, user.ID); err != nil {
		t.Fatalf("logout all: %v", err)
	}
	if _, err := services.Auth.CurrentUser(ctx, token); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("session after logout-all: want ErrNotFound, got %v", err)
	}

	// OAuth state is single use.
	state := model.OAuthState{State: "st-1", Provider: "google", UserID: user.ID, ExpiresAt: time.Now().Add(time.Minute)}
	if err := services.Auth.SaveState(ctx, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if _, err := services.Auth.ConsumeState(ctx, "st-1"); err != nil {
		t.Fatalf("consume state: %v", err)
	}
	if _, err := services.Auth.ConsumeState(ctx, "st-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("state reuse: want ErrNotFound, got %v", err)
	}
}
