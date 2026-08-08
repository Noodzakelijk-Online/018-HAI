# Anti-Hallucination Verification Layer

The verification layer treats model output as a draft until it is checked against
evidence, deterministic validators, tests, or human approval. Unsupported claims
are not promoted into memory or action workflows.

## Verification Statuses

- `verified`
- `source_supported`
- `schema_validated`
- `test_passed`
- `human_approved`
- `uncertain`
- `conflicting`
- `unsupported`
- `needs_review`

## Pipeline

1. Generate a draft answer or extraction.
2. Split important output into atomic claims.
3. Attach available connected-source or provided evidence references.
4. Re-resolve connected-source hits to the exact owner-scoped extraction and
   raw item, then reject unavailable or payload-mismatched records.
5. Use token overlap only to identify candidate support. Authenticated
   provenance can produce `source_supported`; it cannot by itself produce
   `verified` or establish semantic truth.
6. Run deterministic arithmetic checks for simple numeric claims.
7. Treat authenticated test-result evidence as `test_passed` when it supports
   code claims.
8. Mark high-risk claims as `needs_review` unless human approval is present.
9. Detect simple source contradictions.
10. Deduplicate repeated retrieval hits before ranking and persistence.
11. Persist run, evidence, claim statuses, and audit logs.
12. Only verified, source-supported, test-passed, or human-approved claims can
    be promoted into memory.

Connected-source provenance is resolved independently from the text returned by
search. Freshness comes from the exact raw item's `fetched_at`; a newer
source-level sync timestamp does not refresh old evidence. The raw payload is
not copied into authorization inspection records.

## API Surface

- `POST /api/v1/verification/answer`
- `GET /api/v1/verification/runs`
- `GET /api/v1/verification/runs/:id`
- `POST /api/v1/knowledge/claims`
- `GET /api/v1/knowledge/claims?workspaceId=<workspace>`
- `GET /api/v1/knowledge/claims/review-queue?workspaceId=<workspace>`
- `GET /api/v1/knowledge/claims/:id/lifecycle?workspaceId=<workspace>`
- `GET /api/v1/knowledge/claims/:id/assessment?workspaceId=<workspace>`
- `POST /api/v1/knowledge/claims/:id/corrections`

Knowledge claims are immutable, owner/workspace-scoped atomic assertions.
Assessment compares only claims with the same subject and predicate and returns
`supported`, `corroborated`, `conflicting`, `superseded`, or `needs_review`.
Corroboration requires distinct provenance references and distinct content
digests. Free-form authority labels never elevate a claim, and a bounded scan
that may be truncated fails closed as `needs_review`.

Grounded verification runs with a project key automatically project eligible
`source_supported`, `test_passed`, `human_approved`, or source-linked
`verified` claims into this immutable store. The projection remains local-only,
hashes the exact supporting evidence snippet, is idempotent, and reports a
redacted projection failure in the verification result and audit log.

Generic claim creation cannot assign `verified`, `source_supported`,
`schema_validated`, `test_passed`, or `human_approved`. Those states must come
from their dedicated verification, test, or approval boundary. Human
corrections use the approval-gated correction route and append an immutable
successor plus a local audit source; the original claim remains unchanged and
is linked through `supersedes`. A correction is `human_approved`, but is not
reported as externally source-supported unless independent source evidence
also exists.

The **Truth review** module exposes the deterministic review queue, temporal
assessment, source evidence, and immutable lifecycle. Its Basic view keeps
conflicts and unsupported claims at the front. Workspace/as-of controls and
the complete claim register remain module-local Advanced sections. Correction
controls are shown only to callers with approval permission.

The dashboard page **Grounded Answers** shows the generated answer, source
quality, claim-level status, unsupported claims, missing sources, and previous
verification runs.
