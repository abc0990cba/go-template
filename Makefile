.PHONY: run worker test test-integration test-e2e test-performance lint vet coverage generate docker-up docker-down openspec-validate
run:
	go run ./cmd/api

worker:
	go run ./cmd/worker

test:
	go test -race ./...

test-integration:
	go test -tags=integration ./test/integration/...

test-e2e:
	go test ./test/e2e/...

test-performance:
	@command -v k6 >/dev/null 2>&1 && k6 run test/performance/smoke.js || echo "k6 is not installed; install k6 to run performance tests"

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint is not installed; see .golangci.yml (go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...)"

vet:
	go vet ./...

coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
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
