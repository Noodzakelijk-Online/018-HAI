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
4. Check citation recall and source support through token overlap and source
   provenance.
5. Run deterministic arithmetic checks for simple numeric claims.
6. Treat test-result evidence as `test_passed` when it supports code claims.
7. Mark high-risk claims as `needs_review` unless human approval is present.
8. Detect simple source contradictions.
9. Persist run, evidence, claim statuses, and audit logs.
10. Only verified, source-supported, test-passed, or human-approved claims can
    be promoted into memory.

## API Surface

- `POST /api/v1/verification/answer`
- `GET /api/v1/verification/runs`
- `GET /api/v1/verification/runs/:id`

The dashboard page **Grounded Answers** shows the generated answer, source
quality, claim-level status, unsupported claims, missing sources, and previous
verification runs.
