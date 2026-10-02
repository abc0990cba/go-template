#!/bin/sh
# One-terminal dev loop: `make dev`.
#
# Runs three processes and tears them all down on Ctrl-C:
#   1. the Go API with air hot-reload (falls back to `go run` when air is
#      not installed — reloads then require a manual restart);
#   2. the OpenAPI→TypeScript generator in watch mode, so editing
#      api/openapi/openapi.yaml rewrites web/src/shared/api/schema.gen.ts
#      and the typed client surfaces the change at compile time;
#   3. the Vite dev server, which proxies /api to the API.
set -eu

cd "$(dirname "$0")/.."

AIR_VERSION=${AIR_VERSION:-v1.67.4}

cleanup() {
  trap - EXIT INT TERM
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
pids=""
trap cleanup EXIT INT TERM

if command -v air >/dev/null 2>&1; then
  air -c .air.toml &
  pids="$pids $!"
else
  echo "air is not installed — falling back to 'go run' (no hot reload)." \
    "Install it with: go install github.com/air-verse/air@${AIR_VERSION}"
  go run ./cmd/api &
  pids="$pids $!"
fi

(cd web && npm run gen:api:watch) &
pids="$pids $!"

(cd web && npm run dev) &
pids="$pids $!"

wait
