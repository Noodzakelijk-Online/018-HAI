# Backend Endpoint Usage Audit

Enumerates the real HTTP surface (from `internal/router/routes.go`) and confirms
each route maps to a handler. Unauthenticated probes are intentional; everything
under `/api/v1` requires `X-HAI-Backend-Key` when configured.

## Unauthenticated

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/healthz` | liveness |
| GET | `/readyz` | readiness (doctor diagnosis; 200/503) |
| GET | `/swagger/*` | API docs |

## Authenticated (`/api/v1`)

| Group | Representative routes |
| --- | --- |
| `/automation` | GET `/`, GET `/:id`, POST `/`, PATCH `/`, DELETE `/:id`, POST `/:id/launch`, health/diagnostics, `/images/:imageName` |
| `/agent-runtimes` | GET `/`, `/health`, `/:id/skills`, openclaw ecosystem get/set/refresh/upload |
| `/llm` | GET `/policy`, `/probes`, `/logs`; POST `/route`, `/generate` |
| `/memory` | GET `/`, **GET `/query`**, GET `/:id`, `/export`; POST `/`, `/retrieve`, `/:id/archive|restore`; PATCH/DELETE `/:id` |
| `/memory-engine` | import, dashboard, search, conversations, insights |
| `/sources` | connectors, list/create, search, sync-due, sync-jobs, extractions, audit-logs, per-source sync/pause/resume/revoke |
| `/workflow` | overview, approvals, dashboard, list, intake, run-due, transitions, approval/interruption/proposal resolve, checklist |
| `/pursuits` | list/create, dashboard, brief, decisions, per-pursuit evidence/activity/next-actions/blockers/approvals |
| `/verification` | POST `/answer`, GET `/runs`, `/runs/:id` |
| `/task` | plan, run, success, logs, review-queue |
| `/assistant`, `/agent-cycle`, `/autonomy`, `/ambient`, `/os` | command/logs, run, overview/stress, scan/needs, overview |
| **`/flags`** | GET — feature flags (added this goal run) |
| **`/system`** | **GET `/info`**, **GET `/support-bundle`** (added this goal run) |

## Findings

- Every registered route resolves to a handler (verified by the router smoke test
  and `go build`).
- New reachable surfaces added this run: `/memory/query`, `/flags`, `/system/info`,
  `/system/support-bundle`, plus `/readyz`.
- No orphaned/dead routes found in `routes.go`.
- **Follow-up:** adopt the `apierror` envelope uniformly across handlers (error
  shapes currently vary; frontend depends on the existing shapes — migrate both
  together).
