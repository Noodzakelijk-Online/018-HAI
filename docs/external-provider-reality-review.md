# External Provider Reality Review

External integrations are operating boundaries, not placeholders presented as
working. This document distinguishes implemented code, retained bounded live
evidence, and the configuration or acceptance work still required for a newly
connected account.

## Providers In Scope

| Provider | Adapter status | Verified behavior | Evidence |
| --- | --- | --- | --- |
| Local LLM (Ollama) | Probeable | Local/free model probe and fallback preference. | Unit and probe coverage; no configured live model acceptance run. |
| OpenAI-compatible endpoint | Probeable, paid-gated | Model probe; paid use remains approval-gated. | Unit coverage; no configured live model acceptance run. |
| Gmail (Google OAuth) | `operational`, unconfigured by default | Read-only metadata/snippet ingestion; consent, encrypted token storage, refresh, incremental sync, revoke, and retained source links. Reports `not_implemented` until `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, and `GOOGLE_OAUTH_REDIRECT_URL` are set. | Unit-tested and live-tested against a real sandbox mailbox on 2026-07-23. |
| Trello (read-only REST) | `operational`, unconfigured by default | Read-only board card, list, due-date, and label sync with incremental cursor and card provenance. Every request is a GET; there is no write path. Reports `not_implemented` until a board id and least-privilege `TRELLO_API_KEY` and `TRELLO_READ_TOKEN` are set. | Unit-tested and live-tested against a real board on 2026-07-23. |
| Trello JSON export | `local_only` | Reads Trello JSON exports from an allowlisted local folder. This is distinct from the Trello REST adapter. | Unit-tested. |
| Google Drive | `local_only` | Reads a synced document folder within the local allowlist. Live Drive API access is intentionally deferred. | Unit-tested. |
| Google Calendar | `local_only` | Reads ICS exports from the local allowlist. Live Calendar API access is intentionally deferred. | Unit-tested. |
| GitHub (read-only) | `operational` | Bounded REST sync of repositories, issues, pull requests, commits, and workflow runs. Private or rate-limited access needs a least-privilege token. | Unit-tested. |
| Kafka event bus | Operational | Configured broker/topic integration. | Compose/local coverage. |

## Evidence Levels

- **Implemented**: code, persistence, API contract, and focused automated
  coverage exist in this repository.
- **Live-tested**: a real credential/account completed a bounded approved
  end-to-end run with audit and verification evidence. This proves that exact
  scenario, not any future account or configuration.
- **Live-proven for current use**: the currently configured account, model, or
  runtime has its own bounded retained acceptance evidence. No LLM provider or
  agent runtime currently meets this bar.

The detailed Gmail and Trello evidence is retained in
`docs/completion-matrix.md`.

## Safety Controls

- Gmail requests only `gmail.readonly` and reads metadata/snippets. Trello
  issues only HTTP GET requests and expects a read-scoped token. Neither has a
  remote mutation path.
- Credentials remain environment-managed. Source rows do not persist API keys,
  tokens, or board ids. Google OAuth tokens are encrypted at rest.
- Remote fetches use the shared allowlist, blocked-address guard, timeout, and
  response-size cap. Trello's API host is explicitly allowlisted.
- The local-first provider policy never selects a paid model without explicit
  policy approval. A failed probe or sync keeps its failure/audit record rather
  than reporting simulated success.

## Deferred And Remaining Work

Google Drive and Google Calendar live APIs remain deliberately deferred; the
supported paths are local exports and synced folders. All source adapters are
read-only: provider write-back is not implemented.

Gmail and Trello are unconfigured by default despite their retained bounded
acceptance evidence. Any newly connected account must complete its own
consent/token, source-sync, audit, and revoke test before it is relied upon.
GitHub needs a chosen repository and, where necessary, a least-privilege token.
LLM provider and agent-runtime acceptance runs remain separate external gates.

## Verdict

HAI exposes Gmail and Trello as implemented read-only connectors with bounded
live evidence, not as permanently connected accounts. Drive and Calendar live
APIs are explicitly deferred. The defaults fail safe: unconfigured connectors
and providers do not claim a live connection, and no source, model, or runtime
is treated as operational for a new account until that account has its own
verified run.
