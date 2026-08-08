# Trello HAI Card Requirements Source

This file records the requirements provenance of Robert's primary `018 - HAI`
card on the `005. Automating Life` board. It deliberately does not duplicate
private ShareT or shared-chat URLs into Git. The original Trello card remains
the source record.

## Source identity

| Field | Value |
| --- | --- |
| Trello card | `6a21e84f4cd00f2238c6fb14` (`lbEaWqHQ`) |
| Canonical repository | `Noodzakelijk-Online/018-HAI` |
| Last observed card activity | 2026-07-22T22:01:24.762Z |
| Checklists | none |
| Comments | 2 |
| Attachments | 2 |

## Attached specifications

| Artifact | SHA-256 | Role |
| --- | --- | --- |
| `HAI_COMPLETE_350K_CODEX_GOAL.md` | `AB1E4D1877DA0109B2F73430F0C33804386EA5ED78730DDBD62AA0693F4BCA17` | Newer exhaustive architecture corpus: unified framework composition, human sovereignty, whole-life ontology, agent teams, memory/evidence, progressive dashboard, domain packs, governance, persistence, APIs, and acceptance evidence. Its own loading protocol requires selective volume loading rather than wholesale prompt ingestion. |
| `018-HAI__Giant_Codex_Goal_Prompt.pdf` | `85722BC940E85FBB38F46192A93D4A7A631996FAF1F6265C84E41FF44F5C6178` | Earlier 112-phase production-hardening program covering repository integrity, critical-path behavior, provider reality, security, workers, QA, deployment, operations, and no-fake-success completion reporting. |

The Markdown corpus contains 13 modular volumes. The 55-framework architecture
used by the current implementation is a compact implementation lens over that
corpus, not a replacement for its detailed contracts. The PDF's 112 phases are
tracked in this directory's completion matrix, but historical status claims
must be revalidated against the current tree and real environment before they
are presented as current completion.

## Card-specific requirements

The card and its comments add these operational requirements beyond the broad
architecture catalogues:

1. Work from the canonical `main` history and deliver reviewable changes toward
   `main`; do not create a parallel product implementation.
2. Preserve work before execution capacity ends, but never bypass verification
   merely to produce a commit or pull request.
3. Prove the Trello integration by accurately representing a complex card that
   includes its description, at least 30 comments, at least five attachments,
   and mixed attachment types including text, media, and a Google Drive link.
4. Keep Trello access read-only for this ingestion and summarization flow.
5. Support Odysseus as an optional local runtime on Robert's device. It remains
   unavailable until a pinned upstream installation passes readiness and
   controlled-runtime safety checks; an adapter alone is not evidence that the
   runtime is installed or operational.
6. Model/reasoning-tier instructions in the card configure the Codex work run;
   they do not grant HAI execution authority and are not product runtime policy.

## Current implementation mapping

| Requirement | Current implementation | Remaining acceptance evidence |
| --- | --- | --- |
| Canonical stack and branch | Go backend, Angular frontend, Postgres, and guarded runtime adapters in this repository | Recheck branch/remote and full verification before final delivery |
| Complex Trello card intake | `backend/internal/source/trello.go` imports description, comments in chronological order, checklists, attachment metadata, card URL provenance, due dates, labels, and list/board context in one bounded read-only request | Run the explicit 30-comment/5-attachment live card acceptance case using a least-privilege Trello token |
| Attachment handling | Names, MIME types, sizes, and original URLs are indexed; attachment bodies are not downloaded implicitly | Add approved source-specific readers for text, image/video transcription, and Google Drive content before claiming full attachment comprehension |
| Odysseus | Guarded adapter, readiness contract, approval path, bounded output, and runtime diagnostics | Install a pinned compatible upstream release locally and pass readiness, dry-run, approval, audit, and verification tests |
| Exhaustive framework corpus | Registry, composition, governance, team, whole-life, evidence, learning, proactivity, outcome, and resilience modules are being mapped to the 55 framework areas | Keep requirement-level traceability and do not infer completion from structural presence |

## Safety interpretation

- The Trello card is requirements evidence, not an execution mandate.
- Private share links, credentials, tokens, and attachment download signatures
  must not be copied into Git or public logs.
- External actions remain bounded, allowlisted, approval-gated, idempotent,
  audited, and verified.
- `Implemented` means wired and evidenced behavior. `Partial`, `Blocked`, and
  `Needs review` remain valid outcomes; no document can override real runtime
  evidence.
