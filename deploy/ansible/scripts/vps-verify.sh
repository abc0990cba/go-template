#!/usr/bin/env bash
# `make vps-verify` — post-deploy verification table (works for every site:
# prod, dev, local). Checks Nomad jobs over SSH and the public HTTPS surface
# with curl --resolve, so it validates the actual server even when DNS/hosts
# entries are missing. Exits non-zero when anything fails.
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

require_setup

APP_DOMAIN=$(get_var app_domain)
CI_DOMAIN=$(get_var ci_domain)
GRAF_DOMAIN=$(get_var grafana_domain)
TLS_MODE=$(get_var tls_mode)
CI_ENABLED=$(get_var ci_enabled)
MONITORING_ENABLED=$(get_var monitoring_enabled)

# curl flags: internal CA and LE-staging certs are untrusted on purpose.
CURL_K="-sS"
if [ "$TLS_MODE" = "internal" ] || [ "$(get_var acme_staging)" = "true" ]; then
  CURL_K="$CURL_K -k"
fi

# curl --resolve target: the VPS IP itself (prod) — or, for the local OrbStack
# machine, 127.0.0.1 via OrbStack's automatic port-forwarding to localhost.
TARGET="$vps_host"
if [ "${vps_kind:-vps}" = "orbstack" ]; then
  TARGET="127.0.0.1"
fi

info "Verifying site '${VPS_SITE}' (${vps_host})"
PASS=0; FAIL=0
row() { # row "label" cmd...
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then printf "  \033[0;32mPASS\033[0m  %s\n" "$label"; PASS=$((PASS+1));
  else printf "  \033[0;31mFAIL\033[0m  %s\n" "$label"; FAIL=$((FAIL+1)); fi
}
http_code() { # echo the https status for domain+path via --resolve
  local domain="$1" path="$2"
  curl $CURL_K -o /dev/null -w '%{http_code}' --max-time 10 \
    --resolve "${domain}:443:${TARGET}" "https://${domain}${path}"
}
remote_ok() { # run a check on the VPS as root
  vps_remote "$1"
}

# --- SSH + nomad jobs (single SSH round-trip) -----------------------------------
USER_SSH=$(detect_ssh_user) || true
if [ -n "${USER_SSH:-}" ]; then
  printf "  \033[0;32mPASS\033[0m  SSH reachable (user: %s)\n" "$USER_SSH"; PASS=$((PASS+1))
else
  printf "  \033[0;31mFAIL\033[0m  SSH reachable\n"; FAIL=$((FAIL+1))
fi

JOBS="postgres redis registry go-template caddy"
[ "$CI_ENABLED" = "true" ] && JOBS="$JOBS woodpecker"
[ "$MONITORING_ENABLED" = "true" ] && JOBS="$JOBS monitoring"
if [ -n "${USER_SSH:-}" ]; then
  # One SSH round-trip: "<job>=<Status>" per line (running inside root shell,
  # so nomad-admin's prewired TLS+token env applies directly).
  RESULTS=$(vps_remote 'for j in '"$JOBS"'; do st=$(/usr/local/bin/nomad-admin job status "$j" 2>/dev/null | grep -m1 "^Status *=" | tr -d " "); echo "${j}=${st#Status=}"; done' 2>/dev/null || true)
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    j=${line%%=*}
    st=${line#*=}
    if [ "$st" = "running" ]; then
      printf "  \033[0;32mPASS\033[0m  nomad job %s running\n" "$j"; PASS=$((PASS+1))
    else
      printf "  \033[0;31mFAIL\033[0m  nomad job %s (status: %s)\n" "$j" "${st:-unknown}"; FAIL=$((FAIL+1))
    fi
  done <<< "$RESULTS"
fi

# --- public HTTPS surface ---------------------------------------------------------
code=$(http_code "$APP_DOMAIN" "/health/ready" || echo 000)
if [ "$code" = "200" ]; then printf "  \033[0;32mPASS\033[0m  https://%s/health/ready -> 200\n" "$APP_DOMAIN"; PASS=$((PASS+1));
else printf "  \033[0;31mFAIL\033[0m  https://%s/health/ready -> %s\n" "$APP_DOMAIN" "$code"; FAIL=$((FAIL+1)); fi

code=$(http_code "$APP_DOMAIN" "/version" || echo 000)
if [ "$code" = "200" ]; then printf "  \033[0;32mPASS\033[0m  https://%s/version -> 200\n" "$APP_DOMAIN"; PASS=$((PASS+1));
else printf "  \033[0;31mFAIL\033[0m  https://%s/version -> %s\n" "$APP_DOMAIN" "$code"; FAIL=$((FAIL+1)); fi

code=$(http_code "$APP_DOMAIN" "/metrics" || echo 000)
if [ "$code" = "403" ]; then printf "  \033[0;32mPASS\033[0m  https://%s/metrics blocked at the edge (403)\n" "$APP_DOMAIN"; PASS=$((PASS+1));
else printf "  \033[0;31mFAIL\033[0m  https://%s/metrics -> %s (expected 403)\n" "$APP_DOMAIN" "$code"; FAIL=$((FAIL+1)); fi

if [ -n "$CI_DOMAIN" ] && [ "$CI_ENABLED" = "true" ]; then
  code=$(http_code "$CI_DOMAIN" "/healthz" || echo 000)
  if [ "$code" = "200" ]; then printf "  \033[0;32mPASS\033[0m  https://%s/healthz -> 200\n" "$CI_DOMAIN"; PASS=$((PASS+1));
  else printf "  \033[0;31mFAIL\033[0m  https://%s/healthz -> %s\n" "$CI_DOMAIN" "$code"; FAIL=$((FAIL+1)); fi
fi

if [ -n "$GRAF_DOMAIN" ] && [ "$MONITORING_ENABLED" = "true" ]; then
  code=$(http_code "$GRAF_DOMAIN" "/api/health" || echo 000)
  if [ "$code" = "200" ]; then printf "  \033[0;32mPASS\033[0m  https://%s/api/health -> 200\n" "$GRAF_DOMAIN"; PASS=$((PASS+1));
  else printf "  \033[0;31mFAIL\033[0m  https://%s/api/health -> %s\n" "$GRAF_DOMAIN" "$code"; FAIL=$((FAIL+1)); fi
fi

echo
if [ "$FAIL" -eq 0 ]; then
  printf "\033[0;32mALL %s CHECKS PASSED\033[0m\n" "$PASS"
  exit 0
fi
printf "\033[0;31m%s of %s checks failed\033[0m — see docs/deployment/vps/runbook.md#troubleshooting\n" "$FAIL" "$((PASS+FAIL))"
exit 1
