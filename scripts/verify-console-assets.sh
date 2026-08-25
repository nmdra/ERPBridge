#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

# Keep the tracked fallback sentinel, but remove prior hashed build output so
# repeated local verification does not accumulate assets in the size budget.
rm -rf internal/web/prebuilt/build/assets
npm run build --prefix web >/dev/null

go_binary=$(mktemp)
log_file=$(mktemp)
cleanup() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$go_binary" "$log_file"
}
trap cleanup EXIT

go build -o "$go_binary" ./tools/bridgectl/main.go
"$go_binary" web --no-open >"$log_file" 2>&1 &
server_pid=$!

for _ in {1..50}; do
  if grep -q "ERPBridge Console:" "$log_file"; then
    break
  fi
  sleep 0.1
done

capability_url=$(sed -n 's/^ERPBridge Console: //p' "$log_file" | head -n 1)
if [[ -z "$capability_url" ]]; then
  echo "console did not print a capability URL" >&2
  cat "$log_file" >&2
  exit 1
fi
base_url=${capability_url%%#*}
html=$(curl --fail --silent "$base_url/")
if grep -q "fallback asset" <<<"$html"; then
  echo "console served the fallback asset" >&2
  exit 1
fi
if ! grep -q "ERPBridge Console" <<<"$html"; then
  echo "console HTML does not contain the application title" >&2
  exit 1
fi

compressed_bytes=$(find internal/web/prebuilt/build/assets -type f \( -name '*.js' -o -name '*.css' \) -print0 |
  xargs -0 -r gzip -c | wc -c | tr -d ' ')
if ((compressed_bytes > 768000)); then
  echo "compressed console assets exceed 750 KiB: ${compressed_bytes} bytes" >&2
  exit 1
fi

echo "console assets verified (${compressed_bytes} compressed bytes)"
