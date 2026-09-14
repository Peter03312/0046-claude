#!/usr/bin/env bash
# One-shot verification, run inside the `verify` container by docker compose.
# Steps: Go tests, browser tests (against a real Go API + Vite), production
# build, then an HTTP smoke test against the running web/api containers.
set -euo pipefail

cd /src/backend
echo "==> go test ./..."
go test ./...

echo "==> build api binary for the browser-test harness"
go build -o /tmp/storyproof-api ./cmd/server
export STORYPROOF_API_BIN=/tmp/storyproof-api

cd /src/web
echo "==> playwright browser tests"
E2E_API_PORT=18080 /src/scripts/e2e-api.sh npx playwright test

echo "==> production build"
npm run build

echo "==> HTTP smoke (against compose services)"
# docker compose binary is mounted/available when invoked with the compose
# profile; wait for both health endpoints.
for i in $(seq 1 60); do
  if curl -fsS "http://web:80/" >/dev/null && curl -fsS "http://api:8080/api/health" >/dev/null; then
    break
  fi
  sleep 2
  if [ "$i" = "60" ]; then
    echo "services did not become healthy" >&2
    exit 1
  fi
done

curl -fsS http://web:80/ | grep -q "透明故事校样台"
curl -fsS http://web:80/api/health | grep -q '"ok"'
curl -fsS http://api:8080/api/health | grep -q '"ok"'

# End-to-end API smoke through the nginx proxy: project -> sheet -> act -> proof.
BASE=http://web:80
PID=$(curl -fsS -X POST "$BASE/api/projects" -H 'Content-Type: application/json' \
  -d '{"name":"smoke"}' | sed -E 's/.*"id":([0-9]+).*/\1/')
SID=$(curl -fsS -X POST "$BASE/api/projects/$PID/sheets" -H 'Content-Type: application/json' \
  -d '{"name":"s","vertices":[{"x":0,"y":0},{"x":4,"y":0},{"x":4,"y":4},{"x":0,"y":4}],"r":10,"g":20,"b":30,"opacityMillis":1000}' \
  | sed -E 's/.*"id":([0-9]+).*/\1/')
curl -fsS -o /dev/null -X POST "$BASE/api/projects/$PID/acts" -H 'Content-Type: application/json' \
  -d "{\"name\":\"a\",\"sheets\":[{\"sheetId\":$SID,\"rotation\":0,\"tx\":0,\"ty\":0}],\"regions\":[{\"name\":\"t\",\"kind\":\"target\",\"vertices\":[{\"x\":0,\"y\":0},{\"x\":4,\"y\":0},{\"x\":4,\"y\":4},{\"x\":0,\"y\":4}],\"r\":10,\"g\":20,\"b\":30,\"tolerance\":0}]}"
REPORT=$(curl -fsS -X POST "$BASE/api/projects/$PID/proofs")
echo "$REPORT" | grep -q '"status":"passed"'

echo "ALL VERIFY STEPS PASSED"
