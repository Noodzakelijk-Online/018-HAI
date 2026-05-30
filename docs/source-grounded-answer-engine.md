# Source-Grounded Answer Engine

The source-grounded answer engine builds answers from retrieved evidence rather
than model memory alone. Citations are not enough: each important claim receives
a verification status and a support explanation.

## Answer Modes

- `draft`: brainstorming only; claims remain uncertain.
- `grounded`: factual claims need connected-source or provided evidence support.
- `strict`: unsupported claims are blocked or marked for review.
- `action`: high-risk or external-action claims require grounded evidence and
  human approval before they can affect workflows.

## Source Ranking

Sources are ranked by:

- authority: official and primary sources score highest
- relevance: overlap with the research question
- provenance: connected-source records and source URIs are preferred
- freshness: recent records are preferred for time-sensitive work
- source type: generated or weak sources are rejected unless explicitly useful

Connected-source search runs first for project, account, file, and history
questions. Public/current questions can include external evidence records now;
web-search workers can later feed the same evidence schema.

## Memory and Action Controls

Memory updates are disabled unless explicitly requested by the caller. Even then,
only claims with `verified`, `source_supported`, `test_passed`, or
`human_approved` status are stored. Unsupported, uncertain, conflicting, or
high-risk unapproved claims stay in review.
