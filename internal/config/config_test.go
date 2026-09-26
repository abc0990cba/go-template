package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr: want :8080, got %q", cfg.HTTPAddr)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL: got %q", cfg.BaseURL)
	}
	if cfg.LogLevel.String() != "INFO" {
		t.Errorf("LogLevel: want INFO, got %s", cfg.LogLevel.String())
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout: got %s", cfg.ShutdownTimeout)
	}
	if cfg.ExternalTimeout != 3*time.Second {
		t.Errorf("ExternalTimeout: got %s", cfg.ExternalTimeout)
	}
	if cfg.MaxConcurrency != 8 {
		t.Errorf("MaxConcurrency: want 8, got %d", cfg.MaxConcurrency)
	}
	if cfg.OAuthStateTTL != 10*time.Minute {
		t.Errorf("OAuthStateTTL: got %s", cfg.OAuthStateTTL)
	}
}

func TestLoadAcceptsValidOverrides(t *testing.T) {
	t.Setenv("APP_HTTP_ADDR", ":9090")
	t.Setenv("APP_LOG_LEVEL", "debug")
	t.Setenv("APP_MAX_CONCURRENCY", "16")
	t.Setenv("APP_EXTERNAL_TIMEOUT", "1500ms")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr: got %q", cfg.HTTPAddr)
	}
	if cfg.LogLevel.String() != "DEBUG" {
		t.Errorf("LogLevel: want DEBUG, got %s", cfg.LogLevel.String())
	}
	if cfg.MaxConcurrency != 16 {
		t.Errorf("MaxConcurrency: want 16, got %d", cfg.MaxConcurrency)
	}
	if cfg.ExternalTimeout != 1500*time.Millisecond {
		t.Errorf("ExternalTimeout: got %s", cfg.ExternalTimeout)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  string
	}{
		{"non-numeric concurrency", "APP_MAX_CONCURRENCY", "eight"},
		{"zero concurrency", "APP_MAX_CONCURRENCY", "0"},
		{"negative concurrency", "APP_MAX_CONCURRENCY", "-2"},
		{"non-numeric shutdown timeout", "APP_SHUTDOWN_TIMEOUT", "soon"},
		{"zero shutdown timeout", "APP_SHUTDOWN_TIMEOUT", "0s"},
		{"zero external timeout", "APP_EXTERNAL_TIMEOUT", "0s"},
		{"zero oauth state ttl", "OAUTH_STATE_TTL", "0s"},
		{"unknown log level", "APP_LOG_LEVEL", "loud"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.set)
			if _, err := Load(); err == nil {
				t.Fatalf("%s=%q: want error, got nil", tc.key, tc.set)
			}
		})
	}
}
