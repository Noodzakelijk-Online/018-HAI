# Red-Team Loop 3 — Dependencies, Supply Chain & Operational Resilience

Adversarial review focused on the software supply chain and failure behavior.

## Surface reviewed

- Go/Angular dependency posture and pinning.
- Build/CI reproducibility.
- Committed artifacts.
- Failure and recovery behavior (rate limits, retries, emergency stop).

## Findings

| ID | Severity | Finding | Mitigation / status |
| --- | --- | --- | --- |
| RT3-1 | Low | `go.mod` targets Go 1.21 while the toolchain in use is 1.25.6; CI does not pin the Go version. | Pin Go in `.github/workflows/ci.yml` for reproducible builds (BH-3). |
| RT3-2 | Low | `hai-engine-control.zip` (2.2 MB binary) committed at repo root — opaque to review, a supply-chain blind spot. | Extract to source or move to release assets (BH-2). |
| RT3-3 | Medium | No automated dependency vulnerability scan in CI. | Add `govulncheck` (Go) and `npm audit` (frontend) as CI gates; see release-process gates. |
| RT3-4 | Low (flaky) | `agentruntime` CLI test is load-sensitive (5s timeout), which could mask a real regression under CI load. | Raise timeout or isolate the package run (BH-1). |
| RT3-5 | Info | Resilience controls exist: retry budget + backoff + dead-letter, per-IP rate limiting, and an emergency stop that blocks LLM/automation/task/workflow execution. | Confirmed present; keep exercised in tests. |

## Attempted attacks & result

- **Flood the API** → per-IP rate limiter returns 429 when enabled (blocked).
- **Force runaway execution** → emergency stop (`HAI_EMERGENCY_STOP`) halts
  execution paths while preserving planning/review visibility (blocked).
- **Exhaust worker retries** → dead-letter state captures exhausted items rather
  than looping forever (contained).

## Verdict

No exploitable supply-chain vector found, but two hygiene gaps (unpinned Go,
committed binary) and one missing CI control (vuln scanning) should be closed.
Resilience posture is solid.
