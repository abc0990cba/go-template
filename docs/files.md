# File Storage & Ticket Attachments

The template stores user content through one port, two adapters:

| Adapter | When | Notes |
|---|---|---|
| **S3** (`aws-sdk-go-v2`) | `APP_S3_BUCKET` set | Works with AWS S3 or MinIO; path-style for MinIO; startup probe (bucket + credentials); presigned GET URLs |
| **In-process** | otherwise | Dev convenience — blobs live in RAM, restart loses them, presign answers 501 |

An unreachable bucket **degrades** like every other optional dependency:
startup logs a warning and the in-process store takes over.

## API

| Endpoint | Notes |
|---|---|
| `POST /api/files` | multipart `file` (+ optional `ref_type`/`ref_id`); body cap = `APP_FILES_MAX_MB` + framing (default 10 MiB) instead of the global 1 MiB |
| `GET /api/files/{id}` | metadata incl. sha256 checksum |
| `GET /api/files/{id}/content` | streamed through the API — works in every mode; `Content-Disposition: attachment` |
| `GET /api/files/{id}/url` | presigned URL (15 min), S3 mode only |
| `DELETE /api/files/{id}` | owner only; soft-deletes metadata, drops the blob |
| `GET /api/tickets/{id}/attachments` | ticket-visibility-checked list |

## Permission model

- Uploads are self-service; the metadata row records the owner.
- **Ticket attachments** (`ref_type=ticket`) are accepted only if the
  uploader passes the ticket's `canView` rules in the active domain, and
  every read re-applies the same check — attaching is not a way to read a
  hidden ticket, and a hidden ticket's attachments cannot be downloaded.
- A successful attach appends `attachment_added` to the ticket timeline and
  notifies watchers; the upload never fails because of the notification.

## Run it locally

```bash
docker compose --profile files up -d minio   # console at :19001
APP_S3_ENDPOINT=http://127.0.0.1:19000 APP_S3_BUCKET=app-files \
APP_S3_ACCESS_KEY_ID=filesapp APP_S3_SECRET_ACCESS_KEY=files-secret \
APP_S3_USE_PATH_STYLE=1 go run ./cmd/api
# (create the bucket once: mc mb local/app-files — or via the console UI)
```

Then `curl -F file=@report.pdf -F ref_type=ticket -F ref_id=<id> localhost:8080/api/files`.
Without the S3 env the same commands work against the in-memory store.
