# Study path: from user to contributor in nine stages

This template exists to be studied: every production technique in it —
hexagonal boundaries, contract-first APIs, exactly-once ingestion,
progressive delivery — is implemented end-to-end and documented against the
real code. This page turns the documentation set into an ordered curriculum:
each stage has a goal, a reading list, a hands-on exercise, and questions to
check yourself against before moving on.

The whole path takes roughly **40–55 hours — two to three weeks part-time**
(2–3 hours a day). Stages build on each other. Paths like `internal/...` are
repository paths; doc links are relative to this file.

| Stage | Theme | Time |
|---|---|---|
| 0 | Run it and click around | 2–3 h |
| 1 | The big picture | 4–6 h |
| 2 | One request end-to-end | 5–7 h |
| 3 | Persistence | 4–6 h |
| 4 | Identity | 6–8 h |
| 5 | Async & integration | 6–8 h |
| 6 | Reliability & operations | 5–7 h |
| 7 | Ship it | 4–6 h |
| 8 | Make it yours | 4–6 h + ongoing |

---

## Stage 0 — Run it and click around (2–3 h)

**Goal:** know the *product* before the code. Everything later traces a
behavior you have already seen from the outside — `orgDomain` middleware is
much clearer once you have used the org switcher.

**Read:** [prerequisites.md](prerequisites.md),
[quickstart.md](quickstart.md) (`make docker-up` for the full stack,
`make run` for the zero-dependency process),
[troubleshooting.md](troubleshooting.md), and skim
[../guides/application-overview.md](../guides/application-overview.md).

**Exercise:** bring up the full stack, sign in with the demo login, and
tour it: file a ticket in **My Tickets**, approve it from **Approvals** (as
a different demo user), then the admin console — users, roles (move a cell
in the permission matrix), orgs, audit, jobs, streams. Watch the terminal:
every click emits a structured log line.

**Check yourself**

1. Which two pages make up the ticket workflow, and which roles see which?
2. Where in the UI do you edit a role's permissions, and what does the
   matrix map to?
3. What happens to the app when you stop Redis
   (`docker compose stop redis`)?
4. What lives on port 8080 besides the SPA? (Hint: `/health/ready`,
   `/metrics`, `/docs`.)

---

## Stage 1 — The big picture (4–6 h)

**Goal:** hold the whole system in your head — layers, deployable pieces,
and the reasoning behind the most structural decisions.

**Read**, in this order:
[../architecture/README.md](../architecture/README.md) (the map of the docs
itself), then chapters
[01](../architecture/01-introduction-and-principles.md),
[02](../architecture/02-system-context.md) (C4 L1),
[03](../architecture/03-containers-and-runtime-views.md) (C4 L2),
[04](../architecture/04-components-and-code-map.md) (C4 L3 — the code map
is your index for everything after),
[05](../architecture/05-technology-stack.md); then ADRs
[0001 hexagonal architecture](../architecture/decisions/0001-hexagonal-architecture.md),
[0002 monolith-first](../architecture/decisions/0002-monolith-first.md), and
[0021 package-by-feature](../architecture/decisions/0021-package-by-feature.md).

**Exercise:** from memory, draw the request path of `GET /api/tickets` —
every layer from browser to store and back. Compare with
`internal/httpapp/router.go` and chapter 04's code map, note what you
missed. Redraw it at the end of Stage 2; it should feel unfair.

**Check yourself**

1. What are the four C4 levels, and which chapters cover the first three?
2. Why is the worker a separate binary but the same codebase?
3. Name a port and its two adapters (any feature).
4. Why package by feature rather than by layer?

---

## Stage 2 — One request end-to-end (5–7 h)

**Goal:** narrate a single write request from TCP to JSON and back, naming
every middleware and its job.

**The route.** Tickets mount on the authenticated API group, so the real
path is `POST /api/tickets` — no version segment (check `RegisterHTTP` in
`internal/tickets/http.go` and the group chain in
`internal/httpapp/router.go`).

**The middleware chain, in order** (assembled in `NewRouter`): global —
`Recovery`, `securityHeaders`, `requestID`, `tracing`, `metrics`,
`accessLog`, `maintenanceGate`, `bodyLimit`; the `/api` group —
`originCheck`, `apiRateLimit`, `loadShedMiddleware`, `requestTimeout`,
`openapiValidate`; the authenticated subgroup — `apiAuthRequired`,
`identityRateLimit`, `orgDomain` (resolves the Casbin domain from
`X-Org-Id`); route level — `Idempotency()`,
`RequirePermission("tickets", "create")`, then the handler.

**Open, in this order:** `internal/httpapp/router.go` (read `NewRouter`
top to bottom); the one-job middleware files `middleware.go` (`requestID`,
`bodyLimit`), `security.go` (headers, origin check, rate limiters),
`observability.go` (`tracing`, `metrics`, `accessLog`), `openapi_validate.go`,
`loadshed.go`, `timeout.go`, `maintenance.go` — all in
`internal/httpapp/`; then `internal/httpapp/rbac.go` (`orgDomain`,
`requirePermission`) and the route-level `idempotency.go` + `etag.go`;
finally the feature: `internal/tickets/http.go` (handlers), `tickets.go`
(service), `store.go` (the `Store` port the handler never sees past).

**Exercise:** run the stack, log in (the session cookie is `app_session`),
then `curl -v` the endpoint and find each middleware's fingerprint:

```sh
# unauthenticated: note problem+json body, security headers, X-Request-ID
curl -sv http://localhost:8080/api/tickets

# authenticated create, twice with the same key: the retry replays
curl -sv -X POST http://localhost:8080/api/tickets \
  -H "Content-Type: application/json" -b "app_session=…" \
  -H "Idempotency-Key: demo-1" \
  -d '{"type_key":"…","title":"Study path","priority":"low"}'
curl -sv … # same again → header Idempotency-Replayed: true
```

Spot: `X-Request-ID`, the CSP/X-Frame-Options security headers, the 400
problem+json "request does not match the API contract" from the OpenAPI
gate when you send a wrong field, and the ETag on `GET /api/tickets`.

**Check yourself**

1. What does `openapiValidate` compare the request against, and how do you
   turn the gate off?
2. Where does a request's Casbin domain come from when the client sends no
   `X-Org-Id`?
3. How does the idempotency middleware guarantee two concurrent identical
   POSTs cannot both apply?
4. Why do handlers depend on `tickets.Store` and not on a Postgres type?

---

## Stage 3 — Persistence (4–6 h)

**Goal:** own the sqlc/golang-migrate loop and understand why two store
adapters must stay behaviorally identical.

**Read:** [../guides/database.md](../guides/database.md) (the
migration/query/codegen workflow); ADRs
[0003 sqlc over ORM](../architecture/decisions/0003-sqlc-over-orm.md) and
[0004 graceful degradation](../architecture/decisions/0004-graceful-degradation.md)
(why the in-memory mode exists); the real migration pair
`migrations/postgres/000012_tickets.up.sql` + `.down.sql`;
`queries/tickets.sql` and `sqlc.yaml` (how SQL becomes Go);
`internal/platform/postgres/store.go` (transactions, sentinel-error
mapping, health ping) next to its `internal/repository/` in-memory
counterparts; and `test/integration/store_contract_test.go` — the contract
suite both adapters must pass (`make test-integration`).

**Exercise:** add a trivial column end-to-end. Write migration pair
`NNNNNN_ticket_priority_note.up/down.sql`, add the column to a query in
`queries/tickets.sql`, run `make generate` (sqlc), mirror the column in the
in-memory adapter, and extend the contract test so both adapters prove it.

**Check yourself**

1. What does `make generate` actually run, and which outputs does it
   refresh?
2. Why must every `.up.sql` have a `.down.sql`, and how do you recover a
   dirty migration?
3. Where do driver errors become `ErrNotFound`/`ErrConflict`, and why only
   there?
4. What breaks first if the in-memory and Postgres adapters diverge?

---

## Stage 4 — Identity (6–8 h)

**Goal:** understand the hybrid auth model and the authorization stack well
enough to add a protected route without guessing.

**Read:** architecture chapters
[07 Authentication & Identity](../architecture/07-flows-authentication-and-identity.md)
and
[08 Authorization & Tenancy](../architecture/08-flows-authorization-and-tenancy.md);
ADRs
[0005 sessions vs JWT](../architecture/decisions/0005-sessions-vs-jwt.md)
and
[0007 Casbin RBAC with domains](../architecture/decisions/0007-casbin-rbac-with-domains.md);
guides [auth](../guides/auth.md), [mfa](../guides/mfa.md),
[token-auth](../guides/token-auth.md), [rbac](../guides/rbac.md).
Code: `internal/auth/`, `internal/mfa/`, `internal/tokens/`,
`internal/rbac/` (catalog in `rbac.go`, role seed in `roles.go`), and the
`orgDomain`/`requirePermission` pair in `internal/httpapp/rbac.go`.

**Exercise:** enable MFA for the demo user (profile → Two-factor
authentication: enroll, scan the secret into any authenticator app,
confirm). Sign out and sign in again — observe the login challenge: login
now answers with a second-factor requirement instead of a session, and the
UI asks for the 6-digit code (or a recovery code). Then call
`GET /api/users/me` with and without the `X-Org-Id` header and watch the
effective permission set change.

**Check yourself**

1. Why sessions for the browser but JWTs for third-party clients — what
   does ADR 0005 say about revocation?
2. A request arrives with the `X-Org-Id` of an org the caller does not
   belong to. Which middleware rejects it, with which status?
3. What is the difference between `g` and `p` rules, and which does the
   roles-page matrix edit?
4. How does an approver get to decide a ticket step — via a permission or
   via a role, and where is that checked?
5. Which endpoints are involved between "login with correct password" and
   "session exists" when MFA is enrolled?

---

## Stage 5 — Async & integration (6–8 h)

**Goal:** see how the template does background work, reliable event
publishing, and exactly-once consumption — three classic distributed-systems
problems, all solved locally.

**Read:** architecture chapters
[09 Business Domains](../architecture/09-flows-business-domains.md) and
[10 Event Streams](../architecture/10-flows-event-streams.md); ADRs
[0008 asynq/Redis jobs](../architecture/decisions/0008-asynq-redis-jobs.md),
[0012 transactional outbox](../architecture/decisions/0012-transactional-outbox.md),
[0011 exactly-once consumption](../architecture/decisions/0011-exactly-once-stream-consumption.md);
guides [jobs](../guides/jobs.md) and
[enrichment-streams](../guides/enrichment-streams.md) (wire format, inbox
dedupe, sequence guard, DLQ). Code: `internal/jobs/`,
`internal/platform/queue/` (worker handlers), `internal/streams/`
(consumers, outbox relay, dead letters), and the publisher tool
`cmd/devtools/`.

**Exercise:**

```sh
make demo-up                 # core stack + Redpanda (Kafka) + NATS
make demo-enrichment         # publish both streams for demo-u-0001, watch the merge
make demo-kafka-profile SEQ=5         # publish profile seq 5…
SEQ=5 make demo-kafka-profile         # …then again with the same seq → deduped
make demo-dlq                # poison a message, watch it land in dead letters
go run ./cmd/devtools dlq-replay --id <dead-letter-id>   # fix-forward the poison
make demo-outbox             # watch the outbox relay publish user.enriched
```

Keep the streams page of the admin console open while you do it: profiles,
processed events, and dead letters are all visible rows.

**Check yourself**

1. Why is the outbox table drained by a relay instead of publishing inside
   the request transaction?
2. Which two mechanisms together turn at-least-once delivery into
   exactly-once application?
3. What does the sequence guard catch that the inbox dedupe alone cannot?
4. What happens to a job enqueue when Redis is down, and why is that the
   right contract?

---

## Stage 6 — Reliability & operations (5–7 h)

**Goal:** stop trusting and start verifying: trip the protections, watch
the signals, read the alerts.

**Read:** architecture chapter
[12 Observability & Reliability](../architecture/12-observability-and-reliability.md);
ADRs
[0016 circuit breakers](../architecture/decisions/0016-circuit-breakers.md)
and [0025 bulkheads](../architecture/decisions/0025-bulkheads.md), revisiting
[0004](../architecture/decisions/0004-graceful-degradation.md) for the
degradation ladder; then the operations docs —
[chaos lab](../operations/chaos.md) (note: lives under `docs/operations/`,
not `docs/guides/`; Toxiproxy fault injection),
[SLOs](../operations/slo.md) (recording rules, multi-window burn-rate
alerts), [alert runbooks](../operations/runbooks/alerts.md),
[reliability](../operations/reliability.md), and
[observability](../operations/observability.md).

**Exercise:** bring up the observability profile plus the chaos lab
(`make chaos-up`), then inject latency and watch in Grafana:

```sh
make chaos-pg-latency MS=2000   # p95 climbs, pg pool saturates, SLOLatencyFastBurn fires
make demo-breaker               # a real breaker opens: app_cb_state → 2, BreakerOpen fires
make chaos-restore              # everything self-heals
```

Note the lesson the chaos doc makes a point of: under pure latency the
circuit breaker does **not** trip — timeouts are bounded, not failures.
That is why the breaker demo is a separate target.

**Check yourself**

1. Which alert fires first during 2s Postgres latency, and what is its
   burn-rate window pair?
2. Why *shouldn't* the breaker trip on pure latency?
3. Name the four Redis-backed features and their degradation contracts
   during a Redis outage.
4. Where do load shedding, per-request timeouts, and bulkheads each sit in
   the middleware stack?

---

## Stage 7 — Ship it (4–6 h)

**Goal:** take the same immutable image from commit to canary to a
production-shaped deployment, and rehearse a rollback.

**Read:** [../deployment/README.md](../deployment/README.md) (one image,
five ways to run it), [../deployment/ci-cd.md](../deployment/ci-cd.md)
(two pipelines, build once / scan / deploy the same tag); ADRs
[0018 distroless supply chain](../architecture/decisions/0018-distroless-supply-chain.md)
and
[0024 progressive delivery](../architecture/decisions/0024-progressive-delivery.md);
architecture chapter
[13](../architecture/13-deployment-and-operations.md). Then skim the path
you care about: [docker](../deployment/docker.md),
[kubernetes](../deployment/kubernetes.md), [nomad](../deployment/nomad.md),
[gitops](../deployment/gitops.md).

**Exercise:** run the progressive-delivery demo on a local kind cluster,
then force the abort path:

```sh
make kind-up        # kind cluster + Argo Rollouts + Prometheus + the app
make kind-release   # push a new revision; watch the canary 20% → 50% → 100%

# while the canary runs, break it on purpose: a nonexistent image fails
# the analysis gate, the rollout aborts, stable keeps serving
kubectl -n go-template argo rollouts set image go-template-api \
  api=ghcr.io/your-org/go-template:nope
kubectl -n go-template argo rollouts get rollout go-template-api --watch
```

**Check yourself**

1. What makes the image "the same" across all five deployment paths?
2. Which two metrics gate the canary, and what happens on breach?
3. Where does CI publish the release artifact, and what does the VPS
   pipeline do differently?
4. What did the abort exercise prove about stable traffic during a bad
   release?

---

## Stage 8 — Make it yours (4–6 h, then ongoing)

**Goal:** convert from student to owner: rename the template, delete a demo
domain, and ship one feature of your own through the full recipe.

**Read:**
[../architecture/15-template-guide.md](../architecture/15-template-guide.md)
— the whole chapter: the 30-minute rebrand checklist (§15.1), the feature
removal map (§15.2), and the "add a feature" recipe (§15.3) walking
OpenAPI → catalog → service → store → UI; plus
[development-workflow.md](development-workflow.md),
[../testing/testing.md](../testing/testing.md), and
[../reference/openspec.md](../reference/openspec.md) with
[ADR 0019](../architecture/decisions/0019-openspec-workflow.md) (spec-first
changes). The template ships no feature roadmap: §15.6 of the template
guide lists directions, and
[../architecture/history/18-migration-roadmap.md](../architecture/history/18-migration-roadmap.md)
shows how a real restructuring was planned step by step.

**Exercise:** follow the recipe for real. Rebrand the identity strings
(§15.1), delete one demo feature cleanly (§15.2), then add one small
feature end-to-end — resource in the catalog (`internal/rbac/rbac.go`),
migration pair, queries + `make generate`, service port and two adapters,
contract-test rows, routes with `RequirePermission`, and a UI page gated by
`useCan`. Ticket types (`internal/tickets/http_types.go`) is a good scale
reference.

**Check yourself**

1. Which four identity strings does the rebrand checklist rename?
2. Which layers must a new feature touch, in what order, per the recipe?
3. What proves your in-memory adapter matches Postgres before you write
   the UI?
4. Which existing feature is closest to yours, and what will you copy
   versus change?

---

## Beyond the template

The techniques here have canonical sources; go to them for the theory
behind what you just watched happen:

- **Google SRE Book & Workbook** (free online) — the SLO/error-budget math
  behind `deploy/prometheus/rules/slo.yml` and the burn-rate alerts.
- **Designing Data-Intensive Applications** (Kleppmann) — storage engines,
  the transactional outbox, exactly-once semantics (Stages 3 and 5).
- **Release It!** (Nygard) — circuit breakers, bulkheads, load shedding,
  and the stability patterns of Stage 6.
- **Observability Engineering** (Majors, Fong-Jones, Miranda) — the four
  signals and high-cardinality thinking behind the RED metrics.
- **Fundamentals of Software Architecture** (Richards & Ford) — C4/arc42
  and ADR practice, the frame of the architecture chapters.
- **Casbin documentation** — matcher functions (`keyMatch2`), domain/tenant
  RBAC, and RBAC-vs-ABAC model design (Stage 4).
- **Continuous Delivery** (Humble & Farley) and the **Argo Rollouts docs** —
  progressive delivery theory behind ADR 0024 (Stage 7).
