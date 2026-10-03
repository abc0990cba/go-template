.PHONY: helm-lint helm-template run dev hooks worker test test-integration test-e2e test-performance coverage coverage-check seed-users seed-demo seed-demo-reset web-install web-dev web-build web-lint web-test check-size perf-login perf-users perf-analytics perf-enrichment lint vet generate lint-spec vulncheck hooks docker-up docker-down demo-up demo-down demo-kafka-profile demo-nats-contact demo-enrichment demo-dlq demo-outbox demo-breaker demo-ratelimit demo-idempotency chaos-up chaos-down chaos-status chaos-pg-latency chaos-pg-down chaos-redis-down chaos-restore nomad-up nomad-run nomad-stop nomad-validate kind-up kind-release kind-down openspec-validate vps-setup vps-doctor vps-deploy vps-check vps-app-deploy vps-update vps-rollback vps-status vps-logs vps-ui vps-backup vps-restore vps-acme-production
run:
	go run ./cmd/api

# One-terminal dev loop: hot-reloading API, contract→TS watch, and Vite.
dev:
	./scripts/dev.sh

# Install the pre-commit hooks (lefthook must be on PATH; see lefthook.yml).
hooks:
	lefthook install

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
		! -name '*test*' ! -name '*.gen.ts' \
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

# Codegen and lint tooling, version-pinned so every machine and CI run
# produces byte-identical output. The module@version form is one-line so
# Renovate's regex manager can bump each pin.
SQLC := github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI_CODEGEN := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
VACUUM := github.com/daveshanley/vacuum@v0.30.6
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0

generate:
	go run $(SQLC) generate
	go run $(OAPI_CODEGEN) -config api/openapi/openapi.cfg.yaml api/openapi/openapi.yaml
	cd web && npm run gen:api

# The contract lint: operationIds present (the TS generator keys on them),
# every operation documents JSON responses with schemas, problem+json errors
# declared. Runs in CI's codegen job next to the drift gate.
lint-spec:
	go run $(VACUUM) lint -r api/openapi/.vacuum.json api/openapi/openapi.yaml

vulncheck:
	go run $(GOVULNCHECK) ./...

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

# --- VPS deployment (Ansible + Nomad; docs/vps/QUICKSTART.md) -----------------
# One VPS, one command: `make vps-setup` once, then `make vps-deploy` forever.
# Add SITE=dev to any target to operate on the dev VPS instead of prod.

VPS_SITE ?= prod
VPS = deploy/ansible

vps-setup: ## Guided first-time setup: questions -> secrets -> vault -> inventory
	@$(VPS)/scripts/vps-setup.sh

vps-doctor: ## Verify prerequisites, SSH, DNS records, VPS size (read-only)
	@$(VPS)/scripts/vps-doctor.sh

vps-deploy: ## THE one command: full stack converge on the VPS (idempotent)
	@$(VPS)/scripts/run-playbook.sh site.yml

vps-check: ## Dry-run the full deploy (no changes made)
	@$(VPS)/scripts/run-playbook.sh site.yml --check --diff

vps-app-deploy: ## Deploy an app image tag: make vps-app-deploy TAG=v1.2.3
	@test -n "$(TAG)" || { echo "usage: make vps-app-deploy TAG=v1.2.3"; exit 1; }
	@$(VPS)/scripts/run-playbook.sh deploy-app.yml -e app_image_tag=$(TAG)

vps-update: ## Re-render + redeploy jobs: make vps-update JOBS="woodpecker"
	@test -n "$(JOBS)" || { echo 'usage: make vps-update JOBS="caddy monitoring"'; exit 1; }
	@$(VPS)/scripts/run-playbook.sh deploy-app.yml -e jobs="$(JOBS)"

vps-rollback: ## Revert the app to the previous successful deployment
	@$(VPS)/scripts/vps-rollback.sh

vps-status: ## Nomad node + job status on the VPS
	@$(VPS)/scripts/vps-remote.sh 'nomad-admin node status && echo && nomad-admin job status'

vps-logs: ## Follow logs: make vps-logs S=api|worker|postgres|caddy|woodpecker
	@$(VPS)/scripts/vps-remote.sh 'docker logs --tail 200 -f $$(docker ps --format "{{.Names}}" | grep -i "$(S)" | head -1)'

vps-ui: ## SSH tunnels: Nomad UI (127.0.0.1:4646) + Grafana (127.0.0.1:3001)
	@$(VPS)/scripts/vps-ui.sh

vps-backup: ## Run a backup now and list recent archives
	@$(VPS)/scripts/vps-remote.sh 'systemctl start go-template-backup.service && ls -lht /opt/go-template/backups | head -8'

vps-restore: ## Restore a dump: make vps-restore FILE=go_template-YYYYMMDD-HHMMSS.dump
	@test -n "$(FILE)" || { echo "usage: make vps-restore FILE=go_template-YYYYMMDD-HHMMSS.dump"; exit 1; }
	@$(VPS)/scripts/vps-restore.sh $(FILE)

vps-acme-production: ## Switch Let's Encrypt staging -> production, redeploy Caddy
	@sed -i.bak 's/^acme_staging: true/acme_staging: false/' $(VPS)/inventories/$(VPS_SITE)/group_vars/vps/main.yml && rm -f $(VPS)/inventories/$(VPS_SITE)/group_vars/vps/main.yml.bak
	@$(VPS)/scripts/run-playbook.sh deploy-app.yml -e jobs=caddy
	@echo "Caddy reloaded against the production LE CA. Certificates are re-issued automatically."
