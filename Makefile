.PHONY: run worker test test-integration test-e2e test-performance seed-users perf-login perf-users perf-analytics perf-enrichment lint vet coverage generate docker-up docker-down openspec-validate
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

openspec-validate:
	@command -v openspec >/dev/null 2>&1 && openspec validate || echo "openspec CLI is not installed; see docs/openspec.md"
