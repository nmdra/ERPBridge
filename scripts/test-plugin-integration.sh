#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="erpbridge-plugin-test"
DEFAULT_PLUGIN_IMAGE="ghcr.io/nmdra/erpbridge-plugins/mock-plugin:0.1.0"

random_hex() {
  od -An -N16 -tx1 /dev/urandom | tr -d ' \n'
}

plugin_api_key="plugin-test-$(random_hex)"
plugin_api_secret="plugin-secret-$(random_hex)"
mock_plugin_api_key="mock-plugin-$(random_hex)"
admin_api_token="admin-$(random_hex)"
export MOCK_ERP_CREDENTIALS_JSON
MOCK_ERP_CREDENTIALS_JSON=$(printf '{"credentials":[{"api_key":"%s","api_secret":"%s","role":"admin","identity":"plugin-test"}]}' "$plugin_api_key" "$plugin_api_secret")
export ERP_PRIMARY_KEY="token ${plugin_api_key}:${plugin_api_secret}"
export MOCK_PLUGIN_API_KEY="$mock_plugin_api_key"
export PLUGIN_MOCK_API_KEY="$mock_plugin_api_key"
export API_AUTH_TOKEN="$admin_api_token"
export PLUGIN_ENDPOINT_ALLOWLIST="mock-plugin:8080"
export INSECURE_AUTH_ALLOWED_HOSTS="mock-erp:8081,mock-plugin:8080"
plugin_image_override="${MOCK_PLUGIN_IMAGE:-}"
export MOCK_PLUGIN_IMAGE="${plugin_image_override:-$DEFAULT_PLUGIN_IMAGE}"
export ERPBRIDGE_HOST_PORT=18080
export MOCK_ERP_HOST_PORT=18081
export REDIS_HOST_PORT=16379
export REDIS_INSIGHT_HOST_PORT=18001

COMPOSE=(docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.plugin-test.yml)

cleanup() {
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cleanup
if [[ -z "$plugin_image_override" ]]; then
  docker build --file ../ERPBridge-Plugins/plugins/mock-plugin/Dockerfile --tag "$MOCK_PLUGIN_IMAGE" ../ERPBridge-Plugins/plugins/mock-plugin
fi

"${COMPOSE[@]}" up -d --build

for attempt in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:18090/health >/dev/null 2>&1 &&
    curl --fail --silent http://127.0.0.1:18080/mcp/health >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" == "60" ]]; then
    echo "plugin integration stack did not become ready" >&2
    "${COMPOSE[@]}" ps
    exit 1
  fi
  sleep 2
done

plugin_process_payload='{"protocolVersion":"v1","invocationId":"black-box","tool":{"name":"fixture","version":"1.0.0"},"result":{}}'
missing_key_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Content-Type: application/json' \
  --data "$plugin_process_payload" http://127.0.0.1:18090/v1/process)
wrong_key_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Content-Type: application/json' --header 'X-API-Key: wrong-plugin-key' \
  --data "$plugin_process_payload" http://127.0.0.1:18090/v1/process)
correct_key_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Content-Type: application/json' --header "X-API-Key: $MOCK_PLUGIN_API_KEY" \
  --data "$plugin_process_payload" http://127.0.0.1:18090/v1/process)
if [[ "$missing_key_status" != "401" || "$wrong_key_status" != "401" || "$correct_key_status" != "200" ]]; then
  echo "plugin API-key black-box checks failed" >&2
  exit 1
fi

ERPBRIDGE_TEST_BASE_URL=http://127.0.0.1:18080 \
  go test -tags pluginintegration ./internal/integration -run TestPluginSystemBlackBox -count=1 -v
