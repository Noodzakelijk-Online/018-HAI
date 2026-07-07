# Requirements Traceability

Traces the goal prompt's critical-path requirement to the code and tests that
satisfy each link, so every requirement has a verifiable home.

## Critical path

`dashboard → source ingestion/memory → task planning → LLM/tool routing →
approval gates → controlled execution → verification → workflow state → audit`

| Requirement link | Implementation | Test/evidence |
| --- | --- | --- |
| Authenticated dashboard | `idp/`, API-key middleware, `fe/pages/login` | router smoke; `session` |
| Source ingestion | `be/source`, `connected-sources` | `source/service_test.go` |
| Memory | `be/memory` | `memory/service_test.go`, `query_test.go` |
| Task planning | `be/task` | `task/service_test.go` |
| LLM/tool routing | `be/llm`, `be/router`, `providerfallback` | `llm/*_test.go`, `providerfallback` |
| Approval gates | `be/safety`, `be/workflow`, `autonomygate` | `safety/*`, `autonomygate` |
| Controlled execution | `be/automation`, `be/agentruntime` | `automation/*_test.go` |
| Verification | `be/verification` | `verification/handler_test.go` |
| Workflow state | `be/workflow`, `statemachine` | `workflow/*_test.go`, `statemachine` |
| Audit | `be/events`, `auditevent` | `auditevent` |

## Cross-cutting requirements

| Requirement | Home | Evidence |
| --- | --- | --- |
| No false completion | honest matrix + `stabilization-gates` | rows cite evidence |
| Bounded/rate-limited external actions | `ratelimit`, `idempotency`, `backoff` | package tests |
| Safety boundary (no unrestricted autonomy) | `autonomygate`, `actionresolver`, emergency stop | package tests |
| Grounded verification | `be/verification` | tests |

## Coverage summary

Every critical-path link has a real implementation and at least one test. The
open traceability gaps are end-to-end (cross-link) tests that require a live
stack — tracked at 003/043.
