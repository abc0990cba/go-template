// Loading: Load reads the configuration from the environment; the type it
// builds lives in config.go, the env parsing helpers in config_env.go.

package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Load reads configuration from the environment. Invalid values fail startup
// instead of silently falling back: a typo like APP_MAX_CONCURRENCY=eight
// must never become the default under the hood.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:         env("APP_HTTP_ADDR", ":8080"),
		MetricsAddr:      os.Getenv("APP_METRICS_ADDR"),
		BaseURL:          env("APP_BASE_URL", "http://localhost:8080"),
		RedisAddr:        os.Getenv("APP_REDIS_ADDR"),
		ReportCron:       os.Getenv("APP_REPORT_CRON"),
		LogFormat:        env("APP_LOG_FORMAT", "text"),
		OTLPEndpoint:     os.Getenv("APP_OTLP_ENDPOINT"),
		PyroscopeURL:     os.Getenv("APP_PYROSCOPE_URL"),
		PprofEnabled:     os.Getenv("APP_PPROF") == "1",
		GoogleAuthURL:    os.Getenv("GOOGLE_AUTH_URL"),
		GoogleTokenURL:   os.Getenv("GOOGLE_TOKEN_URL"),
		GoogleUserInfo:   os.Getenv("GOOGLE_USERINFO_URL"),
		GitHubAuthURL:    os.Getenv("GITHUB_AUTH_URL"),
		GitHubTokenURL:   os.Getenv("GITHUB_TOKEN_URL"),
		GitHubUserURL:    os.Getenv("GITHUB_USER_URL"),
		GitHubEmailURL:   os.Getenv("GITHUB_EMAIL_URL"),
		FacebookAuthURL:  os.Getenv("FACEBOOK_AUTH_URL"),
		FacebookTokenURL: os.Getenv("FACEBOOK_TOKEN_URL"),
		FacebookUserInfo: os.Getenv("FACEBOOK_USERINFO_URL"),
		TwitterAuthURL:   os.Getenv("TWITTER_AUTH_URL"),
		TwitterTokenURL:  os.Getenv("TWITTER_TOKEN_URL"),
		TwitterUserInfo:  os.Getenv("TWITTER_USERINFO_URL"),
		ClickHouseAddr:   os.Getenv("APP_CLICKHOUSE_ADDR"),
		ClickHouseDB:     env("APP_CLICKHOUSE_DB", "app"),
		ClickHouseUser:   env("APP_CLICKHOUSE_USER", "default"),
		ClickHousePass:   os.Getenv("APP_CLICKHOUSE_PASSWORD"),
		DemoEmail:        os.Getenv("APP_DEMO_EMAIL"),
		DemoPassword:     os.Getenv("APP_DEMO_PASSWORD"),
		DemoSeed:         os.Getenv("APP_DEMO_SEED") == "1",
		GoogleClientID:   os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:     os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:   os.Getenv("GITHUB_CLIENT_ID"),
		GitHubSecret:     os.Getenv("GITHUB_CLIENT_SECRET"),
		FacebookClientID: os.Getenv("FACEBOOK_CLIENT_ID"),
		FacebookSecret:   os.Getenv("FACEBOOK_CLIENT_SECRET"),
		TwitterClientID:  os.Getenv("TWITTER_CLIENT_ID"),
		TwitterSecret:    os.Getenv("TWITTER_CLIENT_SECRET"),
		SMTPHost:         os.Getenv("APP_SMTP_HOST"),
		SMTPUsername:     os.Getenv("APP_SMTP_USERNAME"),
		SMTPPassword:     os.Getenv("APP_SMTP_PASSWORD"),
		SMTPFrom:         env("APP_SMTP_FROM", "no-reply@go-template.local"),

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
	// Secrets may come from mounted files (Docker/K8s secrets) instead of
	// plain environment variables.
	if cfg.DatabaseURL, err = secretValue("APP_DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.SMTPPassword, err = secretValue("APP_SMTP_PASSWORD"); err != nil {
		return Config{}, err
	}
	if cfg.ClickHousePass, err = secretValue("APP_CLICKHOUSE_PASSWORD"); err != nil {
		return Config{}, err
	}
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
	if cfg.OTLPSampleRatio, err = floatEnv("APP_OTLP_SAMPLER", 1); err != nil {
		return Config{}, err
	}
	if cfg.OTLPSampleRatio <= 0 || cfg.OTLPSampleRatio > 1 {
		return Config{}, fmt.Errorf("APP_OTLP_SAMPLER must be in (0, 1], got %g", cfg.OTLPSampleRatio)
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
	if cfg.BulkheadLimit, err = intEnv("APP_BULKHEAD_LIMIT", 16); err != nil {
		return Config{}, err
	}
	if cfg.BulkheadAcquireTimeout, err = durationEnv("APP_BULKHEAD_ACQUIRE_TIMEOUT", time.Second); err != nil {
		return Config{}, err
	}
	if cfg.BulkheadLimit <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_BULKHEAD_LIMIT")
	}
	if cfg.BulkheadAcquireTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_BULKHEAD_ACQUIRE_TIMEOUT")
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
	if cfg.RetentionProcessedEvents, err = durationEnv("APP_RETENTION_PROCESSED_EVENTS", 30*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.RetentionProcessedEvents <= 0 {
		return Config{}, fmt.Errorf("%s must be positive", "APP_RETENTION_PROCESSED_EVENTS")
	}
	if cfg.UserRateRPS, err = floatEnv("APP_USER_RATE_RPS", 100); err != nil {
		return Config{}, err
	}
	if cfg.UserRateBurst, err = intEnv("APP_USER_RATE_BURST", 200); err != nil {
		return Config{}, err
	}
	if cfg.UserRateRPS <= 0 || cfg.UserRateBurst <= 0 {
		return Config{}, fmt.Errorf("user rate limit must be positive")
	}
	if cfg.Pool, err = poolTuningEnv(); err != nil {
		return Config{}, err
	}
	if cfg.JWTSecret, err = secretValue("APP_JWT_SECRET"); err != nil {
		return Config{}, err
	}
	if cfg.JWTSecret != "" && len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("APP_JWT_SECRET must be at least 32 bytes, got %d", len(cfg.JWTSecret))
	}
	if cfg.JWTAccessTTL, err = durationEnv("APP_JWT_ACCESS_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.JWTRefreshTTL, err = durationEnv("APP_JWT_REFRESH_TTL", 30*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.JWTAccessTTL <= 0 || cfg.JWTRefreshTTL <= 0 {
		return Config{}, fmt.Errorf("JWT TTLs must be positive")
	}
	if cfg.JWTRefreshTTL < cfg.JWTAccessTTL {
		return Config{}, fmt.Errorf("APP_JWT_REFRESH_TTL must not be shorter than APP_JWT_ACCESS_TTL")
	}
	if cfg.S3Endpoint = os.Getenv("APP_S3_ENDPOINT"); cfg.S3Endpoint != "" || os.Getenv("APP_S3_BUCKET") != "" {
		cfg.S3Region = env("APP_S3_REGION", "us-east-1")
		cfg.S3Bucket = os.Getenv("APP_S3_BUCKET")
		if cfg.S3AccessKeyID, err = secretValue("APP_S3_ACCESS_KEY_ID"); err != nil {
			return Config{}, err
		}
		if cfg.S3SecretKey, err = secretValue("APP_S3_SECRET_ACCESS_KEY"); err != nil {
			return Config{}, err
		}
		if cfg.S3UsePathStyle, err = boolEnv("APP_S3_USE_PATH_STYLE", true); err != nil {
			return Config{}, err
		}
		if cfg.S3Bucket == "" {
			return Config{}, fmt.Errorf("APP_S3_BUCKET is required when file storage is configured")
		}
	}
	if cfg.FilesMaxBytes, err = int64Env("APP_FILES_MAX_MB", 10); err != nil {
		return Config{}, err
	}
	cfg.FilesMaxBytes *= 1 << 20
	if cfg.FilesMaxBytes <= 0 {
		return Config{}, fmt.Errorf("APP_FILES_MAX_MB must be positive")
	}
	return cfg, nil
}

// secretValue resolves a sensitive variable: either KEY is set directly, or
// KEY_FILE points at a file whose trimmed contents are the value (the
// Docker/K8s secrets mount pattern). Setting both is a configuration error.
func secretValue(key string) (string, error) {
	direct := os.Getenv(key)
	file := os.Getenv(key + "_FILE")
	if direct != "" && file != "" {
		return "", fmt.Errorf("%s and %s are mutually exclusive", key, key+"_FILE")
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("%s: %w", key+"_FILE", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	return direct, nil
}
