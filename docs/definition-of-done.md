# Feature-Level Definition of Done

A feature is Done only when every item below is true. Partial features say so.

## Checklist

- [ ] **Behavior implemented** — real, wired code (not a mock or a doc that
  describes intended behavior).
- [ ] **Reachable** — exposed through a real entrypoint (API route / CLI) or
  consumed by one.
- [ ] **Tested** — unit tests for the logic; where it crosses HTTP, an httptest;
  `go vet` + `go test ./...` green.
- [ ] **Safe inputs** — hostile/degenerate inputs handled (no panic, bounded
  output) where the surface is user-facing.
- [ ] **Errors** — failures return a clear, typed error (`apierror`) with the
  right status; no silent success.
- [ ] **Docs** — user/operator docs updated; completion matrix status set with
  evidence.
- [ ] **No regression** — existing suites still green; no existing contract
  changed without intent.
- [ ] **Honest status** — if any box is unchecked, the matrix says Partial, not
  Implemented.

## Anti-checklist (never call it done if…)

- It only exists as documentation.
- It's wired but unverified end-to-end.
- It changes an existing response shape the frontend depends on without updating
  the frontend.
- Tests were skipped "because it obviously works."
