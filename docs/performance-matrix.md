# Performance Matrix

| Scenario | Script | Target | Status |
|---|---|---|---|
| Health smoke | `test/performance/smoke.js` | p95 < 500ms, errors < 1% | Implemented |
| Login burst | `test/performance/login.js` | p95 < 300ms | Planned |
| Users offset pagination | `test/performance/users-offset.js` | p95 < 500ms on 100k users | Planned |
| Users cursor pagination | `test/performance/users-cursor.js` | p95 < 300ms on 100k users | Planned |
| Users keyset pagination | `test/performance/users-keyset.js` | p95 < 250ms on 100k users | Planned |
| Enrichment batch | `test/performance/enrichment.js` | bounded memory and retries | Planned |
| Analytics dashboard | `test/performance/analytics.js` | p95 < 400ms | Planned |
