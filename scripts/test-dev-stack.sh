#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
DEV_STACK="$SCRIPT_DIR/dev-stack.sh"
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

FAKE_DOCKER="$TEST_DIR/docker"
LOG="$TEST_DIR/compose.log"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'args=%s\n' "$*" >>"$FAKE_LOG"
case "${EXPECT_MODE:-}" in
  generated)
    [[ "${MOCK_ERP_CREDENTIALS_JSON:-}" == '{"credentials":[{"api_key":"erpbridge-dev-'* ]] || exit 1
    [[ "${ERP_PRIMARY_KEY:-}" == 'token erpbridge-dev-'* ]] || exit 1
    [[ -z "${MOCK_ERP_CREDENTIALS_FILE:-}" ]] || exit 1
    ;;
  provided)
    [[ "${MOCK_ERP_CREDENTIALS_JSON:-}" == "${EXPECTED_JSON:?}" ]] || exit 1
    [[ "${ERP_PRIMARY_KEY:-}" == "${EXPECTED_PRIMARY:?}" ]] || exit 1
    ;;
  file)
    [[ -z "${MOCK_ERP_CREDENTIALS_JSON:-}" ]] || exit 1
    [[ "${MOCK_ERP_CREDENTIALS_FILE:-}" == /run/secrets/mockerp.json ]] || exit 1
    [[ "${ERP_PRIMARY_KEY:-}" == token-from-caller ]] || exit 1
    ;;
  *)
    printf 'missing fake-compose expectation\n' >&2
    exit 1
    ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

run_dry_run() {
  FAKE_LOG="$LOG" DOCKER_BIN="$FAKE_DOCKER" EXPECT_MODE=generated \
    "$DEV_STACK" --dry-run >/dev/null
}

# Generated credentials reach Compose, but the dry-run output and test log do
# not contain the generated JSON or derived primary key.
run_dry_run
if ! grep -Fq 'args=compose -p erpbridge-dev -f docker-compose.yml config --quiet' "$LOG"; then
  printf 'expected Compose config validation\n' >&2
  exit 1
fi
if grep -Fq ' up --build --force-recreate -d' "$LOG"; then
  printf 'dry-run must not start Compose\n' >&2
  exit 1
fi

# Values containing spaces and shell metacharacters are inherited unchanged;
# the bootstrap must not source or rewrite caller-provided values.
quoted_json='{"credentials":[{"api_key":"quoted key","api_secret":"quoted secret","role":"admin"}]}'
quoted_primary='token quoted key:quoted secret'
: >"$LOG"
FAKE_LOG="$LOG" DOCKER_BIN="$FAKE_DOCKER" EXPECT_MODE=provided \
  EXPECTED_JSON="$quoted_json" EXPECTED_PRIMARY="$quoted_primary" \
  MOCK_ERP_CREDENTIALS_JSON="$quoted_json" ERP_PRIMARY_KEY="$quoted_primary" \
  "$DEV_STACK" --dry-run >/dev/null

# A credential file is a supported source and must prevent generated JSON.
: >"$LOG"
FAKE_LOG="$LOG" DOCKER_BIN="$FAKE_DOCKER" EXPECT_MODE=file \
  MOCK_ERP_CREDENTIALS_FILE='/run/secrets/mockerp.json' ERP_PRIMARY_KEY='token-from-caller' \
  "$DEV_STACK" --dry-run >/dev/null

printf 'dev-stack dry-run checks passed\n'
