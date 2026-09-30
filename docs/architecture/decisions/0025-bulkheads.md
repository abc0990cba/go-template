# ADR 0025 — Bulkheads Around Slow Dependencies

**Status:** Accepted
**Follows up:** [0016](0016-circuit-breakers.md), which named bulkheads as
the natural next step.

## Context

Breakers answer "is the dependency healthy?" but not "even while healthy,
how much of this process may it occupy?". A dependency that turns *slow*
instead of down — a webhook receiver taking 30 s to answer, an enrichment
API stalling mid-response — produces no failures for the breaker to count,
while every caller still stacks up for the full client timeout. At RPS,
that is goroutines and sockets multiplying by `timeout × rate`; in the
worst case the process exhausts memory or starves unrelated work. ADR
0016's per-request timeouts bound each call but not their *sum*.

## Decision

A `Bulkhead` in `internal/reliability` (same dependency-free style as the
breaker): a counting semaphore admitting at most `APP_BULKHEAD_LIMIT`
(16) concurrent calls per named dependency, with a bounded slot wait
(`APP_BULKHEAD_ACQUIRE_TIMEOUT`, 1 s). A full bulkhead fails fast with
`ErrBulkheadFull` — surfaced as backpressure (the caller's own retry
policy applies), **never** counted as a breaker failure. Composition order
is bulkhead-outermost, breaker-innermost:

```
bulkheads.Do(ctx, name, func() error {          // capacity check, fast reject
    return breakers.Do(ctx, name, func() error { // health check, fail fast
        ...the actual call
    })
})
```

so a slow dependency fills slots and sheds excess in milliseconds, while
a broken one opens the breaker and rejects before slots even matter.
Applied to the two fan-out paths that can grow with traffic:
`enrichment.http` and `webhooks.http` (job-queue execution already bounds
export/report concurrency at worker level). Gauges and counters —
`app_bulkhead_in_flight{name}`, `app_bulkhead_rejected_total{name}` —
appear on the Resilience dashboard.

## Consequences

**Positive:** the slow-dependency failure mode now sheds load instead of
accumulating it; backpressure is visible and measurable; the pattern is
~120 lines and symmetrical with the breaker registry, so the teaching
surface stays small.

**Negative:** one more config pair to reason about — a bulkhead below
real concurrency turns healthy traffic away (watch the rejected counter);
bulkheads protect *this process*, not the callee (it still bears the 16
concurrent calls); rejections surface as failed webhook deliveries rather
than queued ones — asynq's retry schedule absorbs that, inline dev mode
shows it to the caller.

## Alternatives considered

- *Do nothing; rely on asynq concurrency* — covers queued jobs, not the
  inline dispatcher or api-process enrichment.
- *failsafe-go / sony/gobreaker* — same verdict as ADR 0016: the
  in-repo version is the point of a template.
- *Per-dependency worker pools (physical bulkheads)* — stronger
  isolation at the cost of queued-task machinery; the semaphore keeps
  callers synchronous, which this codebase's error handling expects.
