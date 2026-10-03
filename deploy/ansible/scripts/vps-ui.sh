#!/usr/bin/env bash
# `make vps-ui` — open SSH tunnels to the loopback-only admin surfaces:
#   Nomad UI  -> http://127.0.0.1:4646
#   Grafana   -> http://127.0.0.1:3001
# Keep the terminal open while browsing. Ctrl-C closes the tunnels.
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

require_setup

USER_SSH="$(detect_ssh_user)" || die "Cannot SSH to ${vps_host}"
GRAFANA_DOMAIN="$(get_var grafana_domain)"

echo
echo "Tunnels are up while this command runs (Ctrl-C to close):"
echo "  Nomad UI  http://127.0.0.1:4646   (token: sudo cat /etc/nomad.d/tokens/bootstrap.token on the VPS)"
if [ -n "${grafana_domain:-}" ]; then
  echo "  Grafana   https://${grafana_domain}   (or http://127.0.0.1:3001; login admin)"
else
  echo "  Grafana   http://127.0.0.1:3001   (login admin)"
fi
echo
exec ssh -N \
  -L 4646:127.0.0.1:4646 \
  -L 3001:127.0.0.1:3000 \
  -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
  "$USER_SSH@${vps_host}"
