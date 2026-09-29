## Why

Real workflows carry documents: approval tickets need evidence, exports need somewhere to land (the GDPR work next), and every template pretending at production shape needs an object-storage story. Today the app cannot store a single byte of user content.

## What Changes

- Add a `FileStore` port (`internal/platform/filestore/`): an S3 adapter (`aws-sdk-go-v2/service/s3`, works with AWS S3 or MinIO, path-style optional, presigned GET URLs) and an in-process adapter for dev. Bootstrap prefers S3 when `APP_S3_BUCKET` is set and degrades to memory with a warning when the bucket is unreachable — the same optional-infrastructure contract as ClickHouse/Redis.
- Add `files` metadata (migration 000018, sqlc, memory + postgres behind `FilesRepo`): owner, org, filename, content type, size, sha256 (computed while streaming), polymorphic `ref_type`/`ref_id` attachment pointer, soft delete.
- Add `FilesService`: upload with per-file size cap (`APP_FILES_MAX_MB`, default 10), checksum, and attachment validation — a `ticket` reference must pass the ticket's `canView` rules in the active domain before the blob is accepted; downloads re-check visibility; presigned URLs only in S3 mode (501 otherwise); deletes are owner-only.
- HTTP: `POST /api/files` (multipart; this route's body cap becomes the upload limit + framing instead of 1 MiB), `GET /api/files/{id}`, `GET /api/files/{id}/content` (streamed through the API in every mode), `GET /api/files/{id}/url` (presign), `DELETE /api/files/{id}`, `GET /api/tickets/{id}/attachments`. Ticket attachments append an `attachment_added` timeline entry and notify watchers.
- Compose: MinIO under the `files` profile (host ports 19000/19001; 9000 collides with ClickHouse).

## Capabilities

### New Capabilities

- `files`

## Impact

- `aws-sdk-go-v2` joins go.mod; `testcontainers-go/modules/minio` joins the integration test deps.
- Export (next change) stores its bundles through this port.
