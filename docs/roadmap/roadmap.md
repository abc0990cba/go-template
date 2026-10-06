# Roadmap

This page is the forward-looking plan for the template. It is maintained by hand and
derived **primarily from repo signals** — the OpenSpec change history, the architecture
history documents, the testing matrices, and the deployment docs. Claims that cannot be
grounded in the repository are moved to "Later / ideas" or dropped.

The relationship to the change workflow is simple: **an item here is a candidate; it
becomes real work when it graduates into an OpenSpec change proposal** under
`openspec/changes/<change-name>/` (see the [OpenSpec workflow](../reference/openspec.md)
and [ADR 0019](../architecture/decisions/0019-openspec-workflow.md)). Nothing on this
page is committed until that proposal exists; conversely, work must not start without
one. A roadmap item must also never silently contradict an accepted
[ADR](../architecture/decisions/README.md) — where one would, the item says so
("requires revisiting ADR N").

Effort: **S** ≤ 1 day, **M** ≈ 2–5 days, **L** ≥ 1 week, for one contributor who knows
the repo. Known limitations live separately in [known-issues.md](known-issues.md);
several "Now" items below fix entries there.

## Recently completed

The last major arcs, newest first — pointers over prose:

- **Package-by-feature migration** — package-by-layer to vertical slices: `internal/kernel`,
  consumer-side store interfaces, all features rolled over, the ≤ 400-line guardrail
  (`make check-size`), and the feature playbook.
  [18 — plan](../architecture/history/18-migration-roadmap.md) (complete 2026-10-01),
  [19 — record](../architecture/history/19-migration-record.md).
- **Frontend slice architecture** — `web/` reorganized to routes → features → shared,
  enforced by dependency-cruiser and size budgets in CI; the contract is `web/AGENTS.md`.
- **Test-pyramid hardening** — MSW handlers gated against the OpenAPI spec, visual
  regression (Linux-Chromium baselines), axe a11y gate, i18n parity (`test-pyramid`; the
  `WEB-*` rows in the [test matrix](../testing/test-matrix.md)).
- **Streaming enrichment, exactly-once** — Kafka + NATS consumers with
  inbox/sequences/field ownership, transactional outbox, dead-letter queue with replay,
  idempotency keys, Redis sliding-window rate limiting (`add-streaming-enrichment`; ADRs
  0011, 0012, 0014–0016).
- **Progressive delivery** — Argo Rollouts canary with metric-gated rollback, GitOps
  handoff, a real Nomad alternative, the kind rehearsal (`add-progressive-delivery`;
  ADR 0024; [orchestrators](../deployment/orchestrators.md)).
- **Monitoring as code** — Grafana dashboards, SLO burn alerts, blackbox probe,
  Alertmanager routing, continuous profiling (`add-monitoring-stack`; ADRs 0022, 0023).
- **The auth arc** — real OAuth2 provider linking → TOTP MFA with recovery codes and a
  login challenge bridge → OAuth-style token grants with rotation and reuse detection
  (`auth-security`, `add-mfa`, `add-token-auth`).
- **Performance harness** — every documented threshold became an executable k6 scenario
  or Go benchmark (`performance-harness`;
  [performance matrix](../testing/performance-matrix.md)).

## Now

Small, well-defined, high-value. Each graduates to an OpenSpec change when work starts.

### Restore the `make devtools` target

- **What:** add the missing target building `cmd/devtools` into a local `./devtools`
  binary; fix the `.gitignore` comment that already promises it.
- **Why:** `.gitignore` says "make devtools builds it locally" but no such target exists;
  `DEVTOOLS = go run ./cmd/devtools` is the only supported path (KI-014 in
  [known-issues.md](known-issues.md)).
- **Effort:** S.
- **How to start:** the `Makefile` near the `DEVTOOLS` definition;
  `docs/reference/make-targets.md` gains one row.
- **Constraints:** none — no API, schema, or contract changes.

### Webhook signing-secret rotation

- **What:** rotate an endpoint's signing secret with a short overlap window (old secret
  accepted while consumers switch), audited.
- **Why:** the secret is generated once and revealed once (`internal/webhooks/webhooks.go`);
  a leak today forces delete-and-recreate, dropping the endpoint's configuration.
- **Effort:** S/M.
- **How to start:** `internal/webhooks/` (service + `http.go`); a migration for a
  `previous_secret`/`rotated_at` pair; `api/openapi/openapi.yaml` → `make generate`.
- **Constraints:** contract-first (ADR 0009); permission-guarded and audited; the HMAC
  signing format itself unchanged.

### Rate-limit response headers on 429s

- **What:** standard `RateLimit` header fields (or `X-RateLimit-*` fallbacks) on
  rate-limited responses, beside the existing 429 + `Retry-After`.
- **Why:** ADR 0015 ships layered limiting, but clients cannot *see* their budget and
  cannot back off before hitting the wall.
- **Effort:** S.
- **How to start:** the limiter middlewares in `internal/httpapp/`; extend
  `hardening_test.go`'s 429 assertions.
- **Constraints:** keep ADR 0015's fail-open-on-outage behavior; headers must not leak
  org/user identifiers.

### Loki + promtail in the compose observability profile

- **What:** add Loki and promtail to the `observability` profile of `docker-compose.yml`
  and provision the Grafana datasource, matching what the VPS stack already ships.
- **Why:** the Ansible path deploys Loki + promtail
  (`deploy/ansible/roles/nomad_jobs/templates/loki-config.yml.j2` +
  `promtail-config.yml.j2`) and provisions the datasource, but the compose profile stops
  at Prometheus, Jaeger, Alertmanager, blackbox-exporter, Grafana, and Pyroscope — so
  log↔trace correlation (ADR 0023) cannot be exercised against logs locally.
- **Effort:** S.
- **How to start:** `docker-compose.yml` (observability profile);
  `deploy/grafana/provisioning/datasources/datasources.yml`; reuse the Ansible templates
  as the config source.
- **Constraints:** optional profile, never the default up-path (ADR 0004); configs stay
  versioned in the repo (ADR 0022).

### Close the "Planned" cells in the testing matrices

- **What:** implement or explicitly close every `Planned` cell in the
  [test matrix](../testing/test-matrix.md) and the Performance column of the
  [traceability matrix](../testing/traceability-matrix.md).
- **Why:** "matrices are updated before a feature is complete" is a project rule (root
  `README.md`), and half-Planned rows blur what is covered. AUTH-003 (real-OAuth e2e)
  should close honestly as mock-provider integration + a manual checklist, not pretend.
- **Effort:** M.
- **How to start:** grep `Planned` in `docs/testing/`; small cells first (WEB-GUARDS e2e,
  USER-002 e2e); per cell: implement, or mark `n/a` with a one-line reason.
- **Constraints:** one OpenSpec change for the batch; never delete rows — close them.

### Browser E2E beyond Chromium

- **What:** Firefox and WebKit projects in `web/playwright.config.ts` for CI; Chromium
  stays the default local run.
- **Why:** the suite is deliberately Chromium-only with one worker (KI-003 in
  [known-issues.md](known-issues.md)); WebKit/Firefox-only regressions pass CI today. The
  visual specs already self-skip off Linux (`web/e2e/visual.spec.ts`), so extra projects
  do not touch the baseline.
- **Effort:** S/M — the config change is S; the flake-triage budget is the M part.
- **How to start:** `web/playwright.config.ts` `projects`; the CI workflow; keep
  `workers: 1` initially.
- **Constraints:** visual regression stays Linux-Chromium only; run the a11y gate on at
  most one engine to keep CI minutes sane.

## Next

Bigger arcs; expect a design discussion in the proposal before tasks.

### Passkeys / WebAuthn as a second factor

- **What:** platform authenticators and security keys as an MFA factor beside TOTP —
  enrollment, ceremony endpoints, and a challenge bridge so login and the token grants
  treat it like the TOTP factor they support.
- **Why:** the only second factor today is TOTP (`add-mfa`; `internal/service/mfa.go`);
  no WebAuthn code exists in the repo (verified). Passkeys are the phishing-resistant
  factor users actually enable, and the token-auth MFA challenge bridge (TOK-004) means
  the step-up integration is already in place.
- **Effort:** L — tables + repo adapter, ceremony endpoints, a maintained library such as
  `go-webauthn/webauthn`, SPA credential management, store contract coverage.
- **How to start:** an OpenSpec proposal extending the `mfa` capability, modeled on
  `add-mfa`'s shape; frontend work in the profile slice.
- **Constraints:** ADR 0005 (opaque sessions for browsers) untouched — a passkey is a
  login factor, not a session token; the `mfa_factors` model needs a credential-type
  column or sibling table — decide in the proposal and record an ADR if contested.

### Multi-node Nomad and the HA story

- **What:** document and rehearse the VPS stack on 3 Nomad servers / 2+ clients:
  `api_replicas` > 1 across nodes, Postgres streaming replication with a failover
  runbook, backups with stated RTO/RPO.
- **Why:** the single-node VPS is the template's biggest architectural limitation (KI-001
  in [known-issues.md](known-issues.md)): `auto_revert` guards rollouts, but the node is
  a SPOF and Postgres is not replicated. `docs/deployment/nomad.md` already names it:
  "`api_replicas` … multi-node scaling is a Phase-2 change".
- **Effort:** M/L.
- **How to start:** `deploy/nomad/app.nomad.hcl` (the production job); the
  `deploy/ansible/` inventory shape for a 3-node group; the
  `docs/deployment/vps/runbook.md` failover chapter.
- **Constraints:** single-node stays the supported default — do not tax the one-command
  deploy story; Postgres replication is documented guidance, not an Ansible role, until
  someone operates it for real; the Nomad gate stays health-based (ADR 0024) — say so
  again in the doc.

### Structured audit export

- **What:** admin-triggered export of the audit log (filter by org, actor, resource,
  time range) as CSV or JSONL, delivered via the existing file-export pipeline and pruned
  like other export artifacts.
- **Why:** the audit log is readable in UI/API (`internal/audit/`) and the GDPR bundle
  (`internal/export/`) includes the *subject's* trail, but there is no bulk,
  machine-readable export for security teams or SIEM ingestion.
- **Effort:** M.
- **How to start:** `internal/audit/` (query surface), `internal/export/` (reuse the ZIP
  + presigned-URL mechanics), OpenAPI spec → generate, matrix rows.
- **Constraints:** contract-first (ADR 0009); permission-guarded and itself audited; the
  retention sweep must apply (the REL-003 pattern in the
  [requirements matrix](../testing/requirements-matrix.md)).

### Per-organization settings

- **What:** a second scope for the settings catalog — org-level values for a subset of
  keys (e.g. `mfa.required`, webhook enablement), resolved org-first with the platform
  value as fallback.
- **Why:** settings are a global, code-defined key-value catalog today
  (`internal/settings/settings.go` `Definitions`) while tenancy is Casbin domains
  (ADR 0007); multi-tenant adopters cannot vary policy per org.
- **Effort:** M/L — resolution touches auth, webhooks, tickets, and interacts with the
  perms cache from `add-caching-search`.
- **How to start:** extend `internal/settings/` with a scope field and a resolution
  service; SPA admin pages per org.
- **Constraints:** unknown keys still cannot be created at runtime — the catalog stays
  code-defined; the platform settings UI/API shapes stay backward-compatible (an additive
  scope, not a redesign).

### Presence over the WebSocket gateway

- **What:** presence (who of my org is online, typing indicators) built on the existing
  realtime hub, with heartbeats and a Redis-backed presence registry so multi-process
  deployments agree.
- **Why:** the realtime stack is one-way fan-out today: the Hub routes backend events to
  SSE (`internal/realtime/`) and the WebSocket gateway (`internal/httpapp/ws.go`) relays
  the same hub; nothing flows client → server. The pieces presence needs — the Redis
  bridge for cross-process fan-out and scoped routing (tested in
  `internal/realtime/scoping_test.go`) — already exist.
- **Effort:** M.
- **How to start:** a client→server message envelope on the WS gateway; a presence
  registry beside the Redis bridge in `internal/realtime/`.
- **Constraints:** the socket carries no credentials and refuses cross-origin opens
  (`ws.go` header) — presence messages inherit exactly that trust model; the scoping
  invariants must extend, not bypass, the existing tests.

## Later / ideas

Speculative: grounded in the repo, but low priority and/or low confidence. Re-validate
the need before starting any of these.

### ClickHouse retention configurability

- **What:** a knob (and optionally downsampling) for the analytics events TTL instead of
  the fixed 90-day `MODIFY TTL` in `migrations/clickhouse/002_events_ttl.sql`.
- **Why (low confidence):** ADR 0017 leaves retention as adopter policy and the template
  hardcodes one sane default — it has been enough so far.
- **Effort:** M. **How to start:** a startup `ALTER` in the ClickHouse bootstrap path or
  a documented adopter migration; `internal/analytics/`.
- **Constraints:** ClickHouse stays optional (ADR 0004 / ADR 0017); the Postgres-side
  retention sweeps (REL-003) are separate and implemented.

### Additional OAuth providers

- **What:** Microsoft Entra ID, Apple, etc., following the existing provider pattern
  (Google/GitHub/Facebook/Twitter, Twitter via PKCE — `internal/auth/auth.go`).
- **Why (not "Now"):** the linking flow, the `/oauth/providers` discovery endpoint, and
  per-provider endpoint overrides make each addition mechanical — which is also why it
  is low value for a template: the pattern is proven and adopters add their own.
- **Effort:** S each. **How to start:** the `internal/auth/` provider registry;
  `docs/architecture/07-flows-authentication-and-identity.md`.
- **Constraints:** contract-first per provider; credentials stay adopter-supplied (unset
  provider = hidden, verified behavior).

### GraphQL gateway

- **What:** none, unless adopter need appears — and if it ever does, a sidecar gateway in
  front of the REST api, never a replacement.
- **Why (why it is not "Now"):** **requires revisiting ADR 0009.** OpenAPI contract-first
  with types-only codegen is the single-source-of-truth decision for every wire shape; a
  second, differently-shaped contract layer doubles the drift surface the whole workflow
  exists to prevent. Listed only so the answer is on the record.
- **Effort:** L, and a permanent tax. **Constraints:** any proposal must come with a new
  ADR superseding or bounding ADR 0009; absent that, this is a non-goal.

### Windows-native development support

- **What:** native Windows paths for `scripts/dev.sh`, the `vps-*.sh` family, and
  Ansible — or a supported dev-container workflow as the cheap first step.
- **Why (not "Now"):** development requires WSL2
  (`docs/getting-started/prerequisites.md`: "Windows: use WSL2" — every script targets
  POSIX shells); that is deliberate scoping (KI-012). Only worth revisiting with
  sustained contributor demand.
- **Effort:** L. **How to start:** a dev-container path in `docs/getting-started/`.
- **Constraints:** do not fork the scripts into per-OS variants — one supported path per
  concern.

## Non-goals

Things this template deliberately will **not** do. Each is settled by an ADR or an
enforced repo contract; reopening one starts with a new ADR, not a PR.

- **No microservices split.** One codebase, two binaries (`cmd/api`, `cmd/worker`) —
  ADR 0002. Both binaries scale horizontally, and the feature-boundary work that would
  justify a split is done — which is exactly why it is not needed.
- **No ORM.** sqlc stays the data-access layer — ADR 0003. Typed SQL codegen over
  `queries/*.sql` is load-bearing, and the store-contract suite (memory ≡ Postgres)
  depends on adapters staying thin.
- **No JWT sessions for browsers.** Opaque server-side sessions; JWT grants exist for
  API/mobile only — ADR 0005. Token auth was built to *avoid* refresh tokens in browsers.
- **No state-management library on the frontend.** Server state is TanStack Query;
  mirroring it into local state, or adding Redux/Zustand, is on the forbidden list in
  `web/AGENTS.md` and enforced by CI boundaries.
- **No hand-written wire shapes, no raw `fetch`.** `api/openapi/openapi.yaml` is the
  source of truth for Go and TypeScript alike — ADR 0009 plus the web contract; the CI
  codegen drift gate fails a stale spec.
- **No Docker Swarm artifacts.** Swarm is compared and deliberately
  documented-but-not-shipped in [orchestrators.md](../deployment/orchestrators.md) — no
  metric gates, no momentum. Kubernetes (primary) and Nomad (alternative) cover the
  space this template teaches.
- **No heavy i18n framework by default.** Two small dictionaries plus the documented
  "swap in i18next if keys start proliferating" escape hatch (KI-013) is the current
  decision; revisit via this roadmap, not a drive-by dependency.
