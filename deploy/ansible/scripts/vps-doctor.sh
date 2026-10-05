#!/usr/bin/env bash
# `make vps-doctor` — pre-flight check: workstation prerequisites, SSH
# reachability, DNS A-records, and VPS sizing. Read-only; fixes nothing.
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

FAILURES=0
check() { # check "label" command...
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$label"; else fail "$label"; FAILURES=$((FAILURES+1)); fi
}
warn_only() { # like check but never increments failures
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$label"; else warn "$label"; fi
}

info "Doctor for site '${VPS_SITE}'"
echo "--- workstation ---"
check "ansible-playbook installed"            bash -c "command -v ansible-playbook"
check "openssl installed"                     bash -c "command -v openssl"
check "site configured (make vps-setup)"      test -f "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/secrets.yml"
if [ -f "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/secrets.yml" ]; then
  check "vault password works"                ansible-vault view "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/secrets.yml"
fi
if [ -f "$STATE_FILE" ]; then
  # shellcheck disable=SC1090
  source "$STATE_FILE"
else
  fail "no state file — run: make vps-setup (or make vps-local for the local rehearsal)"
  exit 1
fi
check "inventory has a real VPS IP"           bash -c "[ '${vps_host}' != REPLACE_WITH_VPS_IP ] && [ -n '${vps_host}' ]"

if [ "${vps_kind:-vps}" = "orbstack" ]; then
  echo "--- local OrbStack machine ---"
  check "OrbStack CLI installed"              bash -c "command -v orb"
  check "machine exists"                      bash -c "orb list | awk '{print \$1}' | grep -qx \"\$(basename '${vps_host}' .orb.local)\""
  check "SSH reachable"                       bash -c "ssh -i '${vps_ssh_key:-}' -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=5 '${vps_ssh_user}@${vps_host}' true"
  check "dockerd inside the machine"          bash -c "ssh -i '${vps_ssh_key:-}' -o IdentitiesOnly=yes -o BatchMode=yes '${vps_ssh_user}@${vps_host}' 'sudo docker version --format {{.Server.Version}}'"
  echo
  APP_DOMAIN=$(get_var app_domain)
  CI_DOMAIN=$(get_var ci_domain)
  GRAF_DOMAIN=$(get_var grafana_domain)
  echo "Browser access (OrbStack forwards machine ports to localhost) — add to /etc/hosts if you want names:"
  echo "  127.0.0.1 ${APP_DOMAIN} ${CI_DOMAIN} ${GRAF_DOMAIN}"
  if [ "$FAILURES" -eq 0 ]; then
    ok "Local rehearsal looks good. Run: make vps-local (first time) or VPS_SITE=local make vps-deploy"
  else
    die "$FAILURES blocking check(s) failed"
  fi
  exit 0
fi

echo "--- network ---"
WORKING_USER=$(detect_ssh_user) && ok "SSH reachable (user: ${WORKING_USER})" || {
  fail "SSH unreachable as root/deploy — check the VPS IP, your key, and provider firewall (port 22)"
  FAILURES=$((FAILURES+1))
}

# shellcheck disable=SC1091
APP_DOMAIN=$(get_var app_domain)
CI_DOMAIN=$(get_var ci_domain)
GRAF_DOMAIN=$(get_var grafana_domain)

dns_lookup() { # first usable A record for a name; dig -> nslookup -> python3
  if command -v dig >/dev/null 2>&1; then
    dig +short "$1" A 2>/dev/null | grep -v '^$' | head -1
  elif command -v nslookup >/dev/null 2>&1; then
    nslookup -type=A "$1" 2>/dev/null | awk '/^Address/ && !/53/ {print $2; exit}'
  else
    python3 - "$1" <<'PY' 2>/dev/null
import socket, sys
try:
    print(socket.gethostbyname(sys.argv[1]))
except Exception:
    pass
PY
  fi
}
for d in "$APP_DOMAIN" "$CI_DOMAIN" "$GRAF_DOMAIN"; do
  [ -n "$d" ] || continue
  resolved=$(dns_lookup "$d" || true)
  if [ "$resolved" = "$vps_host" ]; then
    ok "DNS $d -> $resolved"
  elif [ -z "$resolved" ]; then
    warn "DNS $d has no A record yet (Let's Encrypt will fail until it points at $vps_host)"
  else
    warn "DNS $d -> $resolved (expected $vps_host)"
  fi
done

echo "--- VPS (via SSH) ---"
if [ -n "${WORKING_USER:-}" ]; then
  remote() { ssh -o BatchMode=yes -o ConnectTimeout=8 "$WORKING_USER@${vps_host}" "sudo -n bash -c $(printf '%q' "$1")" 2>/dev/null; }
  warn_only "Ubuntu 22.04/24.04" remote 'grep -qE "VERSION_ID=(22.04|24.04)" /etc/os-release'
  cpus=$(remote 'nproc' || echo 0)
  [ "${cpus:-0}" -ge 2 ] 2>/dev/null && ok ">= 2 CPU cores" || warn "< 2 CPU cores (builds will be slow)"
  ram=$(remote 'free -gm | grep -m1 Mem | tr -s " " | cut -d" " -f2' || echo 0)
  [ "${ram:-0}" -ge 3 ] 2>/dev/null && ok ">= 3 GB RAM" || warn "< 3 GB RAM — disable CI and/or monitoring (docs/vps/deployment.md#sizing)"
  disk=$(remote 'df -BG / | grep -m1 / | tr -s " " | cut -d" " -f4 | tr -d G' || echo 0)
  [ "${disk:-0}" -ge 15 ] 2>/dev/null && ok ">= 15 GB free disk" || warn "< 15 GB free disk (images + backups + TSDB need room)"
  remote 'ss -tlnp | grep -qE ":80 |:443 "' && warn "ports 80/443 already have listeners (existing web server?)" || ok "ports 80/443 are free"
  remote 'command -v docker' >/dev/null 2>&1 && warn "docker already installed (playbook will keep it)" || ok "no pre-existing docker (clean slate)"
else
  warn "skipping VPS checks (SSH unreachable)"
fi

echo
if [ "$FAILURES" -eq 0 ]; then
  ok "All blocking checks passed. Run: make vps-deploy"
else
  die "$FAILURES blocking check(s) failed — fix them, then re-run make vps-doctor"
fi
