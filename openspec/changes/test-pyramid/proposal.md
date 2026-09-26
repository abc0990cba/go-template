## Why

Integration and e2e columns of the test matrix were "Planned", no e2e test existed, and CI measured nothing about coverage.

## What Changes

- Replace the e2e scaffold with an API happy-path suite (register → login → me → users → enqueue report → poll to done → analytics/reports) that runs against the compose stack and skips itself when the stack is down.
- CI coverage job: measure `./internal/...` with the race detector and enforce a 55% floor (70% target); current baseline ~60%.
- CI e2e job: start the compose stack, wait for `/health/ready`, run the suite, collect logs on failure.
- Matrices updated: integration/e2e cells now reference real suites.

## Capabilities

### New Capabilities

- none (strengthens `engineering-experience` and the existing capability tests)

## Impact

- e2e runs require Docker on the runner; the local `go test ./...` remains unaffected.
