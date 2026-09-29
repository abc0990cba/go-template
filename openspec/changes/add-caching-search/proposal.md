## Why

Under load the hottest reads (the user record behind every session, the expanded permission set on every admin page) hit PostgreSQL on every request, list endpoints re-send identical dashboard payloads, and finding an entity means paging through lists with ILIKE. docs/cache.md described the cache as "planned" — this change builds it.

## What Changes

- Add a `Cache` port with two adapters: in-process TTL map (development) and Redis (shared across replicas; invalidation on one propagates to all). Keys centralized (`user:<id>`, `perms:<domain>:<user>`); a generic `CachedGetOrLoad` helper adds singleflight so a hot-key miss cannot stampede the store.
- Wire the cache into `UserService.Get` (invalidated on update/delete), `AuthService.CurrentUser` (sessions stay uncached — they slide and revoke), and `RBACService.EffectivePermissions` (invalidated on every policy mutation; 30s TTL as cross-replica backstop). Bootstrap selects Redis when configured, memory otherwise.
- Add an ETag middleware (sha256 of the buffered 200 response, bounded buffer) with If-None-Match → 304; applied to users list/detail, jobs list, tickets list, analytics summary.
- Add a `SearchRepo` port with a Postgres adapter (generated tsvector columns + GIN indexes on users and tickets, `websearch_to_tsquery('simple', …)`, ts_rank ordering; migration 000016) and a memory substring adapter; `SearchService` scopes results by permission (users need users:read; tickets need tickets:read and, without read_all, only the caller's own). New `GET /api/search?q=&limit=`.

## Capabilities

### New Capabilities

- `caching-search`

## Impact

- Invalidated user cache means profile changes appear immediately on the next read; permission changes invalidate the affected user+domain or the whole perms prefix.
- Existing `q` ILIKE list filters stay; the ranked search endpoint is additive.
- The tsvector columns join the tickets/users SELECT *-shaped queries (sqlc maps them back to the table structs; the vector scans as an opaque value the adapters ignore).
