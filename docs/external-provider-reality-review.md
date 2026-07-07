# External Provider Reality Review

Confirms external integrations are treated as real operational boundaries — not
placeholders presented as working.

## Providers in scope

| Provider | Status | Notes |
| --- | --- | --- |
| Local LLM (Ollama) | Probeable | `/api/tags` probe; free/local, preferred by fallback |
| OpenAI-compatible | Probeable, paid-gated | `/v1/models` probe; blocked until paid approval |
| Gmail / Drive / Calendar / Trello | Disabled | Sandbox adapters first; enabled only after scope review |
| GitHub (read issues) | Disabled | Token-gated; disabled by default |
| Kafka event bus | Operational | Configured brokers/topic |

## Reality checks (honest)

- **No provider is presented as production-ready unless it can actually be
  reached.** Paid and real-account providers are disabled by construction, not
  faked as connected.
- **Provider selection is real:** `internal/providerfallback` prefers available
  free/local providers and never selects a paid one unless explicitly allowed.
- **Failure is modelled, not ignored:** `internal/fakeprovider` simulates failures
  so handling is tested without a real provider; `backoff`/`worker` cover retries.
- **Assisted, not pretended:** where a provider can't be safely automated, the
  system prepares the work and tells the user what remains manual — it never
  claims the manual step happened.

## Gaps

1. Provider probe *history* persistence and last-success-per-provider are
   enhancements (register #33–36) not yet built.
2. Live end-to-end provider tests require real credentials + the running stack.

## Verdict

External integrations are honest operational boundaries. The defaults fail safe
(disabled/free-only), and nothing fake is dressed up as a live integration.
