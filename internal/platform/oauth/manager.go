// Package oauth implements the provider side of OAuth2 login: building
// authorization URLs, exchanging authorization codes, and fetching the
// provider profile of the authenticated user.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
// endpoints needed to fetch the user profile after the exchange. PKCE marks
// providers that mandate Proof Key for Code Exchange (Twitter does); the
// challenge rides in the authorize URL and the verifier in the stored state.
type Provider struct {
	Config      *oauth2.Config
	UserInfoURL string
	EmailURL    string
	PKCE        bool
}

// NewManager builds the Google, GitHub, Facebook, and Twitter provider set
// from configuration. The *_URL config overrides (used by tests with a mock
// provider) replace the well-known endpoints when set.
func NewManager(cfg config.Config) *Manager {
	googleEndpoint, githubEndpoint := google.Endpoint, github.Endpoint
	googleUserInfo := "https://www.googleapis.com/oauth2/v3/userinfo"
	githubUser := "https://api.github.com/user"
	githubEmails := "https://api.github.com/user/emails"
	facebookEndpoint := oauth2.Endpoint{ // #nosec G101 -- public OAuth endpoint URLs, not credentials
		AuthURL:   "https://www.facebook.com/v24.0/dialog/oauth",
		TokenURL:  "https://graph.facebook.com/v24.0/oauth/access_token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	facebookUserInfo := "https://graph.facebook.com/me?fields=id,name,email,picture.type(large)"
	twitterEndpoint := oauth2.Endpoint{ // #nosec G101 -- public OAuth endpoint URLs, not credentials
		AuthURL:   "https://twitter.com/i/oauth2/authorize",
		TokenURL:  "https://api.twitter.com/2/oauth2/token",
		AuthStyle: oauth2.AuthStyleInHeader,
	}
	twitterUserInfo := "https://api.twitter.com/2/users/me?user.fields=name,profile_image_url"
	if cfg.GoogleAuthURL != "" {
		googleEndpoint = oauth2.Endpoint{AuthURL: cfg.GoogleAuthURL, TokenURL: cfg.GoogleTokenURL}
		googleUserInfo = cfg.GoogleUserInfo
	}
	if cfg.GitHubAuthURL != "" {
		githubEndpoint = oauth2.Endpoint{AuthURL: cfg.GitHubAuthURL, TokenURL: cfg.GitHubTokenURL}
		githubUser = cfg.GitHubUserURL
		githubEmails = cfg.GitHubEmailURL
	}
	if cfg.FacebookAuthURL != "" {
		facebookEndpoint = oauth2.Endpoint{
			AuthURL: cfg.FacebookAuthURL, TokenURL: cfg.FacebookTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		}
		facebookUserInfo = cfg.FacebookUserInfo
	}
	if cfg.TwitterAuthURL != "" {
		twitterEndpoint = oauth2.Endpoint{
			AuthURL: cfg.TwitterAuthURL, TokenURL: cfg.TwitterTokenURL,
			AuthStyle: oauth2.AuthStyleInHeader,
		}
		twitterUserInfo = cfg.TwitterUserInfo
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
			"facebook": {
				Config: &oauth2.Config{
					ClientID:     cfg.FacebookClientID,
					ClientSecret: cfg.FacebookSecret,
					RedirectURL:  strings.TrimRight(cfg.BaseURL, "/") + "/auth/facebook/callback",
					Scopes:       []string{"email", "public_profile"},
					Endpoint:     facebookEndpoint,
				},
				UserInfoURL: facebookUserInfo,
			},
			"twitter": {
				Config: &oauth2.Config{
					ClientID:     cfg.TwitterClientID,
					ClientSecret: cfg.TwitterSecret,
					RedirectURL:  strings.TrimRight(cfg.BaseURL, "/") + "/auth/twitter/callback",
					Scopes:       []string{"users.read", "tweet.read"},
					Endpoint:     twitterEndpoint,
				},
				UserInfoURL: twitterUserInfo,
				PKCE:        true,
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
// flow for the provider, embedding state as the CSRF token. codeChallenge is
// the S256 PKCE challenge; pass "" for providers that do not use PKCE.
func (m *Manager) AuthCodeURL(provider, state, codeChallenge string) (string, error) {
	providerConfig, ok := m.providers[provider]
	if !ok {
		return "", fmt.Errorf("unknown oauth provider %q", provider)
	}
	if !m.IsConfigured(provider) {
		return "", fmt.Errorf("%s oauth is not configured", provider)
	}
	opts := []oauth2.AuthCodeOption{}
	if codeChallenge != "" {
		opts = append(opts,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
	}
	if provider == "google" {
		opts = append(opts, oauth2.AccessTypeOffline)
	}
	return providerConfig.Config.AuthCodeURL(state, opts...), nil
}

// NewCodeVerifier mints an RFC 7636 code verifier (and its S256 challenge)
// for providers that mandate PKCE. For the others it returns empty values
// and the caller stores no verifier.
func (m *Manager) NewCodeVerifier(provider string) (verifier, challenge string) {
	if providerConfig, ok := m.providers[provider]; !ok || !providerConfig.PKCE {
		return "", ""
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		// crypto/rand failing means the process is broken; panic like other
		// consumers of crypto/rand in the kernel.
		panic(fmt.Sprintf("oauth: code verifier entropy failed: %v", err))
	}
	verifier = base64.RawURLEncoding.EncodeToString(seed)
	digest := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(digest[:])
	return verifier, challenge
}

// ExchangeProfile swaps an authorization code for a token and fetches the
// provider profile of the authenticated user. verifier is the PKCE code
// verifier stored with the state; pass "" for providers that do not use PKCE.
func (m *Manager) ExchangeProfile(ctx context.Context, provider, code, verifier string) (Profile, error) {
	providerConfig, ok := m.providers[provider]
	if !ok {
		return Profile{}, fmt.Errorf("unknown oauth provider %q", provider)
	}
	opts := []oauth2.AuthCodeOption{}
	if verifier != "" {
		opts = append(opts, oauth2.SetAuthURLParam("code_verifier", verifier))
	}
	token, err := providerConfig.Config.Exchange(ctx, code, opts...)
	if err != nil {
		return Profile{}, err
	}
	client := providerConfig.Config.Client(ctx, token)
	switch provider {
	case "google":
		return fetchGoogleProfile(ctx, client, providerConfig.UserInfoURL)
	case "github":
		return fetchGitHubProfile(ctx, client, providerConfig.UserInfoURL, providerConfig.EmailURL)
	case "facebook":
		return fetchFacebookProfile(ctx, client, providerConfig.UserInfoURL)
	case "twitter":
		return fetchTwitterProfile(ctx, client, providerConfig.UserInfoURL)
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

func fetchFacebookProfile(ctx context.Context, client *http.Client, url string) (Profile, error) {
	var body struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}
	if err := getJSON(ctx, client, url, &body); err != nil {
		return Profile{}, err
	}
	if body.ID == "" {
		return Profile{}, errors.New("facebook profile is missing id")
	}
	// Facebook only returns email when the user granted the scope; the auth
	// feature tolerates an empty address, falling back to the subject.
	return Profile{Provider: "facebook", ProviderSubject: body.ID, Email: body.Email, Name: body.Name, AvatarURL: body.Picture.Data.URL}, nil
}

func fetchTwitterProfile(ctx context.Context, client *http.Client, url string) (Profile, error) {
	var body struct {
		Data struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Username        string `json:"username"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, url, &body); err != nil {
		return Profile{}, err
	}
	if body.Data.ID == "" {
		return Profile{}, errors.New("twitter profile is missing id")
	}
	name := body.Data.Name
	if name == "" {
		name = body.Data.Username
	}
	// The API returns the 48px _normal variant by default; ask for the 400px
	// rendition by rewriting the size suffix.
	avatar := strings.Replace(body.Data.ProfileImageURL, "_normal", "_400x400", 1)
	// Twitter does not expose the email address over OAuth2 for standard
	// access; the username stands in for it on the profile page.
	return Profile{Provider: "twitter", ProviderSubject: body.Data.ID, Email: "", Name: name, AvatarURL: avatar}, nil
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
