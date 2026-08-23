#!/bin/sh
set -eu

fail() { echo "ngrok exposure gate: $1" >&2; exit 1; }

[ "${RUN_MODE:-}" = "production" ] || fail "RUN_MODE must be production"
[ "${LOCAL_LOGIN_BYPASS_ENABLED:-}" = "false" ] || fail "local login bypass must be false"
[ "${IDP_COOKIE_SECURE:-}" = "true" ] || fail "secure IDP cookies are required"
[ "${GATEWAY_HOST_BIND:-}" = "127.0.0.1" ] || fail "gateway host bind must remain loopback-only"
[ "${HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED:-false}" = "false" ] || fail "A2A must remain private while a public tunnel is active"
case "${RATE_LIMIT_PER_MINUTE:-}" in
  ''|*[!0-9]*) fail "RATE_LIMIT_PER_MINUTE must be a positive integer while a public tunnel is active" ;;
  0) fail "RATE_LIMIT_PER_MINUTE must be a positive integer while a public tunnel is active" ;;
esac

token="${NGROK_AUTHTOKEN:-}"
[ "${#token}" -ge 20 ] || fail "a dedicated ngrok authtoken is required"

case "${HAI_NGROK_URL:-}" in
  https://*.ngrok.app|https://*.ngrok.dev|https://*.ngrok-free.app|https://*.ngrok-free.dev) ;;
  *) fail "HAI_NGROK_URL must be a fixed HTTPS ngrok origin" ;;
esac
origin="${HAI_NGROK_URL#https://}"
case "$origin" in
  */*|*'?'*|*'#'*|*'@'*) fail "HAI_NGROK_URL must not contain credentials, a path, query, or fragment" ;;
esac

[ "${HAI_NGROK_VALIDATE_ONLY:-false}" != "true" ] || { echo "ngrok exposure gate: validation passed"; exit 0; }
exec /bin/ngrok http http://nginx:80 --config=/etc/ngrok.yml --url="$HAI_NGROK_URL" --log=stdout --log-format=json
