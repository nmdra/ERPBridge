#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="erpbridge-plugin-test"
COMPOSE=(docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.plugin-test.yml)

cleanup() {
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cleanup
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

ERPBRIDGE_TEST_BASE_URL=http://127.0.0.1:18080 \
  go test -tags pluginintegration ./internal/integration -run TestPluginSystemBlackBox -count=1 -v
