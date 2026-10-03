#!/usr/bin/env bash
# `make vps-rollback` — revert the app to the previous successful Nomad
# deployment (no image rebuild; Nomad flips job versions in seconds).
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

require_setup

REMOTE='
set -euo pipefail
status=$(/usr/local/bin/nomad-admin job status go-template)
versions=$(printf "%s\n" "$status" \
  | awk -F"|" "/== Deployments/{f=1;next} f&&/successful/{gsub(/ /,\"\",\$2); print \$2}")
prior=$(printf "%s\n" "$versions" | sed -n 2p)
if [ -z "$prior" ]; then
  echo "no previous successful deployment to revert to" >&2
  exit 1
fi
echo "reverting go-template to job version $prior ..."
/usr/local/bin/nomad-admin job revert go-template "$prior" >/dev/null
sleep 3
printf "%s\n" "$status" | sed -n "/== Deployments/,/== Allocations/p"
echo "current deployment status:"
/usr/local/bin/nomad-admin job status go-template | grep -A3 "^== Latest Deployment"
'

info "Rolling back the app on ${vps_host} ..."
vps_remote "$REMOTE"
ok "Rollback submitted. Verify: make vps-status (or the Nomad UI via make vps-ui)"
