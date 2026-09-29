# 05 — Technology Stack

**In one sentence:** Go 1.26 + PostgreSQL + Redis for the core, React 19 +
Vite for the UI, ClickHouse/Kafka/NATS/S3 as specialist infrastructure —
every choice made for *production realism with zero magic*, and every
dependency swappable behind a port.

## 5.1 Backend

| Technology | Version | Role | Why this one (and what it replaces) |
|---|---|---|---|
| **Go** | 1.26 | Language for both binaries | Static binaries, `CGO_ENABLED=0` distroless images, first-class concurrency, one language for app + tooling |
| **Gin** | v1.12 | HTTP router/middleware | De-facto standard, tiny, middleware model fits the chain; net/http stdlib would work but costs boilerplate |
| **pgx/v5** | v5.11 | PostgreSQL driver | Native PG protocol, `COPY` support, pool tuning hooks, otelpgx tracing; replaces database/sql overhead |
| **sqlc** | latest | SQL → Go codegen | Write real SQL, get typed Go; no ORM impedance mismatch, no runtime reflection. ADR [0003](decisions/0003-sqlc-over-orm.md) |
| **golang-migrate** | v4.20 | Migrations | Plain `.up/.down.sql` files, embedded via `io/fs`, applied at boot |
| **asynq** | v0.26 | Redis-backed job queue | Persistent queue + retries + scheduler + task-ID dedupe on Redis you already run; replaces bespoke queue code or a broker dependency |
| **Casbin v2** | v2.135 | RBAC engine | Declarative policy model (`sub, dom, obj, act`), domains = tenants, persisted policies, `SyncedEnforcer`; replaces hand-rolled permission tables |
| **golang-jwt/v5** | v5 | JWT sign/verify (HS256) | Only for the API/mobile token grants — not for browser sessions |
| **golang.org/x/crypto/argon2** | — | Password hashing (Argon2id) | Memory-hard KDF, current OWASP recommendation |
| **pquerna/otp** | — | TOTP enrollment/verification | RFC 6238 with skew control; powers the MFA flow |
| **franz-go** | v1.22 | Kafka client | Pure Go, explicit offset control (needed for exactly-once commit-after-apply), admin client for topic creation |
| **nats.go** | v1.54 | NATS + JetStream | Pure Go; durable pull consumers with `NakWithDelay` give per-message retry semantics |
| **clickhouse-go/v2** | v2.48 | ClickHouse client | Native protocol for the analytics warehouse |
| **aws-sdk-go-v2** | — | S3 client for blobs | Works with AWS and MinIO (`UsePathStyle`); presigned URLs |
| **Prometheus client_golang** | v1.24 | Metrics | The de-facto standard; RED metrics + custom domain metrics |
| **OpenTelemetry** | v1.46 | Tracing | OTLP/HTTP export, W3C propagation across HTTP → queue headers → broker headers → DB spans (otelpgx) |
| **log/slog** | stdlib | Structured logging | Stdlib, JSON or text, zero dependency |
| **oapi-codegen** | v2.8 | OpenAPI → Go types | Types-only generation keeps handlers hand-written while the wire contract stays authoritative |
| **testcontainers-go** | v0.44 | Integration infra | Real Postgres/Redis/Kafka/NATS/ClickHouse/MinIO in Docker for contract tests |

## 5.2 Frontend

| Technology | Version | Role | Why |
|---|---|---|---|
| **React** | 19.3 | UI framework | Current stable; StrictMode, function components only |
| **Vite** | 8.3 | Build/dev server | Instant HMR; proxy rewrites `Origin` so the CSRF check passes in dev |
| **TanStack Router** | 1.170 | File-based routing | Type-safe routes, `beforeLoad` guards, `validateSearch` (URL = state), auto code-splitting per route |
| **TanStack Query** | 5.104 | Server state | Cache + invalidation discipline; `retry: false` for 4xx problem responses |
| **TanStack Table** | 8.21 | Data tables | Headless; `manualPagination: true` keeps the server authoritative |
| **Tailwind CSS v4** | 4.3 | Styling | CSS-first config (`@theme` tokens), six palettes (light/dark/VS Code×2/Catppuccin×2) |
| **shadcn/ui + Radix** | new-york style, 30 components | Component library | Vendored source you can edit; Radix accessibility primitives |
| **react-hook-form + zod** | 7.89 / 4.6 | Forms + validation | `zodResolver`; zod also validates route search params |
| **recharts** | 3.8 | Charts | Wrapped in shadcn `ChartContainer` for theming |
| **qrcode** | 1.5 | MFA enrollment QR | Renders `otpauth://` URL client-side |
| **cmdk + sonner** | 1.1 / 2.0 | ⌘K palette, toasts | Small, standard shadcn companions |
| **Vitest + Testing Library** | 5 / 16 | Unit tests (jsdom) | Shared Vite config |
| **oxlint / oxfmt** | 1.85 / 0.70 | Lint + format | Oxidized toolchain — fast; replaces ESLint/Prettier |
| **i18n** | — | EN/RU | Deliberate ~175-line homegrown layer (`lib/i18n.tsx`); swap for i18next if keys proliferate |

## 5.3 Infrastructure

| Technology | Dev image | Role |
|---|---|---|
| **PostgreSQL** | `postgres:16` | System of record: users, orgs, RBAC, tickets, audit, streams bookkeeping, tokens — 30+ tables |
| **Redis** | `redis:7-alpine` | asynq queue, shared read-through cache, distributed rate limiting (sliding window), realtime pub/sub bridge |
| **ClickHouse** | `clickhouse-server:24.8-alpine` | `analytics_events` MergeTree — product analytics (dashboards, DAU, breakdowns) |
| **Redpanda** | `v24.2.7` (Kafka API) | Enrichment input `user-profile.v1`; outbox output `user-events.v1`; dual internal/external listeners for host tools |
| **NATS** | `2.10-alpine` (JetStream) | Enrichment input `users.contact.v1`; output `users.events.v1`; stream `USERS`, durable `enricher-contact` |
| **MinIO** | `RELEASE.2024-01-16` | S3-compatible blobs: ticket attachments, GDPR export ZIPs |
| **Mailpit** | `v1.24` | Dev SMTP sink with Web UI on `:8025` |
| **Jaeger** | `all-in-one:1.63` | Trace backend (OTLP/HTTP `:4318`) |
| **Prometheus / Grafana** | `v2.54.1` / `11.2.0` | Metrics scrape (5 s) + dashboards; datasources provisioned |
| **Distroless** | `static-debian12:nonroot` | Runtime image: no shell, no package manager, non-root, `HEALTHCHECK` via `/app/api -healthcheck` |
| **Docker Compose** | — | Local orchestration with profiles: `mail`, `observability`, `files`, brokers overlay |
| **Helm** | chart `go-template` 0.1.0 | Production deployment: 2–10 api replicas (HPA CPU 70%), PDB minAvailable 1, probes on `/health/*`, secret mounts, hardened securityContext |
| **GitHub Actions** | — | CI (web lint/build/test, Go vet/build/race-test, golangci-lint, integration containers, image build) and Release (multi-arch GHCR push, Trivy fail-on-critical, Syft SBOM, cosign keyless sign, Helm chart push to OCI) |

## 5.4 What to swap when you adopt the template

The stack was assembled so each piece is replaceable without touching
business logic:

| If you... | Swap | Touches |
|---|---|---|
| Prefer MySQL | Postgres adapter | `internal/platform/postgres` + migrations (ports unchanged) |
| Use RabbitMQ/SQS instead of Redis queue | `service.JobQueue` port | `internal/platform/queue` consumers/producers |
| Already run Kafka only | Drop NATS side | `bootstrap.Brokers`, streams config — each broker side degrades independently |
| Use Firebase Auth/Auth0 | `AuthService` internals | Sessions become provider callbacks; RBAC stays |
| Deploy on VMs, not k8s | Helm chart | systemd units + the same image work unchanged |
| Want Next.js/SSR | the SPA | API is pure JSON; embed step is optional |

The rule of thumb: **anything behind `internal/service/ports.go` is a
decision, not a commitment.**

Next: [06 — Data Model](06-data-model.md).
