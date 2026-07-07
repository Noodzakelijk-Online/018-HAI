#!/usr/bin/env bash
#
# Critical-path smoke test (phases 003 / 011 / 043).
#
# Boots a throwaway PostgreSQL, runs the backend against it, and exercises the
# critical path end-to-end: readiness -> memory create -> memory search ->
# workflow overview -> grounded/system surfaces. Tears everything down on exit.
#
# Requires: a local `postgres`/`initdb`/`pg_ctl`/`createdb` and Go. Does NOT
# require Docker. In CI this can run against the compose stack instead.
#
# Usage: scripts/smoke-critical-path.sh
set -euo pipefail

PG_PORT="${PG_PORT:-55432}"
API_PORT="${API_PORT:-18080}"
API_KEY="${API_KEY:-smoke-key}"
BASE="http://127.0.0.1:${API_PORT}/api/v1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

WORKDIR="$(mktemp -d)"
PGDATA="${WORKDIR}/pgdata"
IMAGES="${WORKDIR}/images"
BIN="${WORKDIR}/hai-backend"
BACKEND_PID=""

pass=0
fail=0

cleanup() {
  set +e
  [ -n "${BACKEND_PID}" ] && kill "${BACKEND_PID}" 2>/dev/null
  pg_ctl -D "${PGDATA}" stop -m fast >/dev/null 2>&1
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

check() { # name, expected_substr, actual
  if echo "$3" | grep -q "$2"; then
    echo "  PASS: $1"; pass=$((pass + 1))
  else
    echo "  FAIL: $1 (wanted '$2', got: $3)"; fail=$((fail + 1))
  fi
}

echo "==> Starting throwaway PostgreSQL on :${PG_PORT}"
initdb -D "${PGDATA}" -U "$(whoami)" --auth=trust --locale=C --encoding=UTF8 >/dev/null
pg_ctl -D "${PGDATA}" -o "-p ${PG_PORT} -h 127.0.0.1" -l "${PGDATA}/server.log" start >/dev/null
for i in $(seq 1 30); do
  pg_isready -h 127.0.0.1 -p "${PG_PORT}" >/dev/null 2>&1 && break
  sleep 1
done
createdb -h 127.0.0.1 -p "${PG_PORT}" automation

echo "==> Building and starting backend on :${API_PORT}"
mkdir -p "${IMAGES}"
( cd "${ROOT}/backend" && go build -o "${BIN}" ./cmd )

# Trust auth ignores the password, but libpq needs a non-empty value in the DSN.
DB_HOST=127.0.0.1 DB_PORT="${PG_PORT}" DB_USER="$(whoami)" DB_PASSWORD=postgres \
  DB_NAME=automation SERVER_PORT="${API_PORT}" BASE_URL=/api \
  BACKEND_API_SHARED_KEY="${API_KEY}" IMAGE_SAVE_DIR="${IMAGES}" \
  RUN_MODE=production KAFKA_BROKERS="" \
  "${BIN}" > "${WORKDIR}/backend.log" 2>&1 &
BACKEND_PID=$!

echo "==> Waiting for liveness"
ready=""
for i in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/healthz" >/dev/null 2>&1; then ready=1; break; fi
  if ! kill -0 "${BACKEND_PID}" 2>/dev/null; then
    echo "backend exited early; log:"; tail -20 "${WORKDIR}/backend.log"; exit 1
  fi
  sleep 1
done
[ -n "${ready}" ] || { echo "backend never became live; log:"; tail -20 "${WORKDIR}/backend.log"; exit 1; }

hdr=(-H "X-HAI-Backend-Key: ${API_KEY}" -H "Content-Type: application/json")

echo "==> Critical path"
check "healthz ok" '"status":"ok"' "$(curl -fsS http://127.0.0.1:${API_PORT}/healthz)"
check "readyz ready" 'ready' "$(curl -sS http://127.0.0.1:${API_PORT}/readyz)"

created="$(curl -sS "${hdr[@]}" -X POST "${BASE}/memory/" \
  -d '{"projectKey":"smoke","kind":"note","content":"Angular dashboard with Go backend","tags":["stack"]}')"
check "memory create" 'Angular dashboard' "${created}"

check "memory search finds it" 'Angular dashboard' \
  "$(curl -sS "${hdr[@]}" "${BASE}/memory/query?projectKey=smoke&q=angular%20backend")"

check "workflow overview" '{' "$(curl -sS "${hdr[@]}" "${BASE}/workflow/overview")"
check "os overview" '{' "$(curl -sS "${hdr[@]}" "${BASE}/os/overview")"
check "system info build" 'goVersion' "$(curl -sS "${hdr[@]}" "${BASE}/system/info")"

echo "==> Workflow lifecycle (intake -> approval gate -> resolve -> audit trail)"
intake="$(curl -sS "${hdr[@]}" -X POST "${BASE}/workflow/intake" \
  -d '{"input":"Email from lawyer about legal hearing. Draft a formal reply for review only."}')"
wf_id="$(echo "${intake}" | jq -r '.item.id // empty')"
check "intake created a workflow item" 'true' "$([ -n "${wf_id}" ] && echo true)"
check "item carries an approval gate" 'true' "$(echo "${intake}" | jq -r '(.item|has("approvalStatus"))')"
check "item persisted & fetchable by id" "${wf_id}" "$(curl -sS "${hdr[@]}" "${BASE}/workflow/${wf_id}" | jq -r '.item.id')"
check "approvals queue reachable" '200' "$(curl -sS -o /dev/null -w '%{http_code}' "${hdr[@]}" "${BASE}/workflow/approvals")"
check "workflow dashboard reachable" '200' "$(curl -sS -o /dev/null -w '%{http_code}' "${hdr[@]}" "${BASE}/workflow/dashboard")"

resolved="$(curl -sS "${hdr[@]}" -X POST "${BASE}/workflow/${wf_id}/approval" \
  -d '{"approved":true,"note":"Reviewed and approved this exact draft.","actor":"smoke"}')"
check "approval gate resolved" 'true' "$(echo "${resolved}" | jq -r 'has("item")')"
check "audit trail recorded (events/transitions/decisions)" 'true' \
  "$(echo "${resolved}" | jq -r '(((.events // [])|length) + ((.transitions // [])|length) + ((.decisions // [])|length)) > 0')"
check "verification runs surface reachable" '200' \
  "$(curl -sS -o /dev/null -w '%{http_code}' "${hdr[@]}" "${BASE}/verification/runs")"

echo ""
echo "==> Result: ${pass} passed, ${fail} failed"
echo "Note: grounded verification (LLM answer generation) is not exercised — no LLM provider is configured in this smoke; the verification runs surface is asserted instead."
[ "${fail}" -eq 0 ]
