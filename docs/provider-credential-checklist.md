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

For Gmail's bounded read-only client acceptance, use a dedicated sandbox OAuth
client and mailbox, set `GMAIL_LIVE_CLIENT_ID`, `GMAIL_LIVE_CLIENT_SECRET`,
`GMAIL_LIVE_REFRESH_TOKEN`, and a non-sensitive test message ID in
`GMAIL_LIVE_EXPECT_MESSAGE_ID`, then run:

```powershell
cd backend
go test -tags live -run LiveGmail -v ./internal/googleoauth ./internal/source
```

The client test refreshes the credential and reads one selected message. The
source test projects that same bounded message into HAI's source-import shape,
checking its stable external identity, project provenance, source link, and
content envelope. Neither test prints message content, headers, addresses,
subjects, or token values. Treat a skipped test as no live acceptance evidence.

## Verification aids

- `backend doctor` / `/readyz` fail when required security-sensitive keys are
  unset in production; non-production modes show an explicit warning.
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
