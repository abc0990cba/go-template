# Caching & Search

Three load-shaping layers on top of the store, each degrade-graceful: the
cache falls back to direct reads, ETag falls back to plain 200s, and search
falls back from Postgres tsvector ranking to in-memory substring matching.

## Read-through cache

A `Cache` port (`internal/service/cache.go`) with two adapters
(`internal/platform/cache/`): an in-process TTL map and Redis
(`go-redis`, SCAN-based prefix invalidation). Bootstrap picks Redis when
`APP_REDIS_ADDR` is set — with multiple api replicas that also means
invalidations propagate, which is the production mode.

What is cached and for how long:

| Key | TTL | Filled by | Invalidated by |
|---|---|---|---|
| `user:<id>` | 30s | `Users.Get`, `Auth.CurrentUser` (the hottest read: every authenticated request resolves its user) | user update/delete |
| `perms:<domain>:<user>` | 30s | `RBAC.EffectivePermissions` (every /api/me + admin page) | role assign/unassign, role permission change, policy add/remove, role delete |

Sessions are deliberately **not** cached: they slide on every read and
revoke immediately. Settings already have their own 30s cache from the
original build. `CachedGetOrLoad` wraps every fill in singleflight — a hot
key expiring under load loads once, not once per waiter.

## ETag on heavy reads

`app.etag()` buffers 200 bodies (bounded at 2 MiB; larger responses pass
through untagged) and serves a strong ETag. Send `If-None-Match` and an
unchanged payload answers **304** without a body. Applied to: users list +
detail, jobs list, tickets list, analytics summary. Error responses are
byte-identical to the un-middleware'd path.

## Search

`GET /api/search?q=&limit=` — one endpoint, two permission-scoped sections:

```json
{"users": [...], "tickets": [...]}
```

- **Postgres adapter**: generated tsvector columns (`to_tsvector('simple',
  name || ' ' || email)` for users, title + description for tickets) with
  GIN indexes (migration 000016), ranked by `ts_rank`, queried with
  `websearch_to_tsquery` so plain phrases like `invoices approved` work.
- **Memory adapter**: case-insensitive substring (dev mode parity).
- **Scoping**: users need `users:read`; tickets need `tickets:read`, and
  without `tickets:read_all` only the caller's own tickets return — same
  visibility contract as the list endpoints.

## Try it locally

```bash
curl -s "localhost:8080/api/search?q=expense" -H "Authorization: Bearer $TOKEN" | jq
curl -sI "localhost:8080/api/users?limit=5" -H "Authorization: Bearer $TOKEN" | grep -i etag
curl -s "localhost:8080/api/users?limit=5" -H "Authorization: Bearer $TOKEN" -H "If-None-Match: <etag>" -i | head -1   # 304
```
