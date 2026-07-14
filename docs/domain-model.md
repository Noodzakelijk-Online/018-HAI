# Domain Model Specification

The core concepts and how they relate.

## Entities

| Entity | Meaning | Key fields |
| --- | --- | --- |
| Context Memory | A remembered fact/preference/decision/contact | project, kind, content, tags, confidence, archived |
| Connected Source | A local folder / JSON feed to ingest | connector, status, schedule, last-sync |
| Extraction | A record extracted from a source | source, content, provenance, review-state |
| Workflow Item | A unit of work moving through states | state, checklist, approval, retries/dead-letter |
| Pursuit | A longer-running goal grouping related work | goal, why it matters, desired outcome, status, evidence, next-actions |
| Verification Run | A grounded-answer check | claim, evidence links, verdict |
| Audit Event | An immutable record of an action | actor, action, resource, result, at |

## Relationships

```
Connected Source ──produces──▶ Extraction ──feeds──▶ Memory / Workflow Item
Workflow Item ──belongs to──▶ Pursuit
Task/Answer ──uses──▶ Memory (retrieval) + Source (evidence) ──▶ Verification Run
Every state-changing action ──emits──▶ Audit Event
```

## Lifecycles

- **Workflow Item:** intake → planned → awaiting_approval → executing →
  done/failed (failed may replan). Enforced by a state machine
  (`internal/statemachine`).
- **Memory:** active ⇄ archived → deleted; retention policy decides archival/
  deletion candidacy (`internal/retention`).

## Ownership & scope

Memories and sources are scoped by project key; project-scoped queries never
leak across projects (proven by isolation tests). Multi-user ownership/roles are
modelled (`internal/rbac`) but not yet enforced in middleware.
