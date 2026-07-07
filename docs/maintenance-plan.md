# Post-Completion Maintenance Plan

Keeps 018-HAI healthy after delivery.

## Cadence

| Frequency | Task |
| --- | --- |
| Daily | DB backup; confirm `/readyz` green; skim error logs. |
| Weekly | Media archive; `backend reconcile` integrity scan; dependency `outdated` review. |
| Monthly | Restore-from-backup drill into a scratch DB; `govulncheck` + `npm audit`; review the tech-debt register. |
| Per release | Run the pre-release gates and canary in `docs/release-process.md`. |

## Ownership

- One operator owns backups and readiness.
- Security items (RBAC enforcement, path-safety adoption, vuln scanning) tracked
  in `docs/technical-debt.md` with an owner and target.

## Monitoring signals

- `/healthz` and `/readyz` for liveness/readiness.
- `backend doctor` exit code in CI/deploy scripts.
- Rate-limit 429s and dead-letter counts as early-warning signals.

## Upgrade policy

- Patch dependencies monthly; security patches immediately.
- Pin the Go toolchain and Node version; bump deliberately with a green test run.

## Definition of "still healthy"

Green build + green `go test ./...`, `/readyz` ready, clean `reconcile`, current
backups with a tested restore, and no open High findings in the bug-hunt log.
