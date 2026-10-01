// Package oauth implements the provider side of OAuth2 login: building
// authorization URLs, exchanging authorization codes, and fetching the
// provider profile of the authenticated user.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"github.com/your-org/go-template/internal/config"
)

// Profile is the normalized profile returned by a provider after the code
// exchange. It lives here because it is the provider wire type; the auth
// feature converts it into its own Identity on link.
type Profile struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatar_url"`
}

// Manager holds the per-provider OAuth2 configuration and an HTTP client
// bounded by the configured external timeout.
type Manager struct {
	httpClient *http.Client
	providers  map[string]Provider
}

// Provider bundles the oauth2 config for one identity provider with the
// endpoints needed to fetch the user profile after the exchange.
type Provider struct {
	Config      *oauth2.Config
	UserInfoURL string
	EmailURL    string
}

// NewManager builds the Google and GitHub provider set from configuration.
// The *_URL config overrides (used by tests with a mock provider) replace
// the well-known endpoints when set.
func NewManager(cfg config.Config) *Manager {
	googleEndpoint, githubEndpoint := google.Endpoint, github.Endpoint
	googleUserInfo := "https://www.googleapis.com/oauth2/v3/userinfo"
	githubUser := "https://api.github.com/user"
	githubEmails := "https://api.github.com/user/emails"
	if cfg.GoogleAuthURL != "" {
		googleEndpoint = oauth2.Endpoint{AuthURL: cfg.GoogleAuthURL, TokenURL: cfg.GoogleTokenURL}
		googleUserInfo = cfg.GoogleUserInfo
	}
	if cfg.GitHubAuthURL != "" {
		githubEndpoint = oauth2.Endpoint{AuthURL: cfg.GitHubAuthURL, TokenURL: cfg.GitHubTokenURL}
		githubUser = cfg.GitHubUserURL
		githubEmails = cfg.GitHubEmailURL
	}
	return &Manager{
		httpClient: &http.Client{
			Timeout:   cfg.ExternalTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		providers: map[string]Provider{
			"google": {
				Config: &oauth2.Config{
					ClientID:     cfg.GoogleClientID,
					ClientSecret: cfg.GoogleSecret,
					RedirectURL:  strings.TrimRight(cfg.BaseURL, "/") + "/auth/google/callback",
					Scopes:       []string{"openid", "email", "profile"},
					Endpoint:     googleEndpoint,
				},
				UserInfoURL: googleUserInfo,
			},
			"github": {
				Config: &oauth2.Config{
					ClientID:     cfg.GitHubClientID,
					ClientSecret: cfg.GitHubSecret,
					RedirectURL:  strings.TrimRight(cfg.BaseURL, "/") + "/auth/github/callback",
					Scopes:       []string{"read:user", "user:email"},
					Endpoint:     githubEndpoint,
				},
				UserInfoURL: githubUser,
				EmailURL:    githubEmails,
			},
		},
	}
}

// IsConfigured reports whether the provider has both a client id and secret.
func (m *Manager) IsConfigured(provider string) bool {
	providerConfig, ok := m.providers[provider]
	return ok && providerConfig.Config.ClientID != "" && providerConfig.Config.ClientSecret != ""
}

// AuthCodeURL returns the redirect target that starts the authorization code
// flow for the provider, embedding state as the CSRF token.
func (m *Manager) AuthCodeURL(provider, state string) (string, error) {
	providerConfig, ok := m.providers[provider]
	if !ok {
		return "", fmt.Errorf("unknown oauth provider %q", provider)
	}
	if !m.IsConfigured(provider) {
		return "", fmt.Errorf("%s oauth is not configured", provider)
	}
	return providerConfig.Config.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

// ExchangeProfile swaps an authorization code for a token and fetches the
// provider profile of the authenticated user.
func (m *Manager) ExchangeProfile(ctx context.Context, provider, code string) (Profile, error) {
	providerConfig, ok := m.providers[provider]
	if !ok {
		return Profile{}, fmt.Errorf("unknown oauth provider %q", provider)
	}
	token, err := providerConfig.Config.Exchange(ctx, code)
	if err != nil {
		return Profile{}, err
	}
	client := providerConfig.Config.Client(ctx, token)
	switch provider {
	case "google":
		return fetchGoogleProfile(ctx, client, providerConfig.UserInfoURL)
	case "github":
		return fetchGitHubProfile(ctx, client, providerConfig.UserInfoURL, providerConfig.EmailURL)
	default:
		return Profile{}, fmt.Errorf("unknown oauth provider %q", provider)
	}
}

func fetchGoogleProfile(ctx context.Context, client *http.Client, url string) (Profile, error) {
	var body struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := getJSON(ctx, client, url, &body); err != nil {
		return Profile{}, err
	}
	if body.Sub == "" {
		return Profile{}, errors.New("google profile is missing subject")
	}
	return Profile{Provider: "google", ProviderSubject: body.Sub, Email: body.Email, Name: body.Name, AvatarURL: body.Picture}, nil
}

func fetchGitHubProfile(ctx context.Context, client *http.Client, userURL, emailURL string) (Profile, error) {
	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := getJSON(ctx, client, userURL, &user); err != nil {
		return Profile{}, err
	}
	email := user.Email
	if email == "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := getJSON(ctx, client, emailURL, &emails); err == nil {
			for _, candidate := range emails {
				if candidate.Primary && candidate.Verified {
					email = candidate.Email
					break
				}
			}
		}
	}
	if user.ID == 0 {
		return Profile{}, errors.New("github profile is missing id")
	}
	name := user.Name
	if name == "" {
		name = user.Login
	}
	return Profile{Provider: "github", ProviderSubject: fmt.Sprintf("%d", user.ID), Email: email, Name: name, AvatarURL: user.AvatarURL}, nil
}

func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("oauth userinfo failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
