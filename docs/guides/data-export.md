# Data Export (GDPR Portability)

Self-service export of everything the account owns or is the subject of.

## Flow

1. `POST /api/users/me/export` → `202 {"job_id": …}` — a real queue job
   (`user.export_generate`, visible on the admin Jobs page). A second
   request while one is queued/running answers 409. Without Redis (dev
   in-memory mode) the request answers 400 naming the missing dependency —
   no fake success.
2. `GET /api/users/me/export` → job status.
3. `GET /api/users/me/export/download` → the ZIP once ready; **202 +
   Retry-After** while the job runs.

## Bundle contents

One JSON document per entity, zipped: `profile.json`, `identities.json`,
`sessions.json`, `tickets.json`, `notifications.json`,
`audit_entries_about_me.json`, `files.json`. The bundle is stored through
the file store (S3/MinIO or in-process) as a file **owned by the requester**;
ownership is checked again at download.

Non-ticket files are owner-only, period — plain uploads and export bundles
cannot be read, listed, or probed by anyone else. Ticket attachments keep
the ticket's shared visibility (`canView`).

## Retention

Bundles expire after **7 days**; the worker prunes expired metadata and
blobs, so the download endpoint answers not-found after that window (request
a fresh export anytime).

## Non-goal (v1)

Erasure (right to be forgotten) is intentionally out of scope: it intersects
the immutable audit log and cross-entity foreign keys, and deserves its own
design. Track it as a follow-up capability.

## Try it locally

**Queue mode** (`APP_REDIS_ADDR` + the worker — `make docker-up`):

```bash
curl -s -X POST localhost:8080/api/users/me/export -H "Authorization: Bearer $TOKEN"
curl -s localhost:8080/api/users/me/export -H "Authorization: Bearer $TOKEN"
curl -s -o my-data.zip localhost:8080/api/users/me/export/download -H "Authorization: Bearer $TOKEN"
unzip -l my-data.zip
```

Remember the multi-process note above: point `APP_S3_*` at MinIO
(`--profile files`) so the api can stream the worker-produced bundle.

**Single-process mode** (no Redis — jobs run when triggered, like every
other job type):

```bash
curl -s -X POST localhost:8080/api/users/me/export -H "Authorization: Bearer $TOKEN"   # 202 queued
curl -s -X POST localhost:8080/api/jobs/process -H "Authorization: Bearer $TOKEN"      # run it inline
curl -s -o my-data.zip localhost:8080/api/users/me/export/download -H "Authorization: Bearer $TOKEN"
```
