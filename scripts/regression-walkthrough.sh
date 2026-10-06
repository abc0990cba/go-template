#!/usr/bin/env bash
# Live regression walkthrough for go-template (run against a seeded stack).
# Usage: BASE=http://localhost:18080 ./scripts/regression-walkthrough.sh
set -u
BASE="${BASE:-http://localhost:18080}"
PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); echo "  ✓ $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  ✗ $1"; }
check() { # check <desc> <expected> <actual>
  if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want $2, got $3)"; fi
}
jqget() { python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('$1',''))" 2>/dev/null; }

echo "── 1. Auth + admin bootstrap"
ADMIN=$(curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}' | jqget token)
[ -n "$ADMIN" ] && ok "demo admin login" || bad "demo admin login"
ADMIN_ID=$(curl -s "$BASE/api/me" -H "Authorization: Bearer $ADMIN" | jqget id)

echo "── 2. MFA: enroll → challenge login → recovery → disable"
SECRET=$(curl -s -X POST "$BASE/api/auth/mfa/enroll" -H "Authorization: Bearer $ADMIN" | jqget secret)
[ -n "$SECRET" ] && ok "mfa enroll returns secret" || bad "mfa enroll"
# generate a TOTP code (RFC 6238, sha1, 30s, 6 digits) with python
CODE=$(python3 - "$SECRET" <<'EOF'
import hmac, hashlib, base64, struct, time, sys
secret = sys.argv[1]
pad = '=' * ((8 - len(secret) % 8) % 8)
key = base64.b32decode(secret.upper() + pad, casefold=True)
counter = int(time.time()) // 30
msg = struct.pack('>Q', counter)
digest = hmac.new(key, msg, hashlib.sha1).digest()
o = digest[19] & 15
print('%06d' % ((struct.unpack('>I', digest[o:o+4])[0] & 0x7fffffff) % 1000000))
EOF
)
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/mfa/verify" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}")
check "mfa verify-enable" 200 "$STATUS"
RECOVERY=$(curl -s -X POST "$BASE/api/auth/mfa/verify" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}" | jqget recovery_codes 2>/dev/null)
LOGIN=$(curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}')
CHALLENGE=$(echo "$LOGIN" | jqget challenge)
[ -n "$CHALLENGE" ] && ok "login answers mfa challenge" || bad "login challenge (got: $(echo "$LOGIN" | head -c 120))"
CODE2=$(python3 - "$SECRET" <<'EOF'
import hmac, hashlib, base64, struct, time, sys
secret = sys.argv[1]
pad = '=' * ((8 - len(secret) % 8) % 8)
key = base64.b32decode(secret.upper() + pad, casefold=True)
counter = int(time.time()) // 30
msg = struct.pack('>Q', counter)
digest = hmac.new(key, msg, hashlib.sha1).digest()
o = digest[19] & 15
print('%06d' % ((struct.unpack('>I', digest[o:o+4])[0] & 0x7fffffff) % 1000000))
EOF
)
SESSION=$(curl -s -X POST "$BASE/api/auth/mfa/challenge" -H 'Content-Type: application/json' \
  -d "{\"challenge\":\"$CHALLENGE\",\"code\":\"$CODE2\"}" | jqget token)
[ -n "$SESSION" ] && ok "challenge completes with TOTP" || bad "challenge TOTP"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/mfa/disable" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"password":"demo-password-123","code":"'"$CODE2"'"}')
check "mfa disable (password+code)" 204 "$STATUS"

sleep 3
echo "── 3. JWT token grants"
sleep 3 # the auth budget is 5 rps / burst 10 — pace the grant calls
GRANT=$(curl -s -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","email":"demo@go-template.local","password":"demo-password-123"}')
ACCESS=$(echo "$GRANT" | jqget access_token); REFRESH=$(echo "$GRANT" | jqget refresh_token)
[ -n "$ACCESS" ] || echo "    grant response: $(echo "$GRANT" | head -c 160)"
[ -n "$ACCESS" ] && ok "password grant issues tokens" || bad "password grant"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/users?limit=1" -H "Authorization: Bearer $ACCESS")
check "access token authenticates" 200 "$STATUS"
GRANT2=$(curl -s -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH\"}")
REFRESH2=$(echo "$GRANT2" | jqget refresh_token)
[ -n "$REFRESH2" ] && [ "$REFRESH2" != "$REFRESH" ] && ok "refresh rotates" || bad "refresh rotation"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH\"}")
check "reuse detection" 401 "$STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH2\"}")
check "family revoked after reuse" 401 "$STATUS"
sleep 2
REVOKE_TOKEN=$(curl -s -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","email":"demo@go-template.local","password":"demo-password-123"}' | jqget refresh_token)
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/token/revoke" -H 'Content-Type: application/json' \
  -d "{\"refresh_token\":\"$REVOKE_TOKEN\"}")
check "revoke idempotent" 204 "$STATUS"

sleep 3
echo "── 4. Org invitations"
sleep 2
INVITEE_EMAIL="walk-invitee-$$_$(date +%s)@example.com"
INVITEE=$(curl -s -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$INVITEE_EMAIL\",\"name\":\"Invitee\",\"password\":\"password123\"}" | jqget token)
[ -n "$INVITEE" ] && ok "invitee registered" || bad "invitee register"
ORG=$(curl -s -X POST "$BASE/api/orgs" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"name\":\"Walkthrough Org $(date +%s)\"}" | jqget id)
[ -n "$ORG" ] || echo "    org create failed"
INV=$(curl -s -X POST "$BASE/api/orgs/$ORG/invitations" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$INVITEE_EMAIL\",\"role\":\"requester\"}")
[ -n "$(echo "$INV" | jqget token)" ] && ok "invitation created (token echoed)" || bad "invitation create: $INV"
ITOKEN=$(echo "$INV" | jqget token)
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/invitations/accept" \
  -H "Authorization: Bearer $INVITEE" -H 'Content-Type: application/json' -d "{\"token\":\"$ITOKEN\"}")
check "invitation accept" 200 "$STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/invitations/accept" \
  -H "Authorization: Bearer $INVITEE" -H 'Content-Type: application/json' -d "{\"token\":\"$ITOKEN\"}")
check "token single-use" 409 "$STATUS"

echo "── 5. Files + ticket attachments"
echo "walkthrough file content" > /tmp/wt-file.txt
UP=$(curl -s -X POST "$BASE/api/files" -H "Authorization: Bearer $ADMIN" -F "file=@/tmp/wt-file.txt")
FILE_ID=$(echo "$UP" | jqget id)
[ -n "$FILE_ID" ] && ok "multipart upload" || bad "upload: $UP"
DOWN=$(curl -s "$BASE/api/files/$FILE_ID/content" -H "Authorization: Bearer $ADMIN")
[ "$DOWN" = "walkthrough file content" ] && ok "download roundtrip" || bad "download: $DOWN"
TT=$(curl -s -X POST "$BASE/api/ticket-types" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"org_id":"","key":"wt-'"$(date +%s)"'","name":"Walkthrough","steps":[{"name":"Review","approver_role":"manager"}]}')
TICKET=$(curl -s -X POST "$BASE/api/tickets" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"type_id":"'"$(echo "$TT" | jqget id)"'","title":"Walkthrough ticket","description":"attachment target"}' | jqget id)
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/files" -H "Authorization: Bearer $ADMIN" \
  -F "file=@/tmp/wt-file.txt" -F "ref_type=ticket" -F "ref_id=$TICKET")
check "ticket attachment upload" 201 "$STATUS"
COUNT=$(curl -s "$BASE/api/tickets/$TICKET/attachments" -H "Authorization: Bearer $ADMIN" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']))")
check "attachment listed" 1 "$COUNT"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/files/$FILE_ID/url" -H "Authorization: Bearer $ADMIN")
check "presign 501 in memory mode" 501 "$STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/files/$FILE_ID" -H "Authorization: Bearer $ADMIN")
check "owner delete" 204 "$STATUS"

echo "── 6. GDPR export (queued job via worker)"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/users/me/export" -H "Authorization: Bearer $ADMIN")
check "export accepted" 202 "$STATUS"
# NOTE: without S3 the blob store is per-process — the worker stores the
# bundle in its own memory, so the api cannot stream it. Multi-process
# deployments must configure APP_S3_* (docs/guides/files.md).
for i in $(seq 1 15); do
  JOB=$(curl -s "$BASE/api/users/me/export" -H "Authorization: Bearer $ADMIN" | jqget status)
  [ "$JOB" = "completed" ] && break
  sleep 2
done
echo "    export job status: $JOB"
DOWNLOAD=$(curl -s -o /tmp/wt-export.zip -w '%{http_code}' "$BASE/api/users/me/export/download" -H "Authorization: Bearer $ADMIN")
if [ "$DOWNLOAD" = "200" ]; then
  python3 -c "import zipfile; z=zipfile.ZipFile('/tmp/wt-export.zip'); names=z.namelist(); assert 'profile.json' in names and 'tickets.json' in names, names" \
    && ok "bundle is a zip with profile+tickets" || bad "bundle contents"
else
  echo "  ⊘ export download skipped (${DOWNLOAD}) — needs APP_S3_* in multi-process mode; job completed: $JOB"
fi

echo "── 7. Search + ETag"
COUNT=$(curl -s "$BASE/api/search?q=demo" -H "Authorization: Bearer $ADMIN" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d['users']) + len(d['tickets']))")
[ "$COUNT" -ge 1 ] && ok "combined search returns matches" || bad "search (matches=$COUNT)"
ETAG=$(curl -s -D - -o /dev/null "$BASE/api/users?limit=3" -H "Authorization: Bearer $ADMIN" | grep -i '^etag' | tr -d '\r' | cut -d' ' -f2)
[ -n "$ETAG" ] && ok "ETag present" || bad "ETag"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/users?limit=3" -H "Authorization: Bearer $ADMIN" -H "If-None-Match: $ETAG")
check "If-None-Match → 304" 304 "$STATUS"

echo "── 8. Maintenance mode + headers"
curl -s -X PUT "$BASE/api/settings/maintenance.enabled" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d '{"value":"true"}' > /dev/null
sleep 6 # the gate memoizes the flag for 5s
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/users" -H "Authorization: Bearer $ADMIN")
check "maintenance 503 on API" 503 "$STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/health/ready")
check "health exempt" 200 "$STATUS"
RA=$(curl -s -D - -o /dev/null "$BASE/api/users" -H "Authorization: Bearer $ADMIN" | grep -i '^retry-after' | tr -d '\r')
[ -n "$RA" ] && ok "Retry-After present" || bad "Retry-After"
curl -s -X PUT "$BASE/api/settings/maintenance.enabled" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d '{"value":"false"}' > /dev/null
sleep 6
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/users" -H "Authorization: Bearer $ADMIN")
check "maintenance off restores API" 200 "$STATUS"

echo "── 9. Per-identity rate limit (default budget: 100 rps / burst 200)"
SEQS=$(seq 1 260 | xargs -P 12 -I{} echo "{}" | tr '\n' ' ')
CODES=$(for i in $SEQS; do curl -s -o /dev/null -w '%{http_code}\n' "$BASE/api/me" -H "Authorization: Bearer $ADMIN" & done; wait) 
echo "$CODES" | grep -q 429 && ok "identity limiter engages (429 seen)" || bad "no 429 observed"
sleep 5

echo "── 10. Tracing, metrics, version"
curl -s "$BASE/metrics" | grep -q "app_http_requests_total" && ok "prometheus metrics" || bad "metrics"
curl -s "$BASE/version" | grep -q version && ok "version endpoint" || bad "version"

echo
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = "0" ]
