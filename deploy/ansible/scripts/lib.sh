#!/usr/bin/env bash
# Shared helpers for the vps-* scripts. Sourced, never executed directly.
# Every script works per-site: VPS_SITE=dev make vps-deploy targets the dev VPS.

set -euo pipefail

ANSIBLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="$ANSIBLE_DIR/.vps-state"
VPS_SITE="${VPS_SITE:-prod}"
STATE_FILE="$STATE_DIR/$VPS_SITE.env"

# Pin every ansible invocation (from any CWD) to this repo's config and vault
# password, so `make vps-*` and manual ansible runs behave identically.
export ANSIBLE_CONFIG="$ANSIBLE_DIR/ansible.cfg"
export ANSIBLE_VAULT_PASSWORD_FILE="$ANSIBLE_DIR/.vault-pass"

C_G="\033[0;32m"; C_R="\033[0;31m"; C_Y="\033[0;33m"; C_B="\033[1;34m"; C_N="\033[0m"
info()  { printf "${C_B}==> ${C_N}%s\n" "$*"; }
ok()    { printf "${C_G} ✓ ${C_N}%s\n" "$*"; }
warn()  { printf "${C_Y} ⚠ ${C_N}%s\n" "$*"; }
fail()  { printf "${C_R} ✗ ${C_N}%s\n" "$*" >&2; }
die()   { fail "$*"; exit 1; }

# Prompt: ask "label" "default" -> echoes the answer.
ask() {
  local __label="$1" __default="${2:-}" __answer=""
  if [ -n "$__default" ]; then
    printf "${C_B}? ${C_N}%s ${C_Y}[%s]${C_N}: " "$__label" "$__default" >&2
  else
    printf "${C_B}? ${C_N}%s: " "$__label" >&2
  fi
  read -r __answer
  echo "${__answer:-$__default}"
}

# Ask yes/no; returns 0 for yes.
ask_yn() {
  local __answer=""
  printf "${C_B}? ${C_N}%s ${C_Y}[y/n]${C_N}: " "$1" >&2
  read -r __answer
  [ "${__answer:-n}" = "y" ] || [ "${__answer:-n}" = "Y" ]
}

# Read a scalar from a group_vars YAML file without sourcing YAML in bash:
# get_var app_domain [file]. Strips quotes and trailing # comments.
get_var() {
  local __key="$1" __file="${2:-$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/site.yml}" __v=""
  local __sed="s/^${__key}:[[:space:]]*//; s/[[:space:]]*#.*$//; s/^\"//; s/\"$//"
  __v="$(grep -E "^${__key}:" "$__file" 2>/dev/null | head -1 | sed -E "$__sed")"
  # Fall back to the committed defaults file when the site file has no such key.
  if [ -z "$__v" ] && [ "$__file" != "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/main.yml" ]; then
    __v="$(grep -E "^${__key}:" "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/main.yml" 2>/dev/null | head -1 | sed -E "$__sed")"
  fi
  echo "$__v"
}

require_setup() {
  [ -f "$STATE_FILE" ] || die "Site '$VPS_SITE' is not set up yet. Run: make vps-setup"
  [ -f "$ANSIBLE_DIR/inventories/$VPS_SITE/group_vars/vps/secrets.yml" ] \
    || die "secrets.yml missing for site '$VPS_SITE'. Run: make vps-setup"
  # shellcheck disable=SC1090
  source "$STATE_FILE"
  [ -n "${vps_host:-}" ] || die "Empty vps_host in $STATE_FILE"
}

# SSH options for every connection: batch mode, short timeouts, and — when the
# site state defines one — the site's dedicated key (the local OrbStack
# rehearsal uses .vps-state/local_key). Sets the global SSH_OPTS ARRAY: repo
# paths may contain spaces (e.g. "/Volumes/ADATA SC750/..."), so callers must
# expand as ssh "${SSH_OPTS[@]}" — never as an unquoted string.
build_ssh_opts() {
  SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new)
  if [ -n "${vps_ssh_key:-}" ]; then
    SSH_OPTS+=(-i "$vps_ssh_key" -o IdentitiesOnly=yes)
  fi
}

# Figure out which SSH identity works right now (bootstrap root vs hardened
# deploy user) and remember it in .vps-state. Echoes the working user.
detect_ssh_user() {
  local candidates=("${vps_ssh_user:-}" root deploy)
  local user attempt
  build_ssh_opts
  for user in "${candidates[@]}"; do
    [ -n "$user" ] || continue
    if attempt=$(ssh "${SSH_OPTS[@]}" "$user@${vps_host}" true 2>/dev/null); then
      echo "$user"
      return 0
    fi
  done
  return 1
}

# Persist the working user into the state file and the inventory hosts.yml so
# plain `ansible-playbook` runs (and humans reading the repo) see the truth.
persist_ssh_user() {
  local user="$1"
  sed -i.bak "s/^          ansible_user: .*/          ansible_user: ${user}/" \
    "$ANSIBLE_DIR/inventories/$VPS_SITE/hosts.yml"
  rm -f "$ANSIBLE_DIR/inventories/$VPS_SITE/hosts.yml.bak"
  if grep -q '^vps_ssh_user=' "$STATE_FILE" 2>/dev/null; then
    sed -i.bak "s/^vps_ssh_user=.*/vps_ssh_user=${user}/" "$STATE_FILE"
    rm -f "$STATE_FILE.bak"
  else
    echo "vps_ssh_user=\"${user}\"" >> "$STATE_FILE"
  fi
}

# Run a command on the VPS as root: vps_remote 'nomad-admin job status'
vps_remote() {
  local user
  user="$(detect_ssh_user)" || die "Cannot SSH to ${vps_host}. Check your key: ssh ${vps_host}"
  build_ssh_opts
  ssh "${SSH_OPTS[@]}" "$user@${vps_host}" \
    "sudo -n bash -c $(printf '%q' "$1")"
}
