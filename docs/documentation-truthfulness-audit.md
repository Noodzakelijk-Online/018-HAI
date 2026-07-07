# Documentation Truthfulness Audit

Checks that documentation describes what the code actually does — no aspirational
claims presented as shipped behavior.

## Method

Cross-check each significant claim in the docs against code/tests/executed
commands. Flag anything that overstates.

## Results

| Claim source | Claim | Verdict |
| --- | --- | --- |
| completion-matrix | "N Implemented" counts | **Corrected** — hand-maintained roll-up had drifted from row states; now recomputed from the actual rows. |
| matrix rows | Each Implemented row cites a package/file/endpoint | Verified — evidence is concrete and checkable. |
| performance-baseline | Benchmark numbers | Verified — numbers came from an executed `go test -bench` run. |
| fresh-clone-dryrun | "backend builds/vets/tests from clean clone" | Verified — commands executed; full compose boot explicitly marked **pending**. |
| readiness/doctor docs | `/readyz`, `backend doctor` behavior | Verified against tests and a live `go run ./cmd doctor`. |
| status labels | "Implemented" vs "Partial" | Mostly accurate; several "Implemented" are **tested packages/docs not yet wired end-to-end** — the rows say so and `stabilization-gates.md` defines the distinction. |

## Corrections applied

1. Completion-matrix roll-up recomputed to match actual rows.
2. "Implemented" evidence wording made explicit where a package exists+tested but
   is not yet wired (e.g. rbac, upload, apierror adoption noted as follow-ups).

## Standing rule

No doc may claim a capability as shipped unless code + a test (or an executed
command) backs it. Aspirational items live in the roadmap, not in feature docs.
