package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/your-org/go-template/internal/model"
)

// SettingDefinition describes one known setting: its type, default, and
// whether the value is safe to expose without authentication (the register
// page needs to know whether signups are open).
type SettingDefinition struct {
	Key     string `json:"key"`
	Type    string `json:"type"` // "bool" | "int" | "string"
	Default string `json:"default"`
	Group   string `json:"group"`
	Label   string `json:"label"`
	Public  bool   `json:"public"`
	IsFlag  bool   `json:"is_flag"`
}

// SettingView is one catalog row merged with its current value.
type SettingView struct {
	SettingDefinition
	Value     string    `json:"value"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// SettingDefinitions is the catalog the settings page renders and the code
// reads through typed accessors. Unknown keys cannot be created at runtime.
var SettingDefinitions = []SettingDefinition{
	{Key: "registration.enabled", Type: "bool", Default: "true", Group: "Access", Label: "Allow self-registration", Public: true, IsFlag: true},
	{Key: "email_verification.enabled", Type: "bool", Default: "false", Group: "Access", Label: "Require email verification", Public: true, IsFlag: true},
	{Key: "password_reset.enabled", Type: "bool", Default: "true", Group: "Access", Label: "Allow password reset by email", Public: true, IsFlag: true},
	{Key: "demo_mode", Type: "bool", Default: "true", Group: "Access", Label: "Show demo account quick sign-in", Public: true, IsFlag: true},
	{Key: "webhooks.enabled", Type: "bool", Default: "true", Group: "Integrations", Label: "Deliver outgoing webhooks", IsFlag: true},
	{Key: "maintenance.enabled", Type: "bool", Default: "false", Group: "Operations", Label: "Maintenance mode (API returns 503)", Public: true, IsFlag: true},
	{Key: "audit.retention_days", Type: "int", Default: "365", Group: "Compliance", Label: "Audit retention (days)"},
	{Key: "sessions.max_per_user", Type: "int", Default: "0", Group: "Access", Label: "Max sessions per user (0 = unlimited)"},
}

// SettingsRepo persists the settings key-value store.
type SettingsRepo interface {
	UpsertSetting(ctx context.Context, setting model.Setting) error
	GetSetting(ctx context.Context, key string) (model.Setting, error)
	ListSettings(ctx context.Context) ([]model.Setting, error)
}

// SettingsService serves the settings catalog with a small read-through
// cache: typed accessors on the hot path, invalidation on write.
type SettingsService struct {
	store SettingsRepo
	cache map[string]model.Setting
	mu    sync.RWMutex
	seen  time.Time
	ttl   time.Duration
}

// NewSettingsService builds the service with a 30s cache TTL.
func NewSettingsService(store SettingsRepo) *SettingsService {
	return &SettingsService{store: store, ttl: 30 * time.Second}
}

// Catalog returns the known settings with their stored values merged in.
func (s *SettingsService) Catalog(ctx context.Context) ([]SettingView, error) {
	stored, err := s.all(ctx)
	byKey := map[string]model.Setting{}
	if err == nil {
		for _, setting := range stored {
			byKey[setting.Key] = setting
		}
	}
	out := make([]SettingView, 0, len(SettingDefinitions))
	for _, def := range SettingDefinitions {
		value := def.Default
		updatedBy := ""
		var updatedAt time.Time
		if existing, ok := byKey[def.Key]; ok {
			value = existing.Value
			updatedBy = existing.UpdatedBy
			updatedAt = existing.UpdatedAt
		}
		out = append(out, SettingView{SettingDefinition: def, Value: value, UpdatedBy: updatedBy, UpdatedAt: updatedAt})
	}
	return out, nil
}

// PublicSettings returns the subset of settings safe to expose without a
// session (the UI needs to know whether registration is open before login).
func (s *SettingsService) PublicSettings(ctx context.Context) map[string]string {
	out := map[string]string{}
	for _, def := range SettingDefinitions {
		if !def.Public {
			continue
		}
		out[def.Key] = s.Get(ctx, def.Key, def.Default)
	}
	return out
}

// Get returns the stored value of a key, or the definition default.
func (s *SettingsService) Get(ctx context.Context, key, fallback string) string {
	stored, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return fallback
	}
	return stored.Value
}

// Bool reads a boolean setting (stored as a JSON boolean).
func (s *SettingsService) Bool(ctx context.Context, key string) bool {
	value := s.Get(ctx, key, s.definition(key).Default)
	var b bool
	return json.Unmarshal([]byte(value), &b) == nil && b
}

// Int reads an integer setting (stored as a JSON number).
func (s *SettingsService) Int(ctx context.Context, key string) int {
	value := s.Get(ctx, key, s.definition(key).Default)
	var n int
	_ = json.Unmarshal([]byte(value), &n)
	return n
}

// Set validates and stores one setting; only catalog keys are accepted and
// the stored value is the JSON literal of the definition's type.
func (s *SettingsService) Set(ctx context.Context, key, value, updatedBy string) error {
	def := s.definition(key)
	if def.Key == "" {
		return validationError("unknown setting " + key)
	}
	storedValue := value
	switch def.Type {
	case "bool":
		if value != "true" && value != "false" {
			return validationError("setting expects true or false")
		}
		storedValue = value // JSON boolean literal
	case "int":
		var n int
		if err := json.Unmarshal([]byte(value), &n); err != nil {
			return validationError("setting expects an integer")
		}
		storedValue = value // JSON number literal
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return validationError("setting must be a string")
		}
		storedValue = string(raw)
	}
	stored := model.Setting{Key: key, Value: storedValue, Description: def.Label, UpdatedBy: updatedBy, UpdatedAt: time.Now()}
	if err := s.store.UpsertSetting(ctx, stored); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *SettingsService) definition(key string) SettingDefinition {
	for _, def := range SettingDefinitions {
		if def.Key == key {
			return def
		}
	}
	return SettingDefinition{}
}

// all returns the stored settings, served through a cache that tolerates a
// missing store (dev mode): defaults then apply.
func (s *SettingsService) all(ctx context.Context) ([]model.Setting, error) {
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
	s.cache = map[string]model.Setting{}
	for _, setting := range settings {
		s.cache[setting.Key] = setting
	}
	s.seen = time.Now()
	return settings, nil
}

func cachedValues(cache map[string]model.Setting) []model.Setting {
	out := make([]model.Setting, 0, len(cache))
	for _, setting := range cache {
		out = append(out, setting)
	}
	return out
}

func (s *SettingsService) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}
