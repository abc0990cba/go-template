#!/usr/bin/env bash
# Run a command on the VPS as root. Used by the make vps-status/logs/ui/...
# helpers. Example: VPS_SITE=prod scripts/vps-remote.sh 'nomad-admin node status'
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

require_setup
[ $# -ge 1 ] || die "usage: vps-remote.sh '<command>'"

exec vps_remote "$1"
