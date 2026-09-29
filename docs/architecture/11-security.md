# 11 — Security

**In one sentence:** the security model is defense-in-depth with boring,
auditable primitives — Argon2id passwords, hashed single-use tokens,
cookie sessions + CSRF origin checks, Casbin RBAC with tenant domains,
strict security headers, body caps, layered rate limits, `_FILE` secrets,
and a distroless non-root runtime with a signed, scanned supply chain.

## 11.1 Authentication surface summary

Detailed flows live in [07](07-flows-authentication-and-identity.md); the
security-relevant facts:

| Secret | At rest | In transit | Single-use |
|---|---|---|---|
| Password | Argon2id (t=1, m=64 MiB, p=4) | TLS (HTTPS) | n/a |
| Session token | opaque random 16 B — no session data client-side | `HttpOnly` cookie | rotation on login/challenge/stop |
| MFA secret (TOTP) | stored in `mfa_factors` (per-column read restricted to the service) | never leaves the server after enrollment | n/a |
| MFA challenge / recovery codes | sha256 | n/a | yes (TTL 5 min / consume-once) |
| Email tokens (reset/verify) | sha256 | link over TLS | yes (TTL 1 h / 24 h) |
| Refresh tokens | sha256 + family id | request body over TLS | rotation + reuse kill |
| API keys | sha256 + prefix | Bearer header | n/a (revocable) |
| OAuth state | random, TTL 10 m | URL | delete-first consume |
| Webhook secrets | plain in `webhook_endpoints` (needed to sign) | shown once | n/a |

## 11.2 Transport & browser hardening

| Control | Implementation |
|---|---|
| Security headers | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, CSP `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:` |
| HSTS | `max-age=31536000; includeSubDomains` when `APP_COOKIE_SECURE=1` (off by default so `http://localhost` works) |
| CSRF | `SameSite=Lax` cookie **plus** middleware rejecting non-GET API calls with cross-site `Sec-Fetch-Site` or mismatched `Origin` — no token dance needed for a same-origin SPA |
| Body caps | global 1 MiB (`413`), uploads capped at `APP_FILES_MAX_MB` (10 MiB default) via `MaxBytesReader` (covers chunked overflows) |
| Request limits | three rate-limit scopes: per-IP `api` (50 rps), per-IP `auth` (5 rps), per-identity `user` (100 rps) — 429 + `Retry-After`, Redis sliding-window shared across replicas, **fail-open** with an error metric |
| Timeouts | server read header 5 s / read 10 s / write 30 s / idle 60 s; outbound client 3 s; shutdown grace 10 s |
| Maintenance mode | sheds all API load with 503 while keeping login/logout/settings alive |

## 11.3 Authorization model

- **Check the verb, not the URL**: every guarded route runs
  `requirePermission(obj, act)` against Casbin; services re-check
  object-level rules (`canView`, `canDecide`, `read_all` scoping, org
  membership) — URL obscurity is never a control.
- **Tenant isolation**: the active domain comes from membership checks,
  not raw trust in `X-Org-Id`; tenant tables filter by `org_id`; admin
  APIs re-verify per request (the SPA's gates are UX only).
- **Fail-safe defaults**: nil enforcer denies everything; unknown settings
  keys rejected; first-registration superadmin is the single bootstrap
  exception, thereafter everyone is `viewer`.
- **Impersonation** never widens permissions — it *is* the target
  identity, banner-visible, fully audited.

## 11.4 Input validation & injection resistance

| Threat | Defense |
|---|---|
| SQL injection | zero string-built SQL — all 190 queries are static sqlc statements with parameters |
| XSS | React escapes by default + CSP `script-src 'self'`; the API never returns HTML (problem+json / JSON only) |
| Path traversal in uploads | filenames sanitized (components and control chars stripped, 200-char cap) and stored under a random id directory |
| Untrusted webhooks *inbound* | none — inbound HTTP from consumers is not accepted; deliveries are outbound-only |
| DoS by parsing | body caps + bounded JSON decode + rate limits + breaker-wrapped outbound |
| Replay | `Idempotency-Key` request-hash binding (`422` on mismatch); inbox dedupe on streams; single-use tokens everywhere |

## 11.5 Secrets management

- Every sensitive variable accepts a **`_FILE` twin**
  (`APP_DATABASE_URL_FILE`, `APP_JWT_SECRET_FILE`, `APP_SMTP_PASSWORD_FILE`,
  `APP_CLICKHOUSE_PASSWORD_FILE`, `APP_S3_ACCESS_KEY_ID_FILE`,
  `APP_S3_SECRET_ACCESS_KEY_FILE`): the value is read from a mounted file
  (Kubernetes secret volume pattern). Setting both forms is a startup
  error.
- The Helm chart renders a `Secret` from values or mounts an
  `existingSecret`; containers are `readOnlyRootFilesystem`,
  `runAsNonRoot` (uid 65532), all capabilities dropped.
- `APP_JWT_SECRET` unset ⇒ token endpoints answer **501** — the template
  refuses to run JWT with a default secret.

## 11.6 Threat notes (STRIDE-flavored, honest)

| Threat | Status |
|---|---|
| Spoofed login | Argon2id + rate limits + optional MFA (`mfa.required` flag) |
| Token theft (refresh) | rotation with family revocation on reuse |
| CSRF | SameSite + Origin/`Sec-Fetch-Site` checks (defense in depth) |
| Privilege escalation via API keys | keys inherit owner's RBAC; scopes stored/narrowed — full scope enforcement at handler level is an extension point |
| Privilege escalation via impersonation | banner + audits + fresh session on stop |
| Insider data scraping | audit log with IP/request-id diffs, per-user activity view |
| Supply chain | SHA-pinned Actions, distroless base, Trivy fail-on-critical, Syft SBOM, cosign keyless signing |
| ClickHouse/Redis compromise | both hold no credentials and no irreplaceable data; Postgres remains the boundary |
| Session fixation / enumeration | new tokens per login; uniform errors everywhere |
| Webhook payload forgery (on consumers) | HMAC-SHA256 over raw body with per-endpoint secret (`X-Template-Signature`) |

Known, deliberate gaps (documented so adopters decide consciously): no
virus scanning of uploads; no content-type allowlist; webhook secrets
stored recoverable (required for signing); analytics events are not
retention-limited by default; export prune is exposed but not scheduled.

## 11.7 Security testing in the repo

- `internal/httpapp/security_test.go` — CSRF/origin matrix, header presence.
- `internal/httpapp/hardening_test.go` — body caps, oversized payload
  handling, header invariants.
- `scripts/regression-walkthrough.sh` — live verification of maintenance
  mode, per-identity rate limits (260 parallel requests → 429), ETag
  semantics, MFA + token-grant flows.
- `test/e2e/rbac_test.go` — permission boundaries end-to-end (403 vs 200
  per role).
- CI: `gosec` enabled in golangci-lint; race detector on every unit test
  run.

Next: [12 — Observability & Reliability](12-observability-and-reliability.md).
