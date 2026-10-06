#!/bin/sh
# Chaos lab: inject faults through Toxiproxy. See docs/operations/chaos.md for the
# game-day scenarios each script supports. Requires the chaos profile:
#
#   docker compose -f docker-compose.yml -f docker-compose.chaos.yml \
#     --profile chaos up -d
#
# Usage: chaos.sh <command>
#   pg-latency [ms]      add latency to every Postgres round-trip (default 2000)
#   pg-down              cut the Postgres proxy (timeouts, not refusals)
#   pg-restore           remove all Postgres toxics and bring the proxy back
#   redis-down           cut the Redis proxy
#   redis-restore        restore the Redis proxy
#   status               list proxies and active toxics
set -eu
API="${TOXIPROXY_API:-http://localhost:8474}"

post() { curl -sS -X POST "$@" >/dev/null; }

case "${1:-status}" in
  pg-latency)
    MS="${2:-2000}"
    post "$API/proxies/pg/post_toxics" -H 'Content-Type: application/json' \
      -d "{\"name\":\"latency\",\"type\":\"latency\",\"stream\":\"downstream\",\"toxicity\":1.0,\"attributes\":{\"latency\":$MS,\"jitter\":50}}"
    echo "postgres latency: ${MS}ms per round-trip"
    ;;
  pg-down)
    post "$API/proxies/pg/post_toxics" -H 'Content-Type: application/json' \
      -d '{"name":"cut","type":"timeout","stream":"downstream","toxicity":1.0,"attributes":{}}'
    echo "postgres: requests through the proxy now time out"
    ;;
  pg-restore)
    curl -sS -X DELETE "$API/proxies/pg/post_toxics/latency" >/dev/null 2>&1 || true
    curl -sS -X DELETE "$API/proxies/pg/post_toxics/cut" >/dev/null 2>&1 || true
    echo "postgres: toxics removed"
    ;;
  redis-down)
    post "$API/proxies/redis/post_toxics" -H 'Content-Type: application/json' \
      -d '{"name":"cut","type":"timeout","stream":"downstream","toxicity":1.0,"attributes":{}}'
    echo "redis: requests through the proxy now time out"
    ;;
  redis-restore)
    curl -sS -X DELETE "$API/proxies/redis/post_toxics/cut" >/dev/null 2>&1 || true
    echo "redis: toxics removed"
    ;;
  status)
    curl -sS "$API/proxies" | python3 -m json.tool
    ;;
  *)
    echo "unknown command: $1" >&2
    exit 1
    ;;
esac
