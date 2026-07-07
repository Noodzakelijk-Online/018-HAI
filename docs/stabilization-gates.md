# Progressive Stabilization Gates

Work advances through gates; nothing is called "done" until it clears its gate.
This prevents half-finished features from being presented as complete.

## Gates

| Gate | Requirement to pass |
| --- | --- |
| G0 — Compiles | `go build ./...` succeeds. |
| G1 — Vetted | `go vet ./...` clean. |
| G2 — Unit-tested | New logic has passing unit tests; `go test ./...` green. |
| G3 — Wired | Behavior reachable through a real entrypoint (API/CLI), not internal-only. |
| G4 — Verified | Exercised end-to-end (test or documented manual run) with expected vs actual. |
| G5 — Documented | User/operator docs + completion-matrix status updated in the same change. |
| G6 — No-regression | Full suite green; existing packages unaffected. |

## Status semantics (used in the completion matrix)

- **Implemented** = cleared G0–G6 for its scope.
- **Partial** = cleared some gates (e.g. tested logic exists at G2 but not yet
  wired at G3, or wired but not fully verified at G4).
- **Missing** = not started.

Marking status honestly against these gates is what keeps "Implemented" meaningful
and prevents false completion.

## Application in this repo

Each pure package added in the goal run clears G0–G2 immediately; wiring into the
API (G3) and end-to-end verification (G4) are tracked explicitly, and the matrix
distinguishes "package exists + tested" from "wired + reachable."
