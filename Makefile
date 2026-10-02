.PHONY: helm-lint helm-template run worker test test-integration test-e2e test-performance coverage coverage-check seed-users seed-demo seed-demo-reset web-install web-dev web-build web-lint web-test check-size perf-login perf-users perf-analytics perf-enrichment lint vet generate docker-up docker-down demo-up demo-down demo-kafka-profile demo-nats-contact demo-enrichment demo-dlq demo-outbox demo-breaker demo-ratelimit demo-idempotency chaos-up chaos-down chaos-status chaos-pg-latency chaos-pg-down chaos-redis-down chaos-restore nomad-up nomad-run nomad-stop nomad-validate kind-up kind-release kind-down openspec-validate
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

check-size: ## fail if any hand-written production Go file exceeds 400 lines
	@oversized=$$(find internal cmd -name '*.go' \
		! -name '*_test.go' ! -name '*.gen.go' ! -path '*postgres/db*' -exec wc -l {} + | \
		awk '$$1 > 400 && $$2 != "total" {print $$1 "\t" $$2}'); \
	if [ -n "$$oversized" ]; then \
		echo "FILES OVER THE 400-LINE BUDGET:"; echo "$$oversized"; exit 1; \
	else echo "size budget OK (all hand-written production files <= 400 lines)"; fi

web-test:
	cd web && npm test

web-check-boundaries: ## fail if any web file breaks the slice architecture rules
	cd web && npx depcruise "src/**/*.ts?(x)" --config .dependency-cruiser.cjs

check-size-web: ## fail if any hand-written web file exceeds its budget (400; routes 80)
	@oversized=$$(find web/src \( -name '*.ts' -o -name '*.tsx' \) \
		! -name '*test*' ! -name 'routeTree.gen.ts' \
		! -path 'web/src/shared/components/ui/*' \
		-exec wc -l {} + | awk '$$1 > 400 && $$2 != "total" {print $$1 "\t" $$2}'); \
	routes=$$(find web/src/routes -name '*.tsx' -exec wc -l {} + | awk '$$1 > 80 && $$2 != "total" {print $$1 "\t" $$2}'); \
	if [ -n "$$oversized$$routes" ]; then \
		echo "WEB FILES OVER BUDGET:"; echo "$$oversized$$routes"; exit 1; \
	else echo "web size budget OK (<=400 per file, <=80 per route)"; fi

web-coverage:
	cd web && npm run test:coverage

web-test-e2e:
	cd web && npx playwright install chromium && npm run test:e2e

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

# The floor guards the whole hand-written internal tree: -coverpkg counts
# never-tested packages as zero, and generated files (OpenAPI types, sqlc
# queries — guarded by the codegen job instead) are excluded from the
# statement sums. The merged profile repeats a block once per instrumenting
# test binary, so the awk dedupes by block and keeps the max hit count —
# the same semantics as `go tool cover -func`.
COVERAGE_FLOOR ?= 46

coverage-check:
	go test -race -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage.out ./internal/... ./cmd/...
	awk -v floor="$(COVERAGE_FLOOR)" '!/api\.gen\.go|platform\/postgres\/db\// && NF == 3 && $$2 ~ /^[0-9]+$$/ { if (!seen[$$1]++) { stmts[$$1] = $$2 } if ($$3 + 0 > hit[$$1]) { hit[$$1] = $$3 + 0 } } END { for (b in stmts) { total += stmts[b]; if (hit[b] > 0) covered += stmts[b] } if (total == 0) { print "no coverage data"; exit 1 } pct = 100 * covered / total; if (pct < floor) { printf "coverage %.1f%% is below the %.0f%% floor\n", pct, floor; exit 1 } else { printf "coverage %.1f%% meets the %.0f%% floor\n", pct, floor } }' coverage.out

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

# --- Chaos lab (docs/chaos.md) -----------------------------------------------
# Fault injection through Toxiproxy: the app talks to postgres/redis through
# the proxy and you break the middle. Best experienced with the
# observability profile up so the dashboards and alerts react.

CHAOS_COMPOSE = docker compose -f docker-compose.yml -f docker-compose.chaos.yml
CHAOS = ./scripts/chaos.sh

chaos-up:
	$(CHAOS_COMPOSE) --profile chaos up -d

chaos-down:
	$(CHAOS_COMPOSE) --profile chaos down

chaos-status:
	$(CHAOS) status

# Add latency to every Postgres round-trip: make chaos-pg-latency MS=2000
chaos-pg-latency:
	$(CHAOS) pg-latency ${MS:-2000}

chaos-pg-down:
	$(CHAOS) pg-down

chaos-redis-down:
	$(CHAOS) redis-down

chaos-restore:
	$(CHAOS) pg-restore && $(CHAOS) redis-restore

helm-lint:
	helm lint deploy/helm/go-template

helm-template:
	helm template go-template deploy/helm/go-template

# --- Progressive delivery (ADR 0024) -----------------------------------------
# Primary path: Kubernetes canary with metric-gated auto-rollback (Argo
# Rollouts) — scripts/kind-demo.sh walks through it on a local kind cluster.
# Lighter alternative: Nomad's native canary + auto-revert (deploy/nomad/).

kind-up:
	./scripts/kind-demo.sh up

kind-release:
	./scripts/kind-demo.sh release

kind-down:
	./scripts/kind-demo.sh down

nomad-up:
	nomad agent -dev >/tmp/nomad-dev.log 2>&1 & echo "nomad dev agent started (log: /tmp/nomad-dev.log, UI: http://localhost:4646)"

nomad-validate:
	nomad job validate deploy/nomad/go-template.nomad.hcl

nomad-run:
	nomad job run deploy/nomad/go-template.nomad.hcl

nomad-stop:
	pkill -f "nomad agent -dev" || true

openspec-validate:
	@command -v openspec >/dev/null 2>&1 && openspec validate || echo "openspec CLI is not installed; see docs/openspec.md"
