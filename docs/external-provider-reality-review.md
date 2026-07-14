# External Provider Reality Review

Confirms external integrations are treated as real operational boundaries, not
placeholders presented as working.

## Providers in scope

| Provider | Status | Notes |
| --- | --- | --- |
| Local LLM (Ollama) | Probeable | `/api/tags` probe; free/local, preferred by fallback |
| OpenAI-compatible | Probeable, paid-gated | `/v1/models` probe; blocked until paid approval |
| Gmail / Drive / Calendar / Trello | Export/local-folder ingestion only | Authorized MBOX/EML, ICS, document-folder, and Trello JSON exports can be imported from the allowlisted local root. Live OAuth/API connectors are not implemented. |
| GitHub (read-only repositories and work) | Implemented, unconfigured by default | Bounded REST sync covers repositories, issues, pull requests, commits, and workflow runs. Public repositories can be read without a token; private/rate-limited use needs a least-privilege `GITHUB_SOURCE_TOKEN`. |
| Kafka event bus | Operational | Configured brokers/topic |

## Reality checks

- **No provider is presented as production-ready unless it can actually be
  reached.** Paid and real-account providers are disabled by construction, not
  faked as connected.
- **Provider selection is real:** `internal/providerfallback` prefers available
  free/local providers and never selects a paid one unless explicitly allowed.
- **Source access is bounded:** export imports are read-only under the local
  folder allowlist, and GitHub sync is read-only. Neither path grants HAI live
  access to Gmail, Drive, Calendar, Trello, or GitHub write operations.
- **Failure is retained, not ignored:** each live provider probe persists a
  redacted result, including the latest failure and last successful check, so
  the dashboard can distinguish unconfigured, unproven, and recently failed
  endpoints. `internal/fakeprovider` still simulates generation failures for
  deterministic tests; `backoff`/`worker` cover retries.
- **Assisted, not pretended:** where a provider cannot be safely automated, the
  system prepares the work and tells the user what remains manual. It never
  claims the manual step happened.

## Remaining gap

Live end-to-end provider and source tests require real credentials and a
running stack. GitHub needs a chosen repository and, where necessary, a
least-privilege token.

## Verdict

External integrations are honest operational boundaries. The defaults fail safe
(disabled/free-only), and nothing fake is dressed up as a live integration.
