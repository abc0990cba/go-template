# ADR 0013 — RFC 9457 problem+json as the Single Error Convention

**Status:** Accepted

## Context

Error handling fragments APIs: some handlers return `{"error": "..."}`,
some `{"message"}`, some plain strings. Clients (and the SPA) then grow a
swamp of parsers. RFC 9457 (`application/problem+json`) is the IETF
standard for exactly this.

## Decision

Every error response is `{"type":"about:blank","title","status","detail"}`
written by one helper (`fail`). Domain errors map centrally
(`httpError`): not-found → 404, conflict/invalid-transition → 409,
circuit-open → 503, forbidden → 403, invalid credentials → 401,
validation → 400, oversized body → 413; everything else → 500 with the
detail **suppressed and logged** (no internal leakage). Validation errors
carry a client-safe message whitelist; the SPA parses problem+json into
one `ApiError` type and treats any 4xx as final (no query retries).

## Consequences

**Positive:** one parser on the client, one mapper on the server; new
endpoints get correct errors for free; status codes stay meaningful
(409 vs 404 vs 422 distinctions are load-bearing for idempotency and
state machines).

**Negative:** `type` URIs are `about:blank` placeholders — fine per RFC,
but adopters wanting machine-readable error subtypes should mint real
URIs; the single mapper becomes a chokepoint that must stay disciplined.

## Alternatives considered

- *Ad-hoc error envelopes* — the status quo this replaces.
- *gRPC-style status codes in JSON* — nonstandard, same information.
- *Retry-After only on 429/503* — kept additionally where relevant
  (maintenance, rate limits, in-flight exports).
