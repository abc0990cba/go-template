# ADR 0015 — Layered Rate Limiting: Token Bucket (Memory) / Sliding Window (Redis)

**Status:** Accepted

## Context

Public endpoints need protection from credential stuffing (auth), scrapers
(per-IP), and noisy tenants (per-identity). The limiter must be correct
across replicas in production but need nothing extra in dev, and its
failure mode must be explicit.

## Decision

One `ratelimit.Limiter` port, three scopes, two adapters:

- **Scopes:** `auth` (per-IP, 5 rps / burst 10 — credential endpoints),
  `api` (per-IP, 50 rps / burst 100), `user` (per-principal — user id or
  API-key owner, 100 rps / burst 200). Rejection → `429` + `Retry-After`
  + a labeled metric.
- **Memory adapter:** per-key token buckets (`x/time/rate`) with a janitor
  — dev and single-process mode.
- **Redis adapter:** sliding-window log in one Lua script
  (`ZREMRANGEBYSCORE` → `ZCARD` → conditional `ZADD`/`PEXPIRE`),
  returning the oldest timestamp for an accurate `Retry-After`; one
  window per second; cluster-wide correctness.
- **Failure policy:** limiter errors **fail open** with a dedicated
  error metric — availability outranks strictness; Redis is also the
  queue, so hard-failing the API when it blips would compound outages.

## Consequences

**Positive:** the three scopes cover the realistic abuse patterns without
config surfaces; `Retry-After` is honest (sliding window math) so
well-behaved clients back off correctly; swapping algorithms is one file.

**Negative:** the sliding-window log costs O(window × rate) memory in
Redis per key (bounded by 1-second windows and rps caps); per-IP limits
behave poorly behind shared NATs (adopters behind corporate proxies
should raise `APP_API_RATE_RPS`).

## Alternatives considered

- *GCRA/token bucket in Redis only* — cheaper memory, but the Lua
  sliding window gives exact `Retry-After` and is easier to audit.
- *Envoy/ingress-level limiting* — the right second layer in production;
  the app keeps its own last line of defense regardless.
- *Fail closed* — rejected: the limiter must not become the single point
  of failure.
