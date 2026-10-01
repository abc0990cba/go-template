package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/repository"
)

// The repository package imports auth for the store types, so this test is
// the external package (the mfa/tokens precedent).
func newService(t *testing.T) *auth.Service {
	t.Helper()
	return auth.NewService(repository.NewMemoryStore(), nil, time.Hour)
}

func TestRegisterLoginAndLinkProviders(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	user, token, err := svc.Register(ctx, "ada@example.com", "Ada", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, _, err := svc.Login(ctx, "ada@example.com", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	profiles := []oauth.Profile{
		{Provider: "google", ProviderSubject: "sub-google-1", Email: "ada@example.com", Name: "Ada"},
		{Provider: "github", ProviderSubject: "42", Email: "ada@example.com", Name: "Ada"},
	}
	for _, profile := range profiles {
		if err := svc.LinkIdentity(ctx, user.ID, profile); err != nil {
			t.Fatalf("link %s: %v", profile.Provider, err)
		}
	}
	identities, err := svc.Identities(ctx, user.ID)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	if len(identities) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(identities))
	}

	// Sessions: the returned token still resolves; logout-all revokes both.
	if _, err := svc.CurrentUser(ctx, token); err != nil {
		t.Fatalf("current user after login: %v", err)
	}
	if err := svc.LogoutAll(ctx, user.ID); err != nil {
		t.Fatalf("logout all: %v", err)
	}
	if _, err := svc.CurrentUser(ctx, token); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("session after logout-all: want ErrNotFound, got %v", err)
	}

	// OAuth state is single use.
	state := auth.OAuthState{State: "st-1", Provider: "google", UserID: user.ID, ExpiresAt: time.Now().Add(time.Minute)}
	if err := svc.SaveState(ctx, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if _, err := svc.ConsumeState(ctx, "st-1"); err != nil {
		t.Fatalf("consume state: %v", err)
	}
	if _, err := svc.ConsumeState(ctx, "st-1"); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("state reuse: want ErrNotFound, got %v", err)
	}
}
