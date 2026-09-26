## 1. Warehouse

- [x] 1.1 ClickHouse client adapter with startup migrations and ping
- [x] 1.2 Batching EventBuffer: flush by size/interval, final flush on Close, drops logged
- [x] 1.3 EventCounts aggregate

## 2. Ports And Wiring

- [x] 2.1 EventSink/EventStats ports replace AddEvent in auth/job store ports
- [x] 2.2 Services emit through the injected sink; bootstrap opens ClickHouse and compounds sinks
- [x] 2.3 Summary merges warehouse counters with graceful degradation; dashboard panel; contract fields

## 3. Verification

- [x] 3.1 Integration test: sink → flush → counts; migration idempotency (skips without the image)
