## 1. Cache Layer
- [x] 1.1 `Cache` port + key constants + `CachedGetOrLoad` (singleflight) in `internal/service/cache.go`
- [x] 1.2 Memory + Redis adapters (`internal/platform/cache/`)
- [x] 1.3 Wiring: UserService (invalidate on write), AuthService.CurrentUser, RBACService.EffectivePermissions (+ mutation invalidations), bootstrap/cmd/api selection

## 2. ETag
- [x] 2.1 Buffered ETag middleware with bounded buffer + 304 path
- [x] 2.2 Applied to users list/detail, jobs, tickets list, analytics summary

## 3. Search
- [x] 3.1 Migration 000016 (tsvector generated columns + GIN) + down; sqlc
- [x] 3.2 `SearchRepo` port, memory + postgres adapters, `SearchService` with permission scoping
- [x] 3.3 `GET /api/search` + OpenAPI

## 4. Verification And Docs
- [x] 4.1 Tests: cache hit/singleflight/invalidation (service + router), ETag 304 + fresh-tag, search scoping (service + router)
- [x] 4.2 docs/caching-search.md (replaces the cache.md stub), traceability
