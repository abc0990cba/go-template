## Purpose

The caching-search capability adds bounded-staleness read-through caching for the hottest reads, conditional GET for heavy lists, and ranked permission-scoped search — the three load-shaping behaviors a highload deployment reaches for first.

## ADDED Requirements

### Requirement: Read-Through User And Permission Cache

User records and expanded permission sets shall be served through the configured cache (Redis when configured, in-process otherwise) with bounded TTLs; writes to users invalidate their cache entry, and every RBAC mutation invalidates the affected permission keys. A concurrent cache miss shall load once (singleflight), and a missing or failing cache shall degrade to direct store reads.

#### Scenario: Update defeats the TTL

- **WHEN** a cached user is updated and immediately read back
- **THEN** the read returns the new values, not the cached ones

#### Scenario: Permission change applies

- **WHEN** a role assignment or policy rule changes
- **THEN** the affected user's expanded permissions reflect the change without waiting for the TTL

### Requirement: Conditional GET On Heavy Reads

The configured list/detail endpoints shall answer 200 with a strong ETag and, for an If-None-Match match on an unchanged payload, 304 without a body. Error responses pass through untouched.

#### Scenario: Polling dashboard

- **WHEN** a client repeats an unchanged GET with the previous ETag
- **THEN** the response is 304 with no body

#### Scenario: Changed payload

- **WHEN** the underlying collection changes
- **THEN** the same conditional request yields 200 with a fresh ETag

### Requirement: Permission-Scoped Full-Text Search

`GET /api/search` shall return users and tickets matching a websearch-style query, ranked by relevance. The users section requires `users:read`; the tickets section requires `tickets:read` and — without `tickets:read_all` — contains only the caller's own tickets. Both sections always serialize; unauthorized sections are empty, not absent.

#### Scenario: Requester sees only own tickets

- **WHEN** a caller with tickets:read (no read_all) searches a term matching two tickets, one of theirs
- **THEN** only their ticket is returned

#### Scenario: Section visibility follows permissions

- **WHEN** a caller without users:read searches a user-matching term
- **THEN** the users section is empty while the response still carries both sections
