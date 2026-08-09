#!/bin/sh
set -eu

fail() {
  echo "ngrok exposure gate: $1" >&2
  exit 1
}

[ "${RUN_MODE:-}" = "production" ] || fail "RUN_MODE must be production"
[ "${LOCAL_LOGIN_BYPASS_ENABLED:-}" = "false" ] || fail "local login bypass must be false"
[ "${IDP_COOKIE_SECURE:-}" = "true" ] || fail "secure IDP cookies are required"
[ "${GATEWAY_HOST_BIND:-}" = "127.0.0.1" ] || fail "gateway host bind must remain loopback-only"
ngrok_token="${NGROK_AUTHTOKEN:-}"
[ "${#ngrok_token}" -ge 20 ] || fail "a dedicated ngrok authtoken is required"

case "${HAI_NGROK_URL:-}" in
  https://*.ngrok.app|https://*.ngrok.dev|https://*.ngrok-free.app|https://*.ngrok-free.dev)
    ;;
  *)
    fail "HAI_NGROK_URL must be a fixed HTTPS ngrok origin"
    ;;
esac

ngrok_origin="${HAI_NGROK_URL#https://}"
case "$ngrok_origin" in
  */*|*'?'*|*'#'*|*'@'*)
    fail "HAI_NGROK_URL must not contain credentials, a path, query, or fragment"
    ;;
esac

if [ "${HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED:-false}" = "true" ]; then
  [ "${HAI_A2A_BRIDGE_ENABLED:-false}" = "true" ] || fail "public A2A requires HAI_A2A_BRIDGE_ENABLED=true"
  a2a_token="${HAI_A2A_BRIDGE_TOKEN:-}"
  [ "${#a2a_token}" -ge 32 ] || fail "public A2A requires a dedicated 32+ character bridge token"
  [ -n "${HAI_A2A_BRIDGE_OWNER_ID:-}" ] || fail "public A2A requires one named owner"
  [ "${HAI_A2A_BRIDGE_URL:-}" = "${HAI_NGROK_URL%/}/api/v1/a2a" ] || fail "public A2A URL must exactly match the fixed ngrok origin"
fi

if [ "${HAI_NGROK_VALIDATE_ONLY:-false}" = "true" ]; then
  echo "ngrok exposure gate: validation passed"
  exit 0
fi

exec /bin/ngrok http http://nginx:80 \
  --config=/etc/ngrok.yml \
  --url="$HAI_NGROK_URL" \
  --log=stdout \
  --log-format=json
