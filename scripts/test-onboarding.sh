#!/usr/bin/env bash
set -Eeuo pipefail

# Run a complete local onboarding check in an isolated Compose project. The
# bootstrap owns ephemeral credentials; this script never reads or prints them.
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
DEV_STACK="$SCRIPT_DIR/dev-stack.sh"
DOCKER_BIN=${DOCKER_BIN:-docker}
CTX=onboarding-test
API_NAME=get-departments
TOOL_NAME=get_departments
PROJECT_NAME="erpbridge-onboarding-$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')"
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/erpbridge-onboarding.XXXXXX")
HOME_DIR="$TMP_DIR/home"
BIN_DIR="$TMP_DIR/bin"
CONFIG_DIR="$HOME_DIR/.bridgectl"
CLI="$BIN_DIR/bridgectl"
SERVER_PORT=${ONBOARDING_SERVER_PORT:-}
MOCK_PORT=${ONBOARDING_MOCK_PORT:-}
STACK_STARTED=false

# Do not allow caller-provided credentials into this disposable fixture.
unset MOCK_ERP_CREDENTIALS_JSON MOCK_ERP_CREDENTIALS_FILE ERP_PRIMARY_KEY API_AUTH_TOKEN BRIDGE_API_TOKEN || true

fail() {
  printf 'onboarding test: %s\n' "$1" >&2
  exit 1
}

pick_port() {
  python3 - <<'PY'
import socket
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
}

if [[ -z "$SERVER_PORT" ]]; then
  SERVER_PORT=$(pick_port)
fi
if [[ -z "$MOCK_PORT" ]]; then
  MOCK_PORT=$(pick_port)
fi

compose=("$DOCKER_BIN" compose -p "$PROJECT_NAME" -f "$PROJECT_ROOT/docker-compose.yml")

cleanup() {
  status=$?
  if [[ "$STACK_STARTED" == true ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>"$TMP_DIR/compose-down.err" || true
  fi
  rm -rf "$TMP_DIR"
  exit "$status"
}
trap cleanup EXIT

run_quiet() {
  local label=$1
  shift
  if ! "$@" >"$TMP_DIR/$label.out" 2>"$TMP_DIR/$label.err"; then
    fail "$label failed"
  fi
}

mkdir -p "$CONFIG_DIR" "$BIN_DIR"
cat >"$CONFIG_DIR/config.yaml" <<EOF
current-context: $CTX
contexts:
  $CTX:
    server: http://127.0.0.1:$SERVER_PORT
    mcp-server: http://127.0.0.1:$SERVER_PORT/mcp/
    erp-base: http://mock-erp:8081
    auth:
      type: api-key
      header: X-API-Key
EOF

# Build only the CLI. The Compose server image is built by dev-stack.sh.
run_quiet build-cli env HOME="$HOME_DIR" go build -o "$CLI" ./tools/bridgectl/main.go

STACK_STARTED=true
run_quiet start-stack env \
  HOME="$HOME_DIR" \
  COMPOSE_PROJECT_NAME="$PROJECT_NAME" \
  ERPBRIDGE_HOST_PORT="$SERVER_PORT" \
  MOCK_ERP_HOST_PORT="$MOCK_PORT" \
  HEALTH_ATTEMPTS=90 \
  HEALTH_INTERVAL=2 \
  "$DEV_STACK"

# Register a read-only endpoint that is reachable from the server network.
run_quiet register env HOME="$HOME_DIR" \
  "$CLI" --context "$CTX" api register \
  --name "$API_NAME" \
  --url 'http://mock-erp:8081/api/resource/Department' \
  --method GET \
  --module onboarding \
  --description 'List departments for the disposable onboarding fixture' \
  --auth-type api-key \
  --auth-header Authorization \
  --credential-ref ERP_PRIMARY_KEY

# Generate one API-derived YAML draft. It is a temporary artifact.
MANIFEST_FILE="$TMP_DIR/generated.yaml"

run_quiet api-test env HOME="$HOME_DIR" \
  "$CLI" --context "$CTX" -o json api test "$API_NAME"
grep -Eq '"isSuccess"[[:space:]]*:[[:space:]]*true' "$TMP_DIR/api-test.out" || fail 'server-side API probe did not succeed'
for field in api status code latency contentType isSuccess; do
  grep -Eq '"'"$field"'"[[:space:]]*:' "$TMP_DIR/api-test.out" || fail "probe output omitted $field"
done
if grep -Eiq '"(url|authType|authHeader|credentialRef)"[[:space:]]*:' "$TMP_DIR/api-test.out" ||
  grep -Fq 'mock-erp:8081' "$TMP_DIR/api-test.out"; then
  fail 'server-side API probe exposed endpoint metadata'
fi

if ! env HOME="$HOME_DIR" "$CLI" --context "$CTX" -o json api register \
  --name "$API_NAME" \
  --url 'http://mock-erp:8081/api/resource/Department' \
  --method GET \
  --module onboarding \
  --description 'Duplicate onboarding fixture' \
  --auth-type api-key \
  --auth-header Authorization \
  --credential-ref ERP_PRIMARY_KEY \
  >"$TMP_DIR/duplicate.out" 2>"$TMP_DIR/duplicate.err"; then
  grep -q 'REGISTRY_CONFLICT' "$TMP_DIR/duplicate.out" || fail 'duplicate API did not return REGISTRY_CONFLICT'
else
  fail 'duplicate API registration unexpectedly succeeded'
fi

if ! env HOME="$HOME_DIR" BRIDGE_MCP_SERVER="http://127.0.0.1:$SERVER_PORT/mcp/wrong" \
  "$CLI" --context "$CTX" tool get \
  >"$TMP_DIR/invalid-root.out" 2>"$TMP_DIR/invalid-root.err"; then
  grep -q 'CONTROL_PLANE_URL_INVALID' "$TMP_DIR/invalid-root.err" || fail 'invalid control-plane root did not return its stable code'
else
  fail 'invalid control-plane root unexpectedly succeeded'
fi

env HOME="$HOME_DIR" "$CLI" --context "$CTX" -o yaml tool generate \
  --api "$API_NAME" \
  >"$MANIFEST_FILE" 2>"$TMP_DIR/generate.err" || fail 'tool generation failed'

env HOME="$HOME_DIR" "$CLI" --context "$CTX" tool validate -f "$MANIFEST_FILE" \
  >"$TMP_DIR/validate.out" 2>"$TMP_DIR/validate.err" || fail 'generated manifest validation failed'

run_quiet apply env HOME="$HOME_DIR" \
  "$CLI" --context "$CTX" tool apply -f "$MANIFEST_FILE"
run_quiet get-tool env HOME="$HOME_DIR" \
  "$CLI" --context "$CTX" -o json tool get "$TOOL_NAME"
grep -q "$TOOL_NAME" "$TMP_DIR/get-tool.out" || fail 'applied tool was not visible in discovery'

# Exercise MCP initialize, discovery, and call. Response bodies stay in the
# private temporary directory and are inspected only for fixed success markers.
MCP_URL="http://127.0.0.1:$SERVER_PORT/mcp/"
MCP_HEADERS="$TMP_DIR/mcp-headers"
MCP_INITIALIZE="$TMP_DIR/mcp-initialize"
MCP_LIST="$TMP_DIR/mcp-list"
MCP_CALL="$TMP_DIR/mcp-call"

curl --fail --silent --show-error --max-time 10 -D "$MCP_HEADERS" -o "$MCP_INITIALIZE" \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"onboarding-check","version":"1.0"}}}' \
  "$MCP_URL" >"$TMP_DIR/mcp-initialize-curl.out" 2>"$TMP_DIR/mcp-initialize-curl.err" || fail 'MCP initialize failed'
SESSION_ID=$(awk '/^[Mm][Cc][Pp]-[Ss]ession-[Ii][Dd]:/ { sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]*\r?$/, ""); print; exit }' "$MCP_HEADERS")
[[ -n "$SESSION_ID" ]] || fail 'MCP initialize did not return a session'

curl --fail --silent --show-error --max-time 10 -o "$MCP_LIST" \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Content-Type: application/json' \
  -H "Mcp-Session-Id: $SESSION_ID" \
  --data '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  "$MCP_URL" >"$TMP_DIR/mcp-list-curl.out" 2>"$TMP_DIR/mcp-list-curl.err" || fail 'MCP tools/list failed'
grep -q "$TOOL_NAME" "$MCP_LIST" || fail 'MCP discovery did not include the applied tool'

curl --fail --silent --show-error --max-time 15 -o "$MCP_CALL" \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Content-Type: application/json' \
  -H "Mcp-Session-Id: $SESSION_ID" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"$TOOL_NAME\",\"arguments\":{\"page\":1}}}" \
  "$MCP_URL" >"$TMP_DIR/mcp-call-curl.out" 2>"$TMP_DIR/mcp-call-curl.err" || fail 'MCP tools/call failed'
if grep -q '"isError":true' "$MCP_CALL"; then
  fail 'MCP tools/call returned an execution error'
fi

echo 'onboarding test: bootstrap, server-side probe, generation, validation, apply, MCP discovery/call, and recovery branches passed'
