## Why

Data portability (GDPR art. 20) is a standard enterprise expectation: users can request everything the system holds about them, as data they can take elsewhere. The template has the pieces (jobs, file storage, audit trail) but no self-service export.

## What Changes

- Add `user.export_generate` (jobs + queue mux handler): the worker collects the caller's profile, linked identities, session metadata, tickets, notifications, audit entries about them, and file metadata; packs one JSON document per entity into a ZIP; stores it through the `FileStore` with a `files` row owned by the requester (`ref_type=export`).
- `JobService.RequestExport` enqueues with a one-live-export-per-user conflict guard; `LatestExport` resolves status. The queue mux handler runs it through the existing `RunJob` lifecycle so the jobs page shows it like any other work item.
- `ExportService` (`internal/service/export.go`): Generate / Latest / Open / Prune (7-day retention; the worker prunes metadata + blobs).
- HTTP: `POST /api/users/me/export` (202; 409 on a live duplicate; 400 naming the missing queue in no-Redis dev mode), `GET /api/users/me/export` (status), `GET /api/users/me/export/download` (200 ZIP; 202 + Retry-After while running). Downloads authenticate as the owner — stronger than a one-time token, and the ownership rule now covers every non-ticket file (plain uploads are owner-only too).
- Both binaries select the blob store through `bootstrap.FileStore`, so exports are produced by the worker in every mode.
- Erasure (right to be forgotten) stays an explicit non-goal for v1: it intersects audit immutability and FK topology and deserves its own change.

## Capabilities

### New Capabilities

- `data-export`

## Impact

- Non-ticket files become owner-only (metadata + bytes); ticket attachments keep shared visibility via `canView`.
- The worker gains the file store; the export bundle is the first worker-produced artifact.
