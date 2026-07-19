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
| ai-gateways | LiteLLM | Integrated profile | Enable a keyed loopback gateway profile; HAI requires manual approval because proxy billing cannot be inferred. |
| Vector Database & Vector Store | pgvector | Integrated profile | Opt-in `vector` extension plus local embeddings; keyword retrieval remains the fallback. |
| Workflow Scheduler | Temporal | Candidate | Run one named local durable workflow through a narrow Go worker. |
| Monitoring Tool | Prometheus | Integrated profile | Enable a token-protected local metrics endpoint; configure a separate local collector when needed. |
| Model Context Protocol Client | MCP Inspector | Integrated profile | HAI-owned, local-only Streamable HTTP preflight lists tools before a separately reviewed adapter activation. |
| ChatGPT Alternatives | llama.cpp | Integrated local provider | Configure a loopback or `host.docker.internal` OpenAI-compatible `llama-server`; HAI probes `/v1/models` before it can route or generate. |
| Testing Tools | Playwright | Candidate | Verify named, allowlisted browser flows; it cannot bypass approval gates. |
| WebAssembly Runtime | Wasmtime | Candidate | Run reviewed capability-limited WASI helper modules only. |
| Optimization Solvers | OR-Tools | Integrated profile | Optional internal CP-SAT schedule proposals with bounded inputs and no apply capability. |
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

## Implemented local inference boundary

`llama.cpp` is now a first-class local provider in both HAI model back-office
registries. Set `LLAMA_CPP_BASE_URL` and, when needed,
`LLAMA_CPP_MODEL_ID` to enable it. The endpoint may only use `localhost`, a
loopback IP, or `host.docker.internal` for the Windows-host/Docker deployment
case. A configured value alone is not enough: HAI marks it usable only after a
live `/v1/models` probe, retains the EUR 0 paid budget, and continues to apply
the router's existing validation, fallback, audit, and approval controls.

## Implemented metrics boundary

HAI now includes an opt-in Prometheus exposition endpoint. Set
`HAI_PROMETHEUS_ENABLED=true` and a separate `HAI_PROMETHEUS_TOKEN`; only then
does `/metrics` exist. It requires a bearer token and exports HTTP request
counts and latency using route templates rather than raw paths. The exporter
does not emit source text, prompts, identities, record IDs, or credentials as
labels. A Prometheus collector is still separately configured and local by
default.

## Implemented LiteLLM boundary

LiteLLM is an optional, operator-installed local gateway profile in both HAI
model back-office registries. Set `LITELLM_ENABLED=true`,
`LITELLM_BASE_URL`, `LITELLM_MODEL_ID`, and a separate `LITELLM_API_KEY`.
HAI accepts only `localhost`, loopback, or `host.docker.internal`, probes
`/v1/models` with the virtual key, and sends the key only to that configured
gateway. HAI does not infer the gateway's upstream providers or billing from a
successful probe: each LiteLLM generation remains approval-gated and the EUR 0
policy remains authoritative.

## Implemented pgvector boundary

HAI now has an opt-in local semantic retrieval path using pgvector inside its
existing automation database. Set `HAI_SEMANTIC_RETRIEVAL_ENABLED=true` with a
loopback or `host.docker.internal` OpenAI-compatible embedding endpoint and a
named model. HAI indexes only already-ingested source extractions, keeps their
source ownership and sensitivity filters in the database query, and uses the
existing keyword search when the semantic path is disabled, empty, or
unavailable. It never sends source text to an arbitrary cloud URL and does not
create embeddings until the operator enables the feature.

## Implemented OR-Tools boundary

HAI now has an opt-in, internal-only OR-Tools CP-SAT planning service. The
`optimization` Compose profile is not started by default and exposes no host
port. HAI passes only opaque job IDs and bounded integer scheduling constraints
to it, validates the full response against the request, and saves an
owner-scoped proposal audit record. The service has no capability to apply a
proposal, call a tool, access sources, read files, or change a calendar or
workflow. Any implementation of a chosen proposal must use HAI's separate
approval and verification paths.
