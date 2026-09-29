## 1. Domain Logic
- [x] 1.1 `ExportService` (Generate/Latest/Open/Prune) + `user.export_generate` job type + queue handler through `RunJob`
- [x] 1.2 `RequestExport` (duplicate guard, no-queue degradation) + `LatestExport`
- [x] 1.3 Owner-only rule for non-ticket files; `bootstrap.FileStore` shared by api + worker; worker exporter wiring

## 2. HTTP Surface
- [x] 2.1 Request/status/download endpoints + routes + OpenAPI

## 3. Operations
- [x] 3.1 Retention pruning loop hook (worker export prune via Prune in the jobs lifecycle)

## 4. Verification And Docs
- [x] 4.1 Service tests (bundle roundtrip ZIP contents, ownership + pruning, private-file download guard) + router test (degraded mode, seeded download)
- [x] 4.2 docs/data-export.md, traceability
