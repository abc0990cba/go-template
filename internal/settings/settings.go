package settings

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Setting is one stored key-value pair; Value holds the JSON literal of
// the definition's type.
type Setting struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	UpdatedBy   string    `json:"updated_by"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Definition describes one known setting: its type, default, and
// whether the value is safe to expose without authentication (the register
// page needs to know whether signups are open).
type Definition struct {
	Key     string `json:"key"`
	Type    string `json:"type"` // "bool" | "int" | "string"
	Default string `json:"default"`
	Group   string `json:"group"`
	Label   string `json:"label"`
	Public  bool   `json:"public"`
	IsFlag  bool   `json:"is_flag"`
}

// View is one catalog row merged with its current value.
type View struct {
	Definition
	Value     string    `json:"value"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Definitions is the catalog the settings page renders and the code
// reads through typed accessors. Unknown keys cannot be created at runtime.
var Definitions = []Definition{
	{Key: "registration.enabled", Type: "bool", Default: "true", Group: "Access", Label: "Allow self-registration", Public: true, IsFlag: true},
	{Key: "email_verification.enabled", Type: "bool", Default: "false", Group: "Access", Label: "Require email verification", Public: true, IsFlag: true},
	{Key: "password_reset.enabled", Type: "bool", Default: "true", Group: "Access", Label: "Allow password reset by email", Public: true, IsFlag: true},
	{Key: "demo_mode", Type: "bool", Default: "true", Group: "Access", Label: "Show demo account quick sign-in", Public: true, IsFlag: true},
	{Key: "webhooks.enabled", Type: "bool", Default: "true", Group: "Integrations", Label: "Deliver outgoing webhooks", IsFlag: true},
	{Key: "maintenance.enabled", Type: "bool", Default: "false", Group: "Operations", Label: "Maintenance mode (API returns 503)", Public: true, IsFlag: true},
	{Key: "mfa.required", Type: "bool", Default: "false", Group: "Access", Label: "Require two-factor authentication at login", IsFlag: true},
	{Key: "audit.retention_days", Type: "int", Default: "365", Group: "Compliance", Label: "Audit retention (days)"},
	{Key: "sessions.max_per_user", Type: "int", Default: "0", Group: "Access", Label: "Max sessions per user (0 = unlimited)"},
}

// Store persists the settings key-value store.
type Store interface {
	UpsertSetting(ctx context.Context, setting Setting) error
	GetSetting(ctx context.Context, key string) (Setting, error)
	ListSettings(ctx context.Context) ([]Setting, error)
}

// Service serves the settings catalog with a small read-through
// cache: typed accessors on the hot path, invalidation on write.
type Service struct {
	store Store
	cache map[string]Setting
	mu    sync.RWMutex
	seen  time.Time
	ttl   time.Duration
}

// NewService builds the service with a 30s cache TTL.
func NewService(store Store) *Service {
	return &Service{store: store, ttl: 30 * time.Second}
}

// Catalog returns the known settings with their stored values merged in.
func (s *Service) Catalog(ctx context.Context) ([]View, error) {
	stored, err := s.all(ctx)
	byKey := map[string]Setting{}
	if err == nil {
		for _, setting := range stored {
			byKey[setting.Key] = setting
		}
	}
	out := make([]View, 0, len(Definitions))
	for _, def := range Definitions {
		value := def.Default
		updatedBy := ""
		var updatedAt time.Time
		if existing, ok := byKey[def.Key]; ok {
			value = existing.Value
			updatedBy = existing.UpdatedBy
			updatedAt = existing.UpdatedAt
		}
		out = append(out, View{Definition: def, Value: value, UpdatedBy: updatedBy, UpdatedAt: updatedAt})
	}
	return out, nil
}

// PublicSettings returns the subset of settings safe to expose without a
// session (the UI needs to know whether registration is open before login).
func (s *Service) PublicSettings(ctx context.Context) map[string]string {
	out := map[string]string{}
	for _, def := range Definitions {
		if !def.Public {
			continue
		}
		out[def.Key] = s.Get(ctx, def.Key, def.Default)
	}
	return out
}

// Get returns the stored value of a key, or the definition default.
func (s *Service) Get(ctx context.Context, key, fallback string) string {
	stored, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return fallback
	}
	return stored.Value
}

// Bool reads a boolean setting (stored as a JSON boolean).
func (s *Service) Bool(ctx context.Context, key string) bool {
	value := s.Get(ctx, key, s.definition(key).Default)
	var b bool
	return json.Unmarshal([]byte(value), &b) == nil && b
}

// Int reads an integer setting (stored as a JSON number).
func (s *Service) Int(ctx context.Context, key string) int {
	value := s.Get(ctx, key, s.definition(key).Default)
	var n int
	_ = json.Unmarshal([]byte(value), &n)
	return n
}

// Set validates and stores one setting; only catalog keys are accepted and
// the stored value is the JSON literal of the definition's type.
func (s *Service) Set(ctx context.Context, key, value, updatedBy string) error {
	def := s.definition(key)
	if def.Key == "" {
		return kernel.Invalid("unknown setting " + key)
	}
	var storedValue string
	switch def.Type {
	case "bool":
		if value != "true" && value != "false" {
			return kernel.Invalid("setting expects true or false")
		}
		storedValue = value // JSON boolean literal
	case "int":
		var n int
		if err := json.Unmarshal([]byte(value), &n); err != nil {
			return kernel.Invalid("setting expects an integer")
		}
		storedValue = value // JSON number literal
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return kernel.Invalid("setting must be a string")
		}
		storedValue = string(raw)
	}
	stored := Setting{Key: key, Value: storedValue, Description: def.Label, UpdatedBy: updatedBy, UpdatedAt: time.Now()}
	if err := s.store.UpsertSetting(ctx, stored); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *Service) definition(key string) Definition {
	for _, def := range Definitions {
		if def.Key == key {
			return def
		}
	}
	return Definition{}
}

// all returns the stored settings, served through a cache that tolerates a
// missing store (dev mode): defaults then apply.
func (s *Service) all(ctx context.Context) ([]Setting, error) {
	s.mu.RLock()
	if s.cache != nil && time.Since(s.seen) < s.ttl {
		out := cachedValues(s.cache)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache != nil && time.Since(s.seen) < s.ttl {
		return cachedValues(s.cache), nil
	}
	settings, err := s.store.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	s.cache = map[string]Setting{}
	for _, setting := range settings {
		s.cache[setting.Key] = setting
	}
	s.seen = time.Now()
	return settings, nil
}

func cachedValues(cache map[string]Setting) []Setting {
	out := make([]Setting, 0, len(cache))
	for _, setting := range cache {
		out = append(out, setting)
	}
	return out
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}
