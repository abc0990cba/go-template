## 1. Storage Layer
- [x] 1.1 `FileStore` port + memory + S3 adapters (probe, presign, not-found mapping)
- [x] 1.2 Migration 000018 + sqlc + model + `FilesRepo` port + memory/postgres adapters
- [x] 1.3 Config (`APP_S3_*` with `_FILE` secrets, `APP_FILES_MAX_MB`), bootstrap degradation, compose `files` profile

## 2. Domain Logic
- [x] 2.1 `FilesService`: upload cap + checksum + attachment validation, visibility-checked downloads, presign, owner-only delete
- [x] 2.2 Ticket timeline + notification on attach; container ordering (Files after Tickets)

## 3. HTTP Surface
- [x] 3.1 Multipart upload with raised body cap (incl. chunked 413), metadata/stream/presign/delete handlers, ticket attachments list
- [x] 3.2 OpenAPI paths + schemas + regen

## 4. Verification And Docs
- [x] 4.1 Router tests (roundtrip, delete, attachment visibility, unknown-ticket refusal) + MinIO contract test (integration tag)
- [x] 4.2 docs/files.md, traceability
