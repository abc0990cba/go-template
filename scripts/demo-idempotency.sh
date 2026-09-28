#!/bin/sh
# Idempotency demo: POST the same ticket twice with one Idempotency-Key and
# watch the second call replay the stored response instead of double-filing.
#
# Needs the demo stack up (make demo-up) and jq on PATH.
set -eu

API="${API_BASE:-http://localhost:8080}"
EMAIL="${DEMO_EMAIL:-demo@go-template.local}"
PASSWORD="${DEMO_PASSWORD:-demo-password-123}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this demo (brew install jq)"; exit 1
fi

TOKEN=$(curl -fsS -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" \
  "$API/api/auth/login" | jq -r .token)

# Filing a ticket needs a ticket type; take the first one available
# (the demo seed creates several).
TYPE_ID=$(curl -fsS -H "Authorization: Bearer $TOKEN" "$API/api/ticket-types?limit=1" | jq -r '.data[0].id // empty')
if [ -z "$TYPE_ID" ]; then
  echo "No ticket types found; seed the demo data first (APP_DEMO_SEED=1 or make seed-demo)."; exit 1
fi

BODY="{\"type_id\":\"$TYPE_ID\",\"title\":\"Idempotent ticket\",\"description\":\"Filed by devtools\",\"priority\":\"medium\"}"
KEY="devtools-$(date +%s)"

echo "POST #1 (creates the ticket):"
curl -sS -D /tmp/idem1.h -o /tmp/idem1.json -X POST "$API/api/tickets" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY" -d "$BODY"
head -1 /tmp/idem1.h
jq -c '{id, title, status}' /tmp/idem1.json || jq -c . /tmp/idem1.json

echo ""
echo "POST #2 (same key, same body — the backend replays):"
curl -sS -D /tmp/idem2.h -o /tmp/idem2.json -X POST "$API/api/tickets" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY" -d "$BODY"
grep -i 'idempotency-replayed\|HTTP/' /tmp/idem2.h
jq -c '{id, title, status}' /tmp/idem2.json || jq -c . /tmp/idem2.json

echo ""
if cmp -s /tmp/idem1.json /tmp/idem2.json; then
  echo "Same response both times, Idempotency-Replayed: true on the retry — no double file."
else
  echo "Responses differ; check that the api runs with idempotency wired (Idempotency repo set)."
  exit 1
fi
