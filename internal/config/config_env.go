package config

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// poolTuningEnv reads the optional APP_PG_* knobs. Zero values keep the
// driver defaults; durations must parse and positive ints must be positive.
func poolTuningEnv() (PoolTuning, error) {
	var p PoolTuning
	var err error
	if p.MaxConns, err = int32Env("APP_PG_MAX_CONNS", 0); err != nil {
		return p, err
	}
	if p.MinConns, err = int32Env("APP_PG_MIN_CONNS", 0); err != nil {
		return p, err
	}
	if p.ConnMaxLifetime, err = durationEnv("APP_PG_CONN_MAX_LIFETIME", 0); err != nil {
		return p, err
	}
	if p.ConnMaxIdleTime, err = durationEnv("APP_PG_CONN_MAX_IDLE_TIME", 0); err != nil {
		return p, err
	}
	if p.HealthCheckPeriod, err = durationEnv("APP_PG_HEALTH_CHECK_PERIOD", 0); err != nil {
		return p, err
	}
	if p.StatementTimeout, err = durationEnv("APP_PG_STATEMENT_TIMEOUT", 0); err != nil {
		return p, err
	}
	if p.IdleTxTimeout, err = durationEnv("APP_PG_IDLE_TX_TIMEOUT", 0); err != nil {
		return p, err
	}
	if p.MaxConns < 0 || p.MinConns < 0 {
		return p, fmt.Errorf("pg pool connection counts must not be negative")
	}
	if p.MinConns > 0 && p.MaxConns > 0 && p.MinConns > p.MaxConns {
		return p, fmt.Errorf("APP_PG_MIN_CONNS must not exceed APP_PG_MAX_CONNS")
	}
	return p, nil
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
	// intEnv is backed by int32Env; clamp the fallback so the int->int32
	// conversion cannot overflow (all defaults are small positive values).
	if fallback > math.MaxInt32 {
		fallback = math.MaxInt32
	}
	if fallback < math.MinInt32 {
		fallback = math.MinInt32
	}
	value, err := int32Env(key, int32(fallback)) // #nosec G115 -- clamped above
	return int(value), err
}

func int64Env(key string, fallback int64) (int64, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func int32Env(key string, fallback int32) (int32, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return int32(parsed), nil
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
