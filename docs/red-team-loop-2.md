# Red-Team Loop 2 — Data Integrity, Injection & Privacy

Adversarial review focused on malicious/degenerate data and information leakage.

## Surface reviewed

- Query/search inputs (`memory.Query`).
- File paths for ingestion/uploads (`internal/pathsafety`).
- Secret handling and log/output redaction.
- Data invariants and reconciliation.

## Findings

| ID | Severity | Finding | Mitigation / status |
| --- | --- | --- | --- |
| RT2-1 | Medium | Free-text search over untrusted input could panic or blow up on pathological strings. | Covered by `memory/adversarial_test.go`: MaxInt pagination, 200k-char search, control/unicode/SQL-ish strings — all degrade safely, output bounded. |
| RT2-2 | High (if unguarded) | Path traversal via `../` or absolute paths in ingestion filenames. | `pathsafety.SafeJoin`/`IsSafeRelative` reject escapes; traversal tests included. **Follow-up:** ensure all upload/ingest call sites route through it. |
| RT2-3 | Medium | Cross-project data leakage through listing. | `memory/isolation_test.go` proves project-scoped queries never return other projects' memories. |
| RT2-4 | Low | Bad data at rest (out-of-range confidence, missing required fields). | `internal/invariants` + `backend reconcile` detect and classify violations (repairable vs manual). |
| RT2-5 | Info | Secrets could leak into logs/errors. | Existing redaction helpers strip passwords/tokens/keys from runtime output, error bodies, and URLs (engineering register #11–20). |

## Attempted attacks & result

- **SQL-injection-style search strings** → treated as literal tokens, no query
  construction from user text in the in-memory path (blocked).
- **`../../etc/passwd` as a relative path** → rejected by `SafeJoin` (blocked).
- **Query another project's memories via `projectKey`** → only that project's
  rows returned (blocked).

## Verdict

No data-leak or injection path found in reviewed surfaces. Primary open action:
enforce `pathsafety` at every file-handling call site (RT2-2 follow-up).
