#!/usr/bin/env bash
# `make vps-restore FILE=...` — restore a pg_dump archive produced by the
# nightly backup into the app database, then redeploy the app job.
# Destructive for the target database by design (that is what restore means);
# runbook: docs/vps/runbook.md#restore-a-backup
set -euo pipefail
# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

FILE="${1:-}"
DB="${2:-go_template}"
[ -n "$FILE" ] || die "usage: vps-restore.sh <path/to/go_template-YYYYMMDD-HHMMSS.dump> [db]"
[ -f "$FILE" ] || die "local file not found: $FILE"
require_setup

USER_SSH="$(detect_ssh_user)" || die "Cannot SSH to ${vps_host}"
BASENAME="$(basename "$FILE")"

info "Uploading ${BASENAME} ..."
build_ssh_opts
scp "${SSH_OPTS[@]}" "$FILE" "$USER_SSH@${vps_host}:/tmp/"

REMOTE="
set -euo pipefail
echo '==> stopping the app job'
/usr/local/bin/nomad-admin job stop go-template >/dev/null || true
sleep 3
container=\$(docker ps -q --filter name=postgres | head -1)
[ -n \"\$container\" ] || { echo 'postgres container not found' >&2; exit 1; }
export PGPASSWORD=\$(cat /opt/go-template/secrets/pg_password)
echo '==> recreating database ${DB}'
docker exec -e PGPASSWORD=\"\$PGPASSWORD\" \"\$container\" psql -U go_template -d postgres \
  -c \"DROP DATABASE IF EXISTS ${DB} WITH (FORCE)\" \
  -c \"CREATE DATABASE ${DB} OWNER go_template\"
echo '==> restoring from ${BASENAME}'
docker run --rm --network host \
  -v /tmp/${BASENAME}:/restore.dump:ro \
  -e PGPASSWORD=\"\$PGPASSWORD\" \
  postgres:16.9 \
  pg_restore -h 127.0.0.1 -U go_template -d ${DB} --no-owner /restore.dump
rm -f /tmp/${BASENAME}
echo '==> restarting the app job'
eval \"\$(cat /opt/go-template/jobs/app.vars)\"
echo '==> waiting for /health/ready'
for i in \$(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then
    echo 'restore complete: app healthy'
    exit 0
  fi
  sleep 10
done
echo 'app did not become healthy after restore — check nomad-admin job status go-template' >&2
exit 1
"

info "Restoring ${DB} on ${vps_host} from ${BASENAME} ..."
vps_remote "$REMOTE"
ok "Restore finished. Verify the app: https://${app_domain:-<app-domain>}/"
