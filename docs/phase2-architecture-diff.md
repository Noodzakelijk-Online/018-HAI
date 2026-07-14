# HAI Phase 2 — Architecture Diff & Final Report (§37/§40)

**Branch** `codex/hai-next` (fork). Base (upstream main) `5c6c358`.
**Standard: working software. No false completion.**

## New backend packages
`operations`, `background`, `executionbroker`, `accountfeed`, `modelintelligence`,
`hardwareprofile`, `runtimelab`, `opscontrol`, plus additions to `privacyfilter`,
`autonomypolicy`, `idempotency`, `models`, `infra`, `router`, `phase2`.

## New/changed API routes
- **Operations:** `GET /operations`, `/operations/dashboard`, `/operations/:id`,
  `/:id/events`, `/:id/approvals`; `POST /:id/approve|reject|later|block-similar|run|evidence-pack`.
- **Evidence packs:** `GET /evidence-packs/:id`.
- **Background/runtime control:** `POST /background/run`, `GET /background/status`,
  `POST /background/pause|resume`, `PATCH /background/mode`;
  `GET /windows-runtime/readiness`, `POST /windows-runtime/recovery`,
  `POST /windows-runtime/emergency-stop/verify`.
- **Account feeds:** `GET/POST /account-feeds`, `GET/PATCH /account-feeds/:id`,
  `POST /account-feeds/:id/sync`, `POST /account-feeds/sync-due`,
  `GET /account-feeds/:id/audit`, `GET /account-feeds/bridges|permissions`.
- **Model intelligence:** `/model-intelligence/overview|profiles|profiles/:p/:m|
  profiles/:p/:m/benchmark|benchmarks|telemetry|lane-winners|cache|token-budgets`.
- **Hardware/power/privacy:** `/hardware/profile|detect`, `/power/policy`,
  `/privacy/scan|scans|scans/:id`.
- **Runtime lab:** `/runtime-lab/overview`, `/:runtimeId/probe|self-test|attempts`.

## New UI pages (Angular, lazy-loaded)
Background Operations, Model Intelligence, Runtime Lab, Account Bridges, Runtime
Control — plus Control Center nav entries.

## New safety boundaries
- No completion without passing verification.
- Only the local safe worker executes; external runtimes/bridges refuse without
  operator verification / approval; no fake OAuth / connected / execution.
- Emergency stop halts background processing, is persisted (survives restart),
  and is self-verifiable.
- Privacy filter redacts secrets before storage/model use; evidence packs never
  embed raw sensitive content.
- Hardware detection never claims Windows ML/GPU/NPU without detection.

## Old behavior
- **Preserved:** all Phase-1 packages and routes (critical-path smoke 19/19).
- **Replaced:** none — Phase 2 is additive.

## Deferred / blocked / requires Robert
- **Deferred (by the prompt, §33):** 2E Controlled External Actions.
- **Blocked on real credentials / target machine:** live Gmail/GitHub/Trello/
  Drive/Calendar reads; live Hermes/OpenClaw/Odysseus/DSpark/Ollama; Windows ML/
  GPU/NPU detection; Windows install/startup/tray verification. All surfaced
  honestly (`credentials_required` / `setup_required` / `pending`).

## Exact smoke path & test results
- `go build ./...` OK; `go vet` OK; `go test ./...` → 66 packages ok, 0 failures.
- `scripts/no-fake-claims-audit.sh` → 13/13.
- `scripts/smoke-all.sh` → 84/84 (background 14, model-intelligence 23,
  runtime-lab 15, account-bridges 16, windows-runtime 16).
- `scripts/smoke-critical-path.sh` (pre-existing) → 19/19.
- Frontend `ng build` → clean.
- **Docker:** not required/used by the smokes (local `initdb` Postgres). Docker
  detection code exists + tested; daemon not pinged in CI.

## Mock/demo/test-only behavior
The `test-fast-triage` / `test-verifier` model providers are deterministic local
providers, honestly labelled test-only; they never claim to be a cloud provider
or reach the network. No other mock/demo behavior exists.

## No-secrets confirmation
Phase 2 added no env/secret/db/model-weight files. Pre-existing repo-baseline
`.env*` files carry dev defaults (`DB_PASSWORD=postgres`).

## No-fake-claims statement
No fake provider, runtime, account, model, or Windows execution path is claimed
anywhere. Every unavailable capability is labelled honestly and enforced in code,
asserted by the audit and smokes.

## Acceptance scoring (§36, 0-5)
Working vertical slice 5 · Operation Ledger 5 · Autonomy policy 5 · Approval UX 5
· Verification 5 · Audit completeness 5 · Model intelligence 5 · Windows/runtime
truthfulness 5 · Dashboards 5 · No-fake-claims discipline 5 · Test coverage 5 ·
Documentation truthfulness 5 (these mandatory flowchart docs).

## What is NOT claimed
Fully-autonomous *external* action is not claimed — 2E is deferred and external
sends remain gated by approval + a real runtime that does not exist in this phase.
Windows-specific runtime capability is pending verification on Robert's target
machine.
