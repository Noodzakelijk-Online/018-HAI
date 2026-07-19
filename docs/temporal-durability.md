# Local Temporal Durability Bridge

HAI includes an opt-in local [Temporal](https://github.com/temporalio/temporal)
bridge for exactly one durable workflow: a governed check of due HAI open loops.
It is a retry-safe scheduling layer, not a second agent framework or a general
execution gateway.

## What it can do

An approval-capable HAI user can schedule a future follow-up check. When it is
due, the Temporal activity calls HAI's existing owner-scoped
`RunDueOpenLoopsForOwner` service. That service may create an HAI checklist item
and a follow-up **proposal**. It cannot send email, post publicly, operate a
browser, call a connector, run a script, modify a calendar, or resolve an HAI
approval.

The activity is idempotency-aware: HAI's existing open-loop claim and
follow-up-artifact checks remain authoritative. The HAI database receives a
durable run record before Temporal is asked to start the workflow, and records
scheduled, running, completed, or failed status afterwards.

## Privacy and isolation

- Temporal services are started only with the `durability` Compose profile.
- No Temporal port is published to the host or the Internet.
- Temporal uses its own local PostgreSQL volume, separate from HAI's data.
- The workflow input contains an HAI run UUID, timestamp, and bounded limit.
  It does not contain the HAI owner identity, source text, emails, attachments,
  credentials, prompts, or tool payloads.
- `HAI_TEMPORAL_ADDRESS` accepts only `temporal`, loopback addresses,
  `localhost`, or `host.docker.internal`.
- The profile pins `temporalio/server` and `temporalio/admin-tools` at `1.31.2`.

## Enable locally

1. Set a strong local-only `HAI_TEMPORAL_POSTGRES_PASSWORD`.
2. Set `HAI_TEMPORAL_ENABLED=true` in your local `.env`.
3. Run `docker compose --profile durability up --build`.
4. HAI retries local worker startup for roughly 150 seconds while the namespace
   comes online. Use `POST /api/v1/temporal/worker/start` as an HAI admin only
   if it remains unavailable after that bounded startup window.
5. Use `GET /api/v1/temporal/status` to confirm the worker is configured and
   started. Schedule a check through `POST /api/v1/temporal/follow-up-runs`.

All routes are authenticated and owner-scoped. Worker start is admin-only;
scheduling is approval-capable. A disabled or unreachable local service returns
an explicit configuration/unavailable response rather than falling back to a
cloud endpoint.

## API surface

| Route | Permission | Purpose |
| --- | --- | --- |
| `GET /api/v1/temporal/status` | read | Shows local configuration and worker state without probing or starting work. |
| `GET /api/v1/temporal/follow-up-runs` | read | Lists only the authenticated owner's durable-run ledger. |
| `POST /api/v1/temporal/worker/start` | admin | Starts the one proposal-only local worker after Temporal is available. |
| `POST /api/v1/temporal/follow-up-runs` | approve | Schedules one bounded due-open-loop check. |

Example request:

```json
{
  "runAt": "2026-07-20T09:00:00Z",
  "limit": 10
}
```

`runAt` must be no more than 365 days ahead and `limit` is capped at 50.
