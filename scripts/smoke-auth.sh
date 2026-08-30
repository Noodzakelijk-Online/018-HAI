#!/usr/bin/env bash

# Production-mode smoke suites intentionally use deterministic, test-only
# secrets unless their caller supplies stronger values. Keeping these here
# prevents individual smoke flows from silently bypassing startup hardening.
: "${HAI_MEMORY_ENCRYPTION_KEY:=smoke-a7c4d9e1f2b8c6d0e5f7a3b9c1d2e4f6}"
: "${HAI_APPROVAL_PROOF_SIGNING_KEY:=smoke-f6e4d2c1b9a3f7e5d0c6b8f2e1d9c4a7}"
export HAI_MEMORY_ENCRYPTION_KEY HAI_APPROVAL_PROOF_SIGNING_KEY

hai_smoke_mint_jwt() { # role, secret, optional subject
  local role="$1"
  local secret="$2"
  local subject="${3:-local-operator}"
  python3 - "${role}" "${secret}" "${subject}" <<'PY'
import base64
import hashlib
import hmac
import json
import sys
import time

role, secret, subject = sys.argv[1:4]

def encode(value):
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode()

header = encode(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = encode(
    json.dumps(
        {"sub": subject, "role": role, "exp": int(time.time()) + 3600},
        separators=(",", ":"),
    ).encode()
)
signature = encode(
    hmac.new(secret.encode(), f"{header}.{payload}".encode(), hashlib.sha256).digest()
)
print(f"{header}.{payload}.{signature}")
PY
}
