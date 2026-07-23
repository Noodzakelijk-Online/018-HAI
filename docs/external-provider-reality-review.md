# External Provider Reality Review

Confirms external integrations are treated as real operational boundaries, not
placeholders presented as working. Status below reflects **verified code
behavior** (the connector `AdapterStatus` the backend actually reports and the
tests that cover it), not intended capability. "Implemented" and "live-proven"
are deliberately different rows — see [Evidence levels](#evidence-levels).

## Providers in scope

| Provider | Adapter status (code) | Verified behavior | Evidence |
| --- | --- | --- | --- |
| Local LLM (Ollama) | Probeable | `/api/tags` probe; free/local, preferred by fallback | unit + probe |
| OpenAI-compatible | Probeable, paid-gated | `/v1/models` probe; blocked until paid approval | unit |
| Gmail (Google OAuth) | `operational`, unconfigured by default | Live read-only Gmail REST sync over Google OAuth, **metadata/snippet only**. Consent, encrypted token storage, and refresh are implemented (`internal/source/oauth.go`, `internal/googleoauth`). Reports `not_implemented` until `GOOGLE_OAUTH_CLIENT_ID/_SECRET/_REDIRECT_URL` are set. | unit-tested; **not yet sandbox/live-proven** |
| Trello (read-only REST) | `operational`, unconfigured by default | Live read-only sync of board cards, lists, due dates, and labels with incremental cursor and card-level provenance (`internal/source/trello.go`). Every request is a GET; there is no write path. Reports `not_implemented` until `TRELLO_API_KEY` + a least-privilege `TRELLO_READ_TOKEN` are set. | unit-tested (mock API); **not yet live-proven** |
| Trello JSON export (`project-board`) | `local_only` | Reads Trello JSON export files from the allowlisted local folder. Distinct from the live Trello adapter above. | unit-tested |
| Google Drive | `local_only` (export path only) | Synced document folders are ingested from the allowlisted local root. **A live Drive API connector is intentionally deferred** (see [Deferred](#intentionally-deferred)). | unit-tested |
| Google Calendar | `local_only` (export path only) | ICS exports are ingested from the allowlisted local root. **A live Calendar API connector is intentionally deferred.** | unit-tested |
| GitHub (read-only) | `operational` | Bounded REST sync of repositories, issues, pull requests, commits, and workflow runs. Public repositories read without a token; private/rate-limited use needs a least-privilege `GITHUB_SOURCE_TOKEN`. | unit-tested |
| Kafka event bus | Operational | Configured brokers/topic | compose/local |

## Evidence levels

- **Implemented** — code, persistence, API contract, and focused automated
  coverage exist in this repository.
- **Live-proven** — a real credential/account completed a bounded, approved,
  end-to-end run with audit and verification evidence. **No provider below is
  live-proven yet**; that requires the external gates listed in
  `docs/completion-matrix.md`.

## Reality checks

- **No provider is presented as production-ready unless it can actually be
  reached.** Gmail and Trello are honest about being *implemented but
  unconfigured*: the catalog downgrades each to `not_implemented` with a reason
  string until its credentials are present, so the dashboard never shows a live
  connection that cannot be made.
- **Read-only by construction.** The Gmail adapter requests only
  `gmail.readonly` and reads metadata/snippets. The Trello adapter issues only
  HTTP GET requests (enforced by test: a non-GET request fails the suite) and
  expects a token carrying only the `read` scope. Neither can mutate the remote
  account.
- **Least privilege, no stored secrets in rows.** Credentials come from the
  environment. No API key, token, or board id is persisted on the
  `ConnectedSource`. Google OAuth tokens are encrypted at rest and refreshed
  transparently.
- **Bounded egress.** Every remote fetch passes the shared host allowlist,
  blocked-address guard (link-local/metadata IPs rejected), timeout, and
  response-size cap. Trello's host (`api.trello.com`) was added to the default
  allowlist deliberately.
- **Provider selection is real:** `internal/providerfallback` prefers available
  free/local providers and never selects a paid one unless explicitly allowed.
- **Failure is retained, not ignored:** sync failures keep the cursor for retry
  and record a redacted audit entry; each live provider probe persists a
  redacted result.
- **Assisted, not pretended:** where a provider cannot be safely automated, the
  system prepares the work and tells the user what remains manual.

## Intentionally deferred

- **Google Drive and Google Calendar live API connectors.** The export/local
  paths are the only supported ingestion today; the live API adapters are
  deferred, not implied. They are marked `local_only` in the catalog so the UI
  cannot present them as live.
- **Write-back to any provider.** All adapters are read-only.

## Remaining gap

Live end-to-end provider tests require real credentials and a running stack:

- **Gmail** needs a Google Cloud OAuth app and a dedicated **sandbox mailbox**
  to prove consent → encrypted token storage → refresh → incremental sync →
  disconnect/revoke → retained source links.
- **Trello** needs a real `TRELLO_API_KEY` and a least-privilege
  `TRELLO_READ_TOKEN` against a throwaway board.
- **GitHub** needs a chosen repository and, where necessary, a least-privilege
  token.

These are tracked as external gates in `docs/completion-matrix.md`.

## Verdict

External integrations are honest operational boundaries. Gmail and Trello are
implemented as **live read-only adapters** that stay disabled until credentials
are supplied; Drive and Calendar live APIs are explicitly deferred; defaults
fail safe (disabled/free-only); and nothing is dressed up as live-proven before
a real credentialed run exists.
