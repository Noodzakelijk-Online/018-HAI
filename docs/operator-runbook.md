# Operator Runbook

Day-to-day operation of 018-HAI on a single local host.

## Start / stop

```bash
# Bring the stack up (Postgres/Redis/Kafka/backend/frontend/gateway).
docker compose -f docker-compose.local.yml up -d
# Stop it.
docker compose -f docker-compose.local.yml down
```

## Health checks

| Check | Command | Healthy |
| --- | --- | --- |
| Liveness | `curl localhost/healthz` | `{"status":"ok"}` |
| Readiness | `curl localhost/readyz` | HTTP 200, `status: ready` |
| Config | `backend doctor` | exit 0, no `fail` lines |
| Data integrity | `backend reconcile` | "all memories satisfy their invariants" |

## Routine tasks

- **Rotate the backend key:** update `BACKEND_API_SHARED_KEY`, restart backend,
  update clients. `doctor` warns if empty.
- **Backups:** see `docs/backup-restore.md` (daily DB dump, weekly media).
- **Enable rate limiting** (if exposed beyond loopback): set
  `RATE_LIMIT_PER_MINUTE` > 0.

## Incident response

| Symptom | First step |
| --- | --- |
| `/readyz` = 503 | Read the failing checks; run `backend doctor`; fix config; restart. |
| Mass 401s | Client missing `X-HAI-Backend-Key`. |
| Runaway execution | Set `HAI_EMERGENCY_STOP=true` — halts LLM/automation/task/workflow execution while keeping planning/review visible. |
| Suspected bad data | `backend reconcile` for a dry-run integrity report. |

## Escalation

Collect a support bundle (build/version + readiness + counts; no secrets) and the
last relevant logs before escalating. See `docs/troubleshooting.md` for the error
catalog.
