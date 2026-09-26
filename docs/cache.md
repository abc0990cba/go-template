# Cache

Redis is planned for:

- sessions
- rate limiting
- hot user profile cache
- job queue backend

Rules:

- Every temporary key needs a TTL.
- Cache misses must be safe.
- Redis outage behavior must be explicit.
- Cache must not be the source of truth for operational data.
