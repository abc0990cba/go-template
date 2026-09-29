# ADR 0014 — Idempotency-Key Middleware for Client Retries

**Status:** Accepted

## Context

Clients retry POSTs on timeouts — mobile networks guarantee it. Without
server-side dedupe, "create ticket" fires twice; with naive dedupe, a
retry with a *different* body silently reuses the first result, and
concurrent duplicates race. The industry pattern is Stripe's
`Idempotency-Key` header.

## Decision

Opt-in middleware (`idempotency()`) on mutating routes (currently
`POST /api/tickets`):

- No header → pass-through (the API never requires it).
- With header: atomically `INSERT` a reservation `(user_id, key, method,
  path, sha256(body), status=0)`; on conflict — same hash + still
  running → `409` + `Retry-After: 1`; same hash + completed → replay the
  stored status/body with `Idempotency-Replayed: true`; different hash →
  `422` (key reuse is a client bug worth a loud error).
- First execution buffers the response and persists it (TTL
  `APP_IDEMPOTENCY_TTL`, 24 h); keys pruned every 10 min.

## Consequences

**Positive:** safe retries with zero client coordination; the replay is
byte-identical (the regression script proves two POSTs → two 201s → one
ticket); the concurrency hole (double-submit) is closed by the atomic
reserve.

**Negative:** response bodies must be buffered (bounded by the 1 MiB
body cap); applies per route, not globally (deliberate — read paths and
naturally idempotent PUTs don't need the overhead); keys are
user-scoped, so cross-user collision is impossible by construction but
clients must generate UUIDs per logical operation.

## Alternatives considered

- *Natural idempotency keys (client-supplied entity ids)* — works for
  some domains, but forces the client to invent ids and leaks uniqueness
  errors as the dedupe mechanism.
- *Dedupe in the service layer per endpoint* — N implementations of the
  same subtle logic instead of one reviewed middleware.
