// Package config loads application configuration from the environment with
// explicit validation; nothing falls back silently on malformed values.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config carries every tunable the binaries need. Zero values are never
// valid: Load fails unless all fields pass validation.
type Config struct {
	HTTPAddr string
	// MetricsAddr serves /metrics; empty disables the listener (the api
	// serves metrics on its main port, the worker opts in via compose).
	MetricsAddr     string
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
	DemoEmail       string
	DemoPassword    string
	DemoSeed        bool
	ShutdownTimeout time.Duration
	ExternalTimeout time.Duration
	MaxConcurrency  int
	OAuthStateTTL   time.Duration
	GoogleClientID  string
	GoogleSecret    string
	GitHubClientID  string
	GitHubSecret    string
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string

	// Streams — user enrichment from two external systems. Empty
	// KafkaBrokers or NatsURL disables that consumer, matching the
	// optional-infrastructure rule used for ClickHouse and Redis.
	KafkaBrokers       []string
	KafkaTopicProfile  string
	KafkaTopicEvents   string
	KafkaGroup         string
	NatsURL            string
	NatsStream         string
	NatsSubjectContact string
	NatsSubjectEvents  string
	NatsDurable        string

	// Circuit breakers wrap outbound calls (HTTP enrichment, webhooks,
	// broker publishes).
	CBFailureThreshold int
	CBOpenTimeout      time.Duration

	// Outbox relay drains transactional events to the brokers.
	OutboxInterval    time.Duration
	OutboxBatch       int
	OutboxMaxAttempts int

	// Rate limiting and idempotency.
	APIRateRPS     float64
	APIRateBurst   int
	IdempotencyTTL time.Duration
}

// Load reads configuration from the environment. Invalid values fail startup
// instead of silently falling back: a typo like APP_MAX_CONCURRENCY=eight
// must never become the default under the hood.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       env("APP_HTTP_ADDR", ":8080"),
		MetricsAddr:    os.Getenv("APP_METRICS_ADDR"),
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
		DemoEmail:      os.Getenv("APP_DEMO_EMAIL"),
		DemoPassword:   os.Getenv("APP_DEMO_PASSWORD"),
		DemoSeed:       os.Getenv("APP_DEMO_SEED") == "1",
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID: os.Getenv("GITHUB_CLIENT_ID"),
		GitHubSecret:   os.Getenv("GITHUB_CLIENT_SECRET"),
		SMTPHost:       os.Getenv("APP_SMTP_HOST"),
		SMTPUsername:   os.Getenv("APP_SMTP_USERNAME"),
		SMTPPassword:   os.Getenv("APP_SMTP_PASSWORD"),
		SMTPFrom:       env("APP_SMTP_FROM", "no-reply@go-template.local"),

		KafkaBrokers:       splitEnv("APP_KAFKA_BROKERS"),
		KafkaTopicProfile:  env("APP_KAFKA_TOPIC_PROFILE", "user-profile.v1"),
		KafkaTopicEvents:   env("APP_KAFKA_TOPIC_EVENTS", "user-events.v1"),
		KafkaGroup:         env("APP_KAFKA_GROUP", "go-template-enricher"),
		NatsURL:            os.Getenv("APP_NATS_URL"),
		NatsStream:         env("APP_NATS_STREAM", "USERS"),
		NatsSubjectContact: env("APP_NATS_SUBJECT_CONTACT", "users.contact.v1"),
		NatsSubjectEvents:  env("APP_NATS_SUBJECT_EVENTS", "users.events.v1"),
		NatsDurable:        env("APP_NATS_DURABLE", "enricher-contact"),
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
	if cfg.SMTPPort, err = intEnv("APP_SMTP_PORT", 1025); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_SESSION_TTL")
	}
	if cfg.AuthRateRPS <= 0 || cfg.AuthRateBurst <= 0 {
		return Config{}, fmt.Errorf("auth rate limit must be positive")
	}
	if cfg.CBFailureThreshold, err = intEnv("APP_CB_FAILURE_THRESHOLD", 5); err != nil {
		return Config{}, err
	}
	if cfg.CBOpenTimeout, err = durationEnv("APP_CB_OPEN_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.CBFailureThreshold <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_CB_FAILURE_THRESHOLD")
	}
	if cfg.CBOpenTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_CB_OPEN_TIMEOUT")
	}
	if cfg.OutboxInterval, err = durationEnv("APP_OUTBOX_INTERVAL", time.Second); err != nil {
		return Config{}, err
	}
	if cfg.OutboxBatch, err = intEnv("APP_OUTBOX_BATCH", 100); err != nil {
		return Config{}, err
	}
	if cfg.OutboxMaxAttempts, err = intEnv("APP_OUTBOX_MAX_ATTEMPTS", 10); err != nil {
		return Config{}, err
	}
	if cfg.OutboxInterval <= 0 || cfg.OutboxBatch <= 0 || cfg.OutboxMaxAttempts <= 0 {
		return Config{}, fmt.Errorf("outbox settings must be positive")
	}
	if cfg.APIRateRPS, err = floatEnv("APP_API_RATE_RPS", 50); err != nil {
		return Config{}, err
	}
	if cfg.APIRateBurst, err = intEnv("APP_API_RATE_BURST", 100); err != nil {
		return Config{}, err
	}
	if cfg.APIRateRPS <= 0 || cfg.APIRateBurst <= 0 {
		return Config{}, fmt.Errorf("api rate limit must be positive")
	}
	if cfg.IdempotencyTTL, err = durationEnv("APP_IDEMPOTENCY_TTL", 24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_IDEMPOTENCY_TTL")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// splitEnv reads a comma-separated list; empty input yields a nil slice,
// which callers treat as "feature disabled".
func splitEnv(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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
