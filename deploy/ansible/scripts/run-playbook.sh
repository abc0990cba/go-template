#!/usr/bin/env bash
# Wrapper around ansible-playbook: resolves the working SSH user, then runs the
# requested playbook against the site inventory.
#   run-playbook.sh [playbook.yml] [extra ansible args...]
#   VPS_SITE=dev run-playbook.sh site.yml --check --diff
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

require_setup

PLAYBOOK="${1:-site.yml}"
shift || true

if [ "$PLAYBOOK" != "site.yml" ] && [ "$PLAYBOOK" != "deploy-app.yml" ]; then
  die "Unknown playbook '$PLAYBOOK' (expected site.yml or deploy-app.yml)"
fi

# Refresh the working SSH user if the stored one stopped working (first run
# bootstraps as root, every later run uses the hardened deploy user).
if ! ssh -o BatchMode=yes -o ConnectTimeout=8 "${vps_ssh_user:-root}@${vps_host}" true 2>/dev/null; then
  if NEW_USER=$(detect_ssh_user); then
    info "SSH user '${vps_ssh_user:-root}' no longer works; switching inventory to '${NEW_USER}'"
    persist_ssh_user "$NEW_USER"
  else
    die "Cannot SSH to ${vps_host} as ${vps_ssh_user:-root}, root or deploy. Check: ssh ${vps_host}"
  fi
fi

cd "$ANSIBLE_DIR"
exec ansible-playbook -i "inventories/$VPS_SITE" "$PLAYBOOK" "$@"
