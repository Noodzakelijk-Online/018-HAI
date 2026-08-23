# Provider Credential Verification Checklist

Before enabling any real external provider, verify each item. Until all pass, the
provider stays disabled (the default).

## Per-provider checklist

- [ ] **Credential present** — required API key / OAuth token is configured via
  env/secret store (never committed to VCS).
- [ ] **Scopes minimal** — OAuth scopes are the least needed; scope diff reviewed
  and approved.
- [ ] **Live probe passes** — provider probe reaches the endpoint (Ollama
  `/api/tags`, OpenAI-compatible `/v1/models`) without following redirects.
- [ ] **Cost posture** — paid usage stays impossible until explicit approval;
  `daily_paid_budget_eur` remains 0 for free-only operation.
- [ ] **Rate/quota respected** — provider quota + budget ledger configured.
- [ ] **Failure handling** — retries/backoff + dead-letter cover transient
  failures (`internal/backoff`, `internal/worker`); simulate with
  `internal/fakeprovider` first.
- [ ] **Redaction** — provider error bodies and outputs are redacted before
  logging/returning.
- [ ] **Rotation** — a rotation schedule is set for the credential
  (`internal/secretrotation`).
- [ ] **Audit** — actions against the provider emit audit events
  (`internal/auditevent`).

## Verification aids

- `backend doctor` / `/readyz` warn when security-sensitive keys are unset.
- `internal/providerfallback` guarantees free/local is preferred and paid is
  never selected unless explicitly allowed.
- The Trello board reader is deliberately bounded to 1,000 cards per board
  response. A response at that ceiling is rejected as potentially truncated;
  HAI does not advance the cursor or call the sync complete. Split, archive, or
  otherwise reduce the board's active scope before retrying.

## Sign-off

A provider is "verified" only when every box is checked and a probe has succeeded
in the target environment. Record the sign-off (who, when, scopes) alongside the
enablement change.
