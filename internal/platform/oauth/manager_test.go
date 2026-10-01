// The GitHub and Facebook provider paths of the OAuth manager. Only Google
// and Twitter are exercised through the HTTP layer, so these profiles and
// error paths are covered here against mock provider endpoints.
package oauth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/platform/oauth"
)

// newTokenMux builds the shared token endpoint every mock provider serves.
func newTokenMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"mock-access-token","token_type":"Bearer"}`))
	})
	return mux
}

// newMockGitHub serves the user and emails endpoints with scripted bodies;
// emailsCalled tracks whether the fallback endpoint was needed.
func newMockGitHub(t *testing.T, userBody, emailsBody string) (*httptest.Server, *bool) {
	t.Helper()
	mux := newTokenMux()
	emailsCalled := false
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userBody))
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		emailsCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emailsBody))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &emailsCalled
}

func githubConfig(server *httptest.Server) config.Config {
	return config.Config{
		BaseURL:         "https://app.example.com",
		GitHubClientID:  "test-client",
		GitHubSecret:    "test-secret",
		GitHubAuthURL:   server.URL + "/auth",
		GitHubTokenURL:  server.URL + "/token",
		GitHubUserURL:   server.URL + "/user",
		GitHubEmailURL:  server.URL + "/user/emails",
		ExternalTimeout: 10 * time.Second,
	}
}

func TestExchangeGitHubProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("public email on the user record", func(t *testing.T) {
		server, emailsCalled := newMockGitHub(t,
			`{"id":12345,"login":"octocat","name":"The Octocat","email":"public@example.com","avatar_url":"https://avatars/1.png"}`,
			`[]`)
		manager := oauth.NewManager(githubConfig(server))
		profile, err := manager.ExchangeProfile(ctx, "github", "code", "")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if profile.Provider != "github" || profile.ProviderSubject != "12345" {
			t.Errorf("identity: want github/12345, got %s/%s", profile.Provider, profile.ProviderSubject)
		}
		if profile.Email != "public@example.com" || profile.Name != "The Octocat" || profile.AvatarURL != "https://avatars/1.png" {
			t.Errorf("profile: got %+v", profile)
		}
		if *emailsCalled {
			t.Error("emails endpoint must not be called when the user record has an email")
		}
	})

	t.Run("primary verified email as fallback", func(t *testing.T) {
		server, _ := newMockGitHub(t,
			`{"id":99,"login":"octocat"}`,
			`[{"email":"secondary@example.com","primary":false,"verified":true},{"email":"primary@example.com","primary":true,"verified":true}]`)
		manager := oauth.NewManager(githubConfig(server))
		profile, err := manager.ExchangeProfile(ctx, "github", "code", "")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if profile.Email != "primary@example.com" {
			t.Errorf("email fallback: want primary@example.com, got %q", profile.Email)
		}
		// A missing display name falls back to the login.
		if profile.Name != "octocat" {
			t.Errorf("name fallback: want octocat, got %q", profile.Name)
		}
	})

	t.Run("unverified primary is not trusted", func(t *testing.T) {
		server, _ := newMockGitHub(t,
			`{"id":99,"login":"octocat"}`,
			`[{"email":"unverified@example.com","primary":true,"verified":false}]`)
		manager := oauth.NewManager(githubConfig(server))
		profile, err := manager.ExchangeProfile(ctx, "github", "code", "")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if profile.Email != "" {
			t.Errorf("unverified email: want empty, got %q", profile.Email)
		}
	})

	t.Run("missing id fails", func(t *testing.T) {
		server, _ := newMockGitHub(t, `{"login":"ghost"}`, `[]`)
		manager := oauth.NewManager(githubConfig(server))
		if _, err := manager.ExchangeProfile(ctx, "github", "code", ""); err == nil {
			t.Fatal("missing id: want error")
		}
	})
}

// newMockFacebook serves the graph /me endpoint with a scripted body.
func newMockFacebook(t *testing.T, meBody string) *httptest.Server {
	t.Helper()
	mux := newTokenMux()
	mux.HandleFunc("/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(meBody))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func facebookConfig(server *httptest.Server) config.Config {
	return config.Config{
		BaseURL:          "https://app.example.com",
		FacebookClientID: "test-client",
		FacebookSecret:   "test-secret",
		FacebookAuthURL:  server.URL + "/auth",
		FacebookTokenURL: server.URL + "/token",
		FacebookUserInfo: server.URL + "/me",
		ExternalTimeout:  10 * time.Second,
	}
}

func TestExchangeFacebookProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("full profile with picture", func(t *testing.T) {
		server := newMockFacebook(t,
			`{"id":"fb-1","name":"Ada","email":"ada@example.com","picture":{"data":{"url":"https://graph.facebook.com/pic.jpg"}}}`)
		manager := oauth.NewManager(facebookConfig(server))
		profile, err := manager.ExchangeProfile(ctx, "facebook", "code", "")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if profile.Provider != "facebook" || profile.ProviderSubject != "fb-1" {
			t.Errorf("identity: want facebook/fb-1, got %s/%s", profile.Provider, profile.ProviderSubject)
		}
		if profile.Email != "ada@example.com" || profile.Name != "Ada" {
			t.Errorf("profile: got %+v", profile)
		}
		if profile.AvatarURL != "https://graph.facebook.com/pic.jpg" {
			t.Errorf("avatar: want nested picture.data.url, got %q", profile.AvatarURL)
		}
	})

	t.Run("missing email scope is tolerated", func(t *testing.T) {
		server := newMockFacebook(t, `{"id":"fb-2","name":"No Email"}`)
		manager := oauth.NewManager(facebookConfig(server))
		profile, err := manager.ExchangeProfile(ctx, "facebook", "code", "")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if profile.Email != "" {
			t.Errorf("email: want empty when not granted, got %q", profile.Email)
		}
	})

	t.Run("missing id fails", func(t *testing.T) {
		server := newMockFacebook(t, `{"name":"ghost"}`)
		manager := oauth.NewManager(facebookConfig(server))
		if _, err := manager.ExchangeProfile(ctx, "facebook", "code", ""); err == nil {
			t.Fatal("missing id: want error")
		}
	})
}

func TestManagerGuardsAndPKCE(t *testing.T) {
	cfg := config.Config{BaseURL: "https://app.example.com"}
	manager := oauth.NewManager(cfg)

	// Without credentials nothing is configured and every URL build fails.
	if manager.IsConfigured("google") {
		t.Error("google: want unconfigured without credentials")
	}
	if _, err := manager.AuthCodeURL("google", "state", ""); err == nil {
		t.Error("unconfigured provider: want error")
	}
	if _, err := manager.AuthCodeURL("no.such", "state", ""); err == nil {
		t.Error("unknown provider: want error")
	}

	// PKCE belongs to Twitter alone: the verifier hashes to its challenge,
	// non-PKCE providers mint nothing.
	verifier, challenge := manager.NewCodeVerifier("twitter")
	if verifier == "" {
		t.Fatal("twitter: want a code verifier")
	}
	sum := sha256.Sum256([]byte(verifier))
	if challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Errorf("challenge: want S256(verifier), got %q", challenge)
	}
	if v, c := manager.NewCodeVerifier("google"); v != "" || c != "" {
		t.Errorf("google: want empty PKCE pair, got %q/%q", v, c)
	}
}
