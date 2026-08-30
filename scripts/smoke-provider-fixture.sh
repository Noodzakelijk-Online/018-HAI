#!/usr/bin/env bash
# Build and exercise the local-only provider compatibility fixture. This does
# not start HAI, download a model, or contact any provider.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${HAI_PROVIDER_FIXTURE_IMAGE:-hai-provider-fixture-smoke}"
NAME="hai-provider-fixture-smoke-$$"
NETWORK="hai-provider-fixture-smoke-net-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker build --target provider-fixture -t "$IMAGE" "$ROOT/backend"
docker network create --internal "$NETWORK" >/dev/null
docker run -d \
  --name "$NAME" \
  --network "$NETWORK" \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 32 \
  --memory 32m \
  --cpus 0.10 \
  -p 127.0.0.1:0:11434 \
  "$IMAGE" >/dev/null

for _ in $(seq 1 20); do
  if docker exec "$NAME" /provider-fixture --healthcheck >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$NAME" /provider-fixture --healthcheck >/dev/null

binding="$(docker port "$NAME" 11434/tcp | head -n 1)"
port="${binding##*:}"
base="http://127.0.0.1:${port}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"; cleanup' EXIT

curl --fail --silent --show-error --connect-timeout 2 --max-time 5 "$base/api/tags" >"$tmp_dir/ollama.json"
curl --fail --silent --show-error --connect-timeout 2 --max-time 5 "$base/v1/models" >"$tmp_dir/openai.json"

python3 - "$tmp_dir" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
ollama = json.loads((root / "ollama.json").read_text(encoding="utf-8"))
openai = json.loads((root / "openai.json").read_text(encoding="utf-8"))

assert ollama["models"][0]["name"] == "hai-fixture-ollama:latest"
assert openai["data"][0]["id"] == "hai-fixture-openai"
PY

echo "Provider fixture live HTTP contract passed."
