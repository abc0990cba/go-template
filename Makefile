.PHONY: run worker test test-integration test-e2e test-performance seed-users seed-demo seed-demo-reset web-install web-dev web-build web-lint web-test perf-login perf-users perf-analytics perf-enrichment lint vet coverage generate docker-up docker-down demo-up demo-down demo-kafka-profile demo-nats-contact demo-enrichment demo-dlq demo-outbox demo-breaker demo-ratelimit demo-idempotency openspec-validate
run:
	go run ./cmd/api

worker:
	go run ./cmd/worker

test:
	go test -race ./...

test-integration:
	go test -tags=integration ./test/integration/...

test-e2e:
	go test -tags=e2e ./test/e2e/... -count=1

test-performance:
	@command -v k6 >/dev/null 2>&1 && k6 run test/performance/smoke.js || echo "k6 is not installed; install k6 to run performance tests"

# Performance scenarios need a live stack with seeded users:
#   make docker-up && make seed-users && make perf-login
seed-users:
	go run ./cmd/seed -users ${USERS:-100000}

# Demo dataset for the SPA: realistic users, jobs, reports, and a 30-day
# event history (ClickHouse when configured).
seed-demo:
	go run ./cmd/seed-demo

seed-demo-reset:
	go run ./cmd/seed-demo -reset

web-install:
	cd web && npm ci

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-lint:
	cd web && npm run lint && npm run format:check

web-test:
	cd web && npm test

K6 = k6 run
perf-login:
	$(K6) test/performance/login.js

# MODE=offset|cursor|keyset picks the pagination scenario
perf-users:
	$(K6) -e MODE=${MODE:-offset} test/performance/users-list.js

perf-analytics:
	$(K6) test/performance/analytics.js

perf-enrichment:
	$(K6) test/performance/enrichment.js

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint is not installed; see .golangci.yml (go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...)"

vet:
	go vet ./...

coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out | tail -1

generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest -config api/openapi/openapi.cfg.yaml api/openapi/openapi.yaml

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

# --- Enrichment streams demo (Kafka + NATS) --------------------------------
# Brings up the core stack plus Redpanda (Kafka API) and NATS with JetStream,
# then use the demo-* targets to push events through the streams and watch
# them land in the app.

DEMO_COMPOSE = docker compose -f docker-compose.yml -f docker-compose.brokers.yml

demo-up:
	$(DEMO_COMPOSE) up -d --build
	@echo ""
	@echo "Stack ready: api http://localhost:8080 (demo login $(shell grep -m1 'DEMO_EMAIL' .env.example 2>/dev/null | cut -d= -f2) or see .env.example)"
	@echo "Now try: make demo-enrichment | make demo-dlq | make demo-outbox | make demo-breaker | make demo-ratelimit | make demo-idempotency"

demo-down:
	$(DEMO_COMPOSE) down -v

DEVTOOLS = go run ./cmd/devtools

# Publish one profile event for a demo user to Kafka: the user's detail page
# in the app gains the "Profile (Kafka)" enrichment card.
demo-kafka-profile:
	$(DEVTOOLS) kafka-profile --ref demo-u-0001 --seq ${SEQ:-1} --company "${COMPANY:-Acme Corp}"

# Publish one contact event to NATS JetStream: the "Contact (NATS)" card.
demo-nats-contact:
	$(DEVTOOLS) nats-contact --ref demo-u-0001 --seq ${SEQ:-1} --phone "+1 555 0100"

# End to end: both streams for one user, then poll the API for the merge.
demo-enrichment:
	$(DEVTOOLS) demo --ref demo-u-0001

# Poison a message, watch it land in dead letters, then replay it.
demo-dlq:
	$(DEVTOOLS) demo-dlq

# Watch the outbox relay publish user.enriched to Kafka and print it.
demo-outbox:
	$(DEVTOOLS) consume-user-events --timeout 15s

# Circuit breaker: a flaky upstream trips app_cb_state open, then recovers.
demo-breaker:
	$(DEVTOOLS) demo-breaker

# Rate limiting: hammer the API until 429s come back with Retry-After.
demo-ratelimit:
	$(DEVTOOLS) hammer --requests 200 --concurrency 10

# Idempotency: POST the same ticket twice with one key; the retry replays.
demo-idempotency:
	./scripts/demo-idempotency.sh

openspec-validate:
	@command -v openspec >/dev/null 2>&1 && openspec validate || echo "openspec CLI is not installed; see docs/openspec.md"
