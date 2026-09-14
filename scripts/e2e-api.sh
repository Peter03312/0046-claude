#!/usr/bin/env bash
# Start the Go API on an ephemeral SQLite database for browser tests, run the
# given command, then stop the API. Vite proxies /api to VITE_API_TARGET.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DB_DIR="$(mktemp -d)"
PORT="${E2E_API_PORT:-18080}"
trap 'kill "${API_PID:-}" 2>/dev/null || true; rm -rf "$DB_DIR"' EXIT

# Refuse to run if a stale server already owns the port; a healthy foreign
# process would otherwise make the readiness check pass against old data.
if curl -fsS "http://localhost:$PORT/api/health" >/dev/null 2>&1; then
  echo "port $PORT is already serving; stop the stale process first" >&2
  exit 1
fi

echo "starting api on :$PORT with db in $DB_DIR"
if [ -n "${STORYPROOF_API_BIN:-}" ]; then
  ADDR=":$PORT" DB_DIR="$DB_DIR" "$STORYPROOF_API_BIN" &
else
  (
    cd "$ROOT/backend"
    ADDR=":$PORT" DB_DIR="$DB_DIR" go run ./cmd/server
  ) &
fi
API_PID=$!

# Wait for health, failing fast if the server process died.
for _ in $(seq 1 60); do
  if curl -fsS "http://localhost:$PORT/api/health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "api process exited before becoming healthy" >&2
    exit 1
  fi
  sleep 1
done
curl -fsS "http://localhost:$PORT/api/health" >/dev/null

export VITE_API_TARGET="http://localhost:$PORT"
exec "$@"
