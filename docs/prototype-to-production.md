# Migration from Prototype to Production

A checklist to take 018-HAI from "works on my machine" to a production-safe local
deployment.

## Configuration

- [ ] Set `BACKEND_API_SHARED_KEY` (never run exposed without it).
- [ ] Set `HAI_MEMORY_ENCRYPTION_KEY` (distinct long passphrase).
- [ ] Set `RUN_MODE=production`.
- [ ] Enable `RATE_LIMIT_PER_MINUTE` if reachable beyond loopback.
- [ ] `backend doctor` exits 0 (no failing checks); `/readyz` ready.

## Data

- [ ] Real migration files applied (not only `AutoMigrate`) — `docs/migrations.md`.
- [ ] Backups configured and a restore tested — `docs/backup-restore.md`.
- [ ] `backend reconcile` clean.

## Security

- [ ] Security headers verified on responses.
- [ ] Secrets not in VCS; rotation schedule set.
- [ ] Threat-model residual risks reviewed — `docs/threat-model.md`.
- [ ] Dependency + license review current; `govulncheck` run.

## Providers

- [ ] Paid budget remains €0 unless explicitly approved.
- [ ] Only reviewed, minimal-scope connectors enabled.

## Operations

- [ ] Release/canary/rollback process understood — `docs/release-process.md`.
- [ ] Runbook and troubleshooting available to the operator.
- [ ] Monitoring on `/healthz` + `/readyz`.

## Cutover

Deploy behind the gateway, verify `/readyz`, smoke the critical path (create a
memory → workflow item → approval → verification → audit event), then enable
scheduled jobs. Roll back per the release process if `/readyz` is not green.
