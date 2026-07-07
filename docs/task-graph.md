# Task Graph & Dependency Map

How work flows through the system and how the modules depend on each other.

## Runtime task flow

```
Connected Source ──sync──▶ Extraction ──▶ Memory (dedup/store)
                                   │
                                   ▼
                            Workflow Intake
                                   │
                     ┌─────────────┼──────────────┐
                     ▼             ▼               ▼
               Task Planning   Pursuit         Ambient scan
                     │        (grouping)            │
                     ▼                              ▼
              LLM/Tool Routing ◀── provider fallback
                     │
                     ▼
              Approval / Autonomy Gate ──review──▶ Human queue
                     │ auto
                     ▼
              Controlled Execution ──retry/backoff──▶ Dead-letter
                     │
                     ▼
               Verification (grounded) ──▶ Workflow state update
                     │
                     ▼
                 Audit Event
```

## Module dependency (backend, high level)

```
router ──▶ {automation, llm, memory, source, workflow, pursuit, verification,
            task, ambient, agentcycle, haios, autonomy, doctor, system}
task ──▶ memory, llm, source, verification
workflow ──▶ memory (+ statemachine concept)
source ──▶ memory, workflow, pursuit
verification ──▶ source, memory
new utilities (leaf, no inbound deps): ratelimit, idempotency, pathsafety,
   featureflags, quality, retention, reconcile→invariants, backoff, worker→backoff,
   autonomygate, actionresolver, session, i18n, demomode, buildinfo, supportbundle→
   {doctor, buildinfo}, dataexport, importexport, upload→pathsafety, entitlements,
   reminders, secretrotation, checkpoint, factories, fakeprovider
```

## Observations

- The new utility packages are **leaves** (few/no inbound dependencies), which is
  why they were safe to add without disturbing existing suites.
- The heaviest coupling is around `memory` (many consumers) — its interface was
  deliberately **not** extended during the goal run to avoid breaking the five
  packages that fake it.
- Critical-path ordering matches the requirements traceability doc.
