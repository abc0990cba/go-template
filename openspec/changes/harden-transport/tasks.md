# harden-transport Tasks

## 1. Config And Platform

- [x] 1.1 `config.go`: `PoolTuning` fields (`APP_PG_*`), user rate scope (`APP_USER_RATE_*`), `_FILE` secret resolver for `APP_DATABASE_URL`, `APP_SMTP_PASSWORD`, `APP_CLICKHOUSE_PASSWORD`; validation + config tests
- [x] 1.2 `postgres.Open`: `WithTuning(PoolTuning)` option applying pool limits + `statement_timeout` / `idle_in_transaction_session_timeout` runtime params; wire in `cmd/api` and `cmd/worker`
- [x] 1.3 `.env.example` documents all new knobs

## 2. HTTP Layer

- [x] 2.1 Server-span middleware (route-pattern name, W3C extraction, 5xx error status) in `internal/httpapp/observability.go`; chain position before metrics
- [x] 2.2 Body limit: `http.MaxBytesReader` wrap + `a.bind` helper mapping `*http.MaxBytesError` to 413; switch `c.BindJSON` call sites
- [x] 2.3 HSTS header when `APP_COOKIE_SECURE`
- [x] 2.4 Per-identity limiter scope `user` on the `authorized` group
- [x] 2.5 Maintenance gate: `maintenance.enabled` setting, `/api` 503 + `Retry-After`, login/logout/settings exempt, SPA + health + metrics served; short flag memoization

## 3. Tests

- [x] 3.1 Unit tests: chunked oversize body → 413; HSTS presence; maintenance exemptions and 503; per-identity 429; config `_FILE`
- [x] 3.2 Tracing: span created with SDK provider, no-op without

## 4. Docs And Hygiene

- [x] 4.1 `docs/observability.md` + `docs/reliability.md` updated to match implementation
- [x] 4.2 `.gitignore` covers the `devtools` binary; traceability matrix rows
