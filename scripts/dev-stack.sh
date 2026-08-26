#!/usr/bin/env bash
set -Eeuo pipefail

# Start the local Compose stack without writing generated credentials to disk.
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
cd "$PROJECT_ROOT"

DOCKER_BIN=${DOCKER_BIN:-docker}
COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-erpbridge-dev}
MOCK_ERP_HOST_PORT=${MOCK_ERP_HOST_PORT:-8081}
ERPBRIDGE_HOST_PORT=${ERPBRIDGE_HOST_PORT:-8080}
HEALTH_ATTEMPTS=${HEALTH_ATTEMPTS:-60}
HEALTH_INTERVAL=${HEALTH_INTERVAL:-2}
DRY_RUN=false

usage() {
  cat <<'EOF'
Usage: scripts/dev-stack.sh [--dry-run]

Start the local ERPBridge Compose stack with ephemeral MockERP credentials when
no credential source is supplied. Generated credentials stay in this process
and are never written to .env or printed.
EOF
}

fail() {
  printf 'dev stack: %s\n' "$1" >&2
  exit 1
}

numeric_setting() {
  local name=$1 value=$2
  case "$value" in
    ''|*[!0-9]*) fail "$name must be a non-negative integer" ;;
  esac
}

while (($# > 0)); do
  case "$1" in
    --dry-run) DRY_RUN=true ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
  shift
done

numeric_setting MOCK_ERP_HOST_PORT "$MOCK_ERP_HOST_PORT"
numeric_setting ERPBRIDGE_HOST_PORT "$ERPBRIDGE_HOST_PORT"
numeric_setting HEALTH_ATTEMPTS "$HEALTH_ATTEMPTS"
numeric_setting HEALTH_INTERVAL "$HEALTH_INTERVAL"

random_hex() {
  od -An -N16 -tx1 /dev/urandom | tr -d ' \n'
}

credential_source=provided
if [[ -z "${MOCK_ERP_CREDENTIALS_JSON:-}" && -z "${MOCK_ERP_CREDENTIALS_FILE:-}" ]]; then
  api_key="erpbridge-dev-$(random_hex)"
  api_secret="erpbridge-dev-secret-$(random_hex)"
  export MOCK_ERP_CREDENTIALS_JSON
  MOCK_ERP_CREDENTIALS_JSON=$(printf '{"credentials":[{"api_key":"%s","api_secret":"%s","role":"admin","identity":"dev-bootstrap"}]}' "$api_key" "$api_secret")
  export ERP_PRIMARY_KEY="token ${api_key}:${api_secret}"
  credential_source=generated
fi

COMPOSE=("$DOCKER_BIN" compose -p "$COMPOSE_PROJECT_NAME" -f docker-compose.yml)

# Compose can report interpolation errors containing rendered values. Discard
# its diagnostics so a malformed local environment cannot disclose a secret.
config_error=$(mktemp)
# shellcheck disable=SC2329 # invoked by the EXIT/INT/TERM trap below
cleanup() {
  rm -f "$config_error"
}
trap cleanup EXIT INT TERM
if ! "${COMPOSE[@]}" config --quiet >/dev/null 2>"$config_error"; then
  fail "docker compose config --quiet failed; check the Compose variables and credential source"
fi

if [[ "$DRY_RUN" == true ]]; then
  if [[ "$credential_source" == generated ]]; then
    printf 'dev stack: Compose configuration is valid (ephemeral credentials generated)\n'
  else
    printf 'dev stack: Compose configuration is valid (caller-provided credentials preserved)\n'
  fi
  printf 'dev stack: dry-run; would run docker compose up --build --force-recreate -d\n'
  printf 'dev stack: would poll http://127.0.0.1:%s/health and http://127.0.0.1:%s/mcp/health\n' "$MOCK_ERP_HOST_PORT" "$ERPBRIDGE_HOST_PORT"
  exit 0
fi

"${COMPOSE[@]}" up --build --force-recreate -d

mock_health_url="http://127.0.0.1:${MOCK_ERP_HOST_PORT}/health"
server_health_url="http://127.0.0.1:${ERPBRIDGE_HOST_PORT}/mcp/health"
for ((attempt = 1; attempt <= HEALTH_ATTEMPTS; attempt++)); do
  if curl --fail --silent --show-error --output /dev/null --connect-timeout 2 --max-time 5 "$mock_health_url" 2>/dev/null &&
    curl --fail --silent --show-error --output /dev/null --connect-timeout 2 --max-time 5 "$server_health_url" 2>/dev/null; then
    printf 'dev stack: MockERP and ERPBridge are healthy\n'
    exit 0
  fi
  if ((attempt < HEALTH_ATTEMPTS)); then
    sleep "$HEALTH_INTERVAL"
  fi
done

printf 'dev stack: services did not become healthy\n' >&2
"${COMPOSE[@]}" ps >&2 || true
exit 1
