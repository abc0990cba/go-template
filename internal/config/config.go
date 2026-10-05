// Package config loads application configuration from the environment with
// explicit validation; nothing falls back silently on malformed values.
package config

import (
	"log/slog"
	"time"
)

// Config carries every tunable the binaries need. Zero values are never
// valid: Load fails unless all fields pass validation.
type Config struct {
	HTTPAddr string
	// MetricsAddr serves /metrics; empty disables the listener (the api
	// serves metrics on its main port, the worker opts in via compose).
	MetricsAddr  string
	BaseURL      string
	LogLevel     slog.Level
	DatabaseURL  string
	RedisAddr    string
	ReportCron   string
	LogFormat    string
	OTLPEndpoint string
	// OTLPSampleRatio is the head-sampling ratio (0..1]; 1 = sample every
	// trace. Production deployments typically run 0.05-0.25.
	OTLPSampleRatio float64
	// PyroscopeURL enables continuous profiling when set (APP_PYROSCOPE_URL);
	// empty keeps the profiler off.
	PyroscopeURL     string
	PprofEnabled     bool
	SessionTTL       time.Duration
	CookieSecure     bool
	AuthRateRPS      float64
	AuthRateBurst    int
	GoogleAuthURL    string
	GoogleTokenURL   string
	GoogleUserInfo   string
	GitHubAuthURL    string
	GitHubTokenURL   string
	GitHubUserURL    string
	GitHubEmailURL   string
	FacebookAuthURL  string
	FacebookTokenURL string
	FacebookUserInfo string
	TwitterAuthURL   string
	TwitterTokenURL  string
	TwitterUserInfo  string
	ClickHouseAddr   string
	ClickHouseDB     string
	ClickHouseUser   string
	ClickHousePass   string
	DemoEmail        string
	DemoPassword     string
	DemoSeed         bool
	ShutdownTimeout  time.Duration
	ExternalTimeout  time.Duration
	MaxConcurrency   int
	OAuthStateTTL    time.Duration
	GoogleClientID   string
	GoogleSecret     string
	GitHubClientID   string
	GitHubSecret     string
	FacebookClientID string
	FacebookSecret   string
	TwitterClientID  string
	TwitterSecret    string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPFrom         string

	// EmailMode selects the transport for transactional mail: "sync" sends
	// inline before the triggering request returns; "queue" records the
	// message and hands delivery to the worker via asynq (requires Redis).
	// Both modes record the delivery log.
	EmailMode string
	// EmailRetention bounds the email delivery log; the worker sweeps older
	// rows on a schedule.
	EmailRetention time.Duration
	// AuditRetention bounds the audit trail; the worker prunes through the
	// audit_prune SECURITY DEFINER function (the only sanctioned delete
	// path). 0 keeps audit entries forever.
	AuditRetention time.Duration

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

	// Bulkheads bound concurrent calls per outbound dependency; a full
	// bulkhead rejects fast (backpressure) instead of stacking goroutines
	// behind a slow dependency.
	BulkheadLimit          int
	BulkheadAcquireTimeout time.Duration

	// Outbox relay drains transactional events to the brokers.
	OutboxInterval    time.Duration
	OutboxBatch       int
	OutboxMaxAttempts int

	// Rate limiting and idempotency.
	APIRateRPS     float64
	APIRateBurst   int
	IdempotencyTTL time.Duration

	// OpenAPIValidation controls the runtime contract validator on /api:
	// "off", "requests" (reject non-conforming requests with 400), or "all"
	// (additionally validate responses, logging mismatches without altering
	// them). The embedded contract is what /openapi.yaml serves.
	OpenAPIValidation string

	// LoadShedMaxInflight caps concurrent /api requests; 0 disables. Bulk
	// surfaces (reports, analytics, exports) are shed earlier than
	// interactive traffic, keeping a reserve lane for the latter.
	LoadShedMaxInflight int

	// RequestTimeout bounds each /api request's context; 0 disables. The
	// deadline propagates to the database and queue calls, so a stuck
	// backend surfaces as a fast 504 instead of a hung connection.
	RequestTimeout time.Duration

	// RetentionProcessedEvents bounds the exactly-once inbox (dedupe table).
	// It must exceed any redelivery delay the brokers can produce; the
	// worker deletes older rows on a schedule.
	RetentionProcessedEvents time.Duration

	// Breached-password checks (HIBP k-anonymity range API): "off",
	// "warn" (log compromised passwords, allow them), or "enforce" (reject
	// them). Checks fail open when the API is unreachable.
	HIBPMode string
	// HIBPURL overrides the range API base URL (tests point it at a stub).
	HIBPURL string

	// Progressive lockout: after LockoutThreshold password failures the
	// account locks for LockoutWindow, doubling per additional failure up
	// to the 24h cap. A threshold of 0 disables the feature entirely.
	LockoutThreshold int
	LockoutWindow    time.Duration

	// UserRate limits each authenticated principal (user id or API-key
	// subject) individually on top of the per-IP API budget.
	UserRateRPS   float64
	UserRateBurst int

	// Pool tunes the pgx connection pool; zero durations mean "driver
	// default". StatementTimeout and IdleTxTimeout are applied server-side
	// as statement_timeout / idle_in_transaction_session_timeout.
	Pool PoolTuning

	// JWT token grants (opt-in): an empty secret and empty keyset disable
	// the token endpoints entirely.
	JWTSecret string
	// JWTKeySetData is the raw APP_JWT_SECRETS_FILE contents (a kid→secret
	// map with the active kid, for zero-downtime rotation); the tokens
	// feature parses it at wiring time.
	JWTKeySetData []byte
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	// File storage: S3/MinIO when APP_S3_BUCKET is set (secrets accept the
	// _FILE pattern); otherwise the in-process store. FilesMaxBytes caps
	// single uploads.
	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKeyID  string
	S3SecretKey    string
	S3UsePathStyle bool
	FilesMaxBytes  int64
}

// PoolTuning carries the pgxpool knobs; all fields optional.
type PoolTuning struct {
	MaxConns          int32
	MinConns          int32
	ConnMaxLifetime   time.Duration
	ConnMaxIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	StatementTimeout  time.Duration
	IdleTxTimeout     time.Duration
}
