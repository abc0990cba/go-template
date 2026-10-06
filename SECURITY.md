# Security Policy

## Reporting a vulnerability

**Do not report security vulnerabilities through public GitHub issues,
PRs, or discussions.**

Report privately via GitHub's *Security → Report a vulnerability*
(private vulnerability reporting) on this repository. If that is
unavailable, contact the maintainers directly and mark the message
confidential.

Please include: a description of the issue, steps to reproduce or a
proof of concept, affected components/versions, and your assessment of
severity. You will get an acknowledgment within a few days, and we will
keep you informed of the fix and the coordinated disclosure timeline.

We ask you to avoid automated scanning or testing against deployed
instances you do not own.

## Supported versions

Only the latest release line receives security fixes. When running the
template in production, track `main` or the latest `v*` tag and rebuild
regularly — CI rebuilds pin dependencies via Renovate.

## Security-relevant behavior in this codebase

If you are auditing or deploying this template, these are the places that
matter (each links to deeper documentation):

- **Transport & headers** — HSTS, security headers, body-size caps
  (chunked-safe), request timeouts: `internal/httpapp/`, described in
  [docs/operations/reliability.md](docs/operations/reliability.md).
- **Authentication** — argon2id password hashing, DB-backed session
  cookies (`Secure`/`HttpOnly`/`SameSite`), OAuth (state + PKCE),
  TOTP MFA with hashed single-use recovery codes, progressive lockout,
  optional Have-I-Been-Pwned checks: [docs/guides/auth.md](docs/guides/auth.md),
  [docs/guides/mfa.md](docs/guides/mfa.md), and
  [docs/architecture/07-flows-authentication-and-identity.md](docs/architecture/07-flows-authentication-and-identity.md).
- **Authorization** — Casbin RBAC with per-organization domains,
  permission-catalog-gated routes, `X-Org-Id` tenant resolution:
  [docs/guides/rbac.md](docs/guides/rbac.md).
- **Secrets** — every secret can be supplied via a `*_FILE` environment
  variable (Docker/K8s/Nomad secret mounting) instead of a plain env var:
  [docs/reference/configuration.md](docs/reference/configuration.md).
- **Rate limiting & abuse** — layered token buckets (per-IP, tighter auth
  budget, per-principal), idempotency-key replay protection:
  [docs/architecture/11-security.md](docs/architecture/11-security.md).
- **Integrity** — HMAC-signed webhooks, immutable (DB-enforced) audit
  trail, signed/scanned/SBOM'd release images (Trivy, Syft, cosign, SLSA):
  [docs/guides/webhooks.md](docs/guides/webhooks.md),
  [docs/deployment/ci-cd.md](docs/deployment/ci-cd.md).
- **Continuous testing** — weekly OWASP ZAP baseline scan in CI
  (`dast.yml`), gosec in the linter, govulncheck on every PR.

## Known limitations

Known security-relevant limitations and trade-offs (e.g. the placeholder
alert-receiver configuration, single-node VPS trust boundary) are tracked
with root-cause analysis in [docs/roadmap/known-issues.md](docs/roadmap/known-issues.md).
