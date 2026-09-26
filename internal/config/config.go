// Package config loads application configuration from the environment with
// explicit validation; nothing falls back silently on malformed values.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config carries every tunable the binaries need. Zero values are never
// valid: Load fails unless all fields pass validation.
type Config struct {
	HTTPAddr        string
	BaseURL         string
	LogLevel        slog.Level
	DatabaseURL     string
	RedisAddr       string
	ReportCron      string
	LogFormat       string
	OTLPEndpoint    string
	PprofEnabled    bool
	SessionTTL      time.Duration
	CookieSecure    bool
	AuthRateRPS     float64
	AuthRateBurst   int
	GoogleAuthURL   string
	GoogleTokenURL  string
	GoogleUserInfo  string
	GitHubAuthURL   string
	GitHubTokenURL  string
	GitHubUserURL   string
	GitHubEmailURL  string
	ClickHouseAddr  string
	ClickHouseDB    string
	ClickHouseUser  string
	ClickHousePass  string
	ShutdownTimeout time.Duration
	ExternalTimeout time.Duration
	MaxConcurrency  int
	OAuthStateTTL   time.Duration
	GoogleClientID  string
	GoogleSecret    string
	GitHubClientID  string
	GitHubSecret    string
}

// Load reads configuration from the environment. Invalid values fail startup
// instead of silently falling back: a typo like APP_MAX_CONCURRENCY=eight
// must never become the default under the hood.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       env("APP_HTTP_ADDR", ":8080"),
		BaseURL:        env("APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL:    os.Getenv("APP_DATABASE_URL"),
		RedisAddr:      os.Getenv("APP_REDIS_ADDR"),
		ReportCron:     os.Getenv("APP_REPORT_CRON"),
		LogFormat:      env("APP_LOG_FORMAT", "text"),
		OTLPEndpoint:   os.Getenv("APP_OTLP_ENDPOINT"),
		PprofEnabled:   os.Getenv("APP_PPROF") == "1",
		GoogleAuthURL:  os.Getenv("GOOGLE_AUTH_URL"),
		GoogleTokenURL: os.Getenv("GOOGLE_TOKEN_URL"),
		GoogleUserInfo: os.Getenv("GOOGLE_USERINFO_URL"),
		GitHubAuthURL:  os.Getenv("GITHUB_AUTH_URL"),
		GitHubTokenURL: os.Getenv("GITHUB_TOKEN_URL"),
		GitHubUserURL:  os.Getenv("GITHUB_USER_URL"),
		GitHubEmailURL: os.Getenv("GITHUB_EMAIL_URL"),
		ClickHouseAddr: os.Getenv("APP_CLICKHOUSE_ADDR"),
		ClickHouseDB:   env("APP_CLICKHOUSE_DB", "app"),
		ClickHouseUser: env("APP_CLICKHOUSE_USER", "default"),
		ClickHousePass: os.Getenv("APP_CLICKHOUSE_PASSWORD"),
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID: os.Getenv("GITHUB_CLIENT_ID"),
		GitHubSecret:   os.Getenv("GITHUB_CLIENT_SECRET"),
	}

	var err error
	if cfg.LogLevel, err = logLevelEnv("APP_LOG_LEVEL", slog.LevelInfo); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationEnv("APP_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ExternalTimeout, err = durationEnv("APP_EXTERNAL_TIMEOUT", 3*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.OAuthStateTTL, err = durationEnv("OAUTH_STATE_TTL", 10*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.MaxConcurrency, err = intEnv("APP_MAX_CONCURRENCY", 8); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = durationEnv("APP_SESSION_TTL", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.CookieSecure, err = boolEnv("APP_COOKIE_SECURE", true); err != nil {
		return Config{}, err
	}
	if cfg.AuthRateBurst, err = intEnv("APP_AUTH_RATE_BURST", 10); err != nil {
		return Config{}, err
	}

	if cfg.MaxConcurrency <= 0 {
		return Config{}, fmt.Errorf("%s must be positive, got %d", "APP_MAX_CONCURRENCY", cfg.MaxConcurrency)
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_SHUTDOWN_TIMEOUT")
	}
	if cfg.ExternalTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_EXTERNAL_TIMEOUT")
	}
	if cfg.OAuthStateTTL <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "OAUTH_STATE_TTL")
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, fmt.Errorf("APP_LOG_FORMAT must be text or json, got %q", cfg.LogFormat)
	}
	if cfg.AuthRateRPS, err = floatEnv("APP_AUTH_RATE_RPS", 5); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_SESSION_TTL")
	}
	if cfg.AuthRateRPS <= 0 || cfg.AuthRateBurst <= 0 {
		return Config{}, fmt.Errorf("auth rate limit must be positive")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func logLevelEnv(key string, fallback slog.Level) (slog.Level, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return level, nil
}
