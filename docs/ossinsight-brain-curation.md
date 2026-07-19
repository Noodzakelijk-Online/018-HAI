# OSS Insight Brain And Operations Curation

## Purpose

This document records the 2026-07-19 review of
[OSS Insight Collections](https://ossinsight.io/collections). OSS Insight is a
repository-discovery and ranking source. HAI uses it to discover candidate
capabilities, never to automatically download, vendor, enable, or execute
third-party code.

## Review rule

An upstream project becomes a HAI catalog entry only when it has a concrete
role in one of these planes:

| Plane | Needed capability | HAI decision rule |
| --- | --- | --- |
| Thinking | Model routing, retrieval, evidence context | Must preserve local-first routing, source provenance, and verified completion. |
| Memory | Semantic search or graph enrichment | Must not create competing truth stores or bypass retention controls. |
| Operations | Durable jobs, retries, follow-ups | Must not self-authorize work or conceal workflow state. |
| Execution | Runtime, code tools, MCP | Must have a reviewed adapter, scoped workspace, tool allowlist, audit trail, and approval gate. |
| Observability | Health, queue, model, and cost telemetry | Must report real source-backed metrics, not placeholders. |

## Curated outcomes

| Collection | Candidate | Disposition | First safe increment |
| --- | --- | --- | --- |
| ai-gateways | LiteLLM | Candidate | Add an optional local proxy profile with paid providers disabled, then run a scoped health probe. |
| Vector Database & Vector Store | pgvector | Candidate | Add a reversible Postgres extension migration and local embedding backfill plan. |
| Workflow Scheduler | Temporal | Candidate | Run one named local durable workflow through a narrow Go worker. |
| Monitoring Tool | Prometheus | Candidate | Expose minimal authenticated metrics and local scrape configuration. |
| Model Context Protocol Client | MCP Inspector | Candidate | Use it only to test an allowlisted MCP server before adapter activation. |
| ChatGPT Alternatives | llama.cpp | Candidate | Add only a loopback local model server through HAI's existing local-provider policy and health check. |
| Testing Tools | Playwright | Candidate | Verify named, allowlisted browser flows; it cannot bypass approval gates. |
| WebAssembly Runtime | Wasmtime | Candidate | Run reviewed capability-limited WASI helper modules only. |
| Optimization Solvers | OR-Tools | Candidate | Return deterministic planning proposals with constraints and assumptions. |
| Monitoring Tool | Grafana | Reference only | Revisit only after real Prometheus data needs advanced visualization. |
| GraphRAG | LangChain, LlamaIndex, Cognee | Reference only | Revisit only for a measured retrieval or graph-provenance gap. |
| Vector Database & Vector Store | Qdrant | Reference only | Revisit only if pgvector proves insufficient with a migration and rollback plan. |
| Zapier Alternatives | Activepieces | Reference only | Do not add a second workflow control plane without a demonstrated connector gap. |
| LLM Tools | Mem0 | Reference only | HAI retains one memory/provenance owner. |
| Open Source Data Catalogs | OpenMetadata | Reference only | Reference governance patterns without deploying a second source catalog. |
| Zapier Alternatives | n8n | License review | Fair-code licensing and overlapping workflow ownership need an explicit decision. |
| Distributed File Storage | MinIO | Excluded | Archived upstream and AGPLv3 do not meet the current adoption bar. |

## Non-negotiable boundaries

1. A catalog candidate is not an installed integration.
2. No candidate can override model policy, paid-budget controls, source permissions, approval, audit, or emergency stop.
3. No external project receives arbitrary local shell, browser, credential, or network access from catalog registration.
4. A health check proves reachability, not safe task execution.
5. Real integration requires its own configuration, adapter, failure tests, owner approval policy, and rollback path.

## Back-office behavior

`GET /api/v1/brain-catalog/` returns the curated entries, their collection
provenance, activation boundary, status, and recommendation data. Planning can
surface a matching candidate, but tool routing marks it unavailable until the
specific adapter is present and reviewed.

The complete category pass, candidate shortlist, and exclusion rationale are in
[the OSS Insight screening ledger](ossinsight-screening-ledger.md).
