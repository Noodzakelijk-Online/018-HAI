# Curated Agent Tool Catalog

HAI uses [e2b-dev/awesome-ai-agents](https://github.com/e2b-dev/awesome-ai-agents) and [OSS Insight Collections](https://ossinsight.io/collections) as discovery sources, not installation sources. A ranking or awesome-list is not a security review, a stable API contract, or permission to run third-party code on Robert's device.

## Operating rule

- The catalog is read-only at `GET /api/v1/brain-catalog/`.
- Listing a project never downloads, installs, enables, or executes it.
- The back-office **Start review** action creates a normal, owner-scoped HAI pursuit with the catalog provenance and adapter-review gates; it does not activate the project.
- An owner-admin can run `POST /api/v1/brain-catalog/:id/revalidate` to retrieve bounded public GitHub metadata for one fixed catalog entry. The recheck never fetches source code or changes an adoption decision.
- The optional local SearXNG profile is a discovery adapter, not an answer engine. `GET /api/v1/research/status` shows its configuration and `POST /api/v1/research/search` returns bounded, unverified source candidates only when the owner has enabled a local endpoint.
- Task planning can recommend a project capability, but does not select it as an executable tool.
- A project becomes executable only after a dedicated adapter has been reviewed, configured, health-checked, and routed through HAI's existing approval and audit controls.
- HAI remains the policy owner: an external framework cannot bypass the local-first policy, paid budget, source controls, folder allowlist, emergency stop, or approval queue.

## Curation snapshot: 2026-07-19

| Project | HAI disposition | Intended role | Why |
| --- | --- | --- | --- |
| [Continue](https://github.com/continuedev/continue) | Candidate | Source-controlled coding checks and review | Active Apache-2.0 project with a focused review/CI surface. Requires a check-only adapter before HAI uses it. |
| [Cline](https://github.com/cline/cline) | Candidate | Review-first interactive coding assistance | Active Apache-2.0 LLM-devtool. Any HAI bridge needs a confined workspace, explicit model provider, tool/network allowlists, and approval before write-capable work. |
| [OpenCode](https://github.com/anomalyco/opencode) | Candidate | Review-first terminal coding assistance | Active MIT MCP-client/terminal project. Any HAI bridge needs a confined workspace, explicit model provider, tool/network allowlists, and approval before write-capable work. |
| [OpenHands](https://github.com/OpenHands/OpenHands) | Candidate | Isolated development-agent runtime | Active project, but workspace and tool access are high-risk. It requires a local container, workspace/network allowlists, and an approval-gated adapter. |
| [CrewAI](https://github.com/crewAIInc/crewAI) | Candidate | Planning and multi-agent orchestration patterns | Active MIT framework. HAI retains the policy, audit, verification, and execution gates. |
| [Aider](https://github.com/Aider-AI/aider) | Candidate | Review-first coding assistance | Available Apache-2.0 coding tool. Any write-capable use needs a confined workspace and explicit approval. |
| [E2B](https://github.com/e2b-dev/E2B) | Reference only | External sandbox design | Its hosted execution model is not local-first and can involve external credentials/billing. Disabled unless separately approved. |
| [AutoGPT](https://github.com/Significant-Gravitas/AutoGPT) | License review | Workflow platform reference | The repository is active but includes differently licensed areas. HAI does not vendor or integrate it until a per-directory license review is complete. |
| [AutoGen](https://github.com/microsoft/autogen) | Compatibility only | Existing AutoGen workload migration, structured agent-event translation, and guarded MCP compatibility | The official project is maintenance mode. HAI does not install or execute AutoGen code, and a reviewed bridge plus approval is required. |
| [MetaGPT](https://github.com/FoundationAgents/MetaGPT) | Excluded | Architecture reference only | Still available, but its release and substantive push activity were older than the active candidates at curation time. |
| [LiteLLM](https://github.com/BerriAI/litellm) | Integrated profile | Keyed loopback provider-gateway normalization | Requires explicit enablement, a local endpoint, model alias, virtual key, probe, and manual generation approval; HAI's EUR 0 policy remains authoritative. |
| [pgvector](https://github.com/pgvector/pgvector) | Integrated profile | Local semantic retrieval inside HAI Postgres | Opt-in `vector` extension plus local embeddings; keyword retrieval remains the truthful fallback. |
| [Temporal](https://github.com/temporalio/temporal) | Integrated, opt-in | Durable governed follow-up checks | Local-only service plus one narrow Go worker. It creates HAI proposals only; HAI retains all approval and completion gates. |
| [Prometheus](https://github.com/prometheus/prometheus) | Integrated profile | Token-protected HTTP metrics export | Opt-in exporter with no raw-data labels; a local collector remains separately configured. |
| [Grafana](https://github.com/grafana/grafana) | Reference only | Optional advanced metrics visualization | Deferred until real Prometheus metrics justify a second dashboard. |
| [MCP Inspector](https://github.com/modelcontextprotocol/inspector) | Integrated profile | Local-only pre-activation MCP inspection | HAI performs only a bounded Streamable HTTP handshake and tool inventory for configured local endpoints; it never spawns a process or calls a tool. |
| [llama.cpp](https://github.com/ggml-org/llama.cpp) | Integrated, opt-in | Local GGUF model inference | Loopback-only model server through HAI's existing local-provider, provenance, live-probe, and approval policy. |
| [Playwright](https://github.com/microsoft/playwright) | Integrated, opt-in | Read-only local browser verification | Named allowlisted local routes only; no clicks, forms, downloads, retained state, public origins, sending, publishing, purchasing, or account changes. |
| [SearXNG](https://github.com/searxng/searxng) | Integrated, opt-in | Local public-source discovery | Operator-hosted local JSON endpoint only. HAI uses bounded queries and returns source candidates, never fetches result pages or accepts snippets as verified facts. AGPL-3.0 requires a separate license and deployment review. |
| [Wasmtime](https://github.com/bytecodealliance/wasmtime) | Integrated, opt-in | Bounded local WASI helper runtime | Reviewed modules only, with no inherited network and explicit resource/capability limits; every run remains approval-gated. |
| [OR-Tools](https://github.com/google/or-tools) | Integrated profile | Internal deterministic CP-SAT schedule proposals | Opt-in `optimization` Compose profile accepts bounded opaque jobs and returns an audited proposal only; it has no workflow, calendar, filesystem, tool, or external-network apply endpoint. |
| [LangChain](https://github.com/langchain-ai/langchain) | Reference only | Retrieval and tool-orchestration patterns | HAI will not add a parallel agent stack without a documented gap. |
| [LlamaIndex](https://github.com/run-llama/llama_index) | Reference only | Connected-source and retrieval patterns | Deferred while HAI's native extraction, search, and pgvector path mature. |
| [Cognee](https://github.com/topoteretes/cognee) | Reference only | Evidence-graph and entity-linking patterns | Deferred until a graph-query need, provenance model, and retention plan are proven. |
| [Qdrant](https://github.com/qdrant/qdrant) | Reference only | Future dedicated vector-store option | Deferred to avoid a second vector store before pgvector has a measured limit. |
| [Activepieces](https://github.com/activepieces/activepieces) | Reference only | Connector and workflow-pattern reference | Do not introduce a competing workflow, secrets, or approval control plane by default. |
| [Mem0](https://github.com/mem0ai/mem0) | Reference only | Memory-consolidation reference | HAI remains the sole personal-memory and provenance authority. |
| [OpenMetadata](https://github.com/open-metadata/OpenMetadata) | Reference only | Source-governance reference | Defer its independent metadata control plane until an enterprise-scale gap is measured. |
| [n8n](https://github.com/n8n-io/n8n) | License review | Workflow-platform comparison | Sustainable Use License restrictions and workflow overlap require an explicit decision. |
| [MinIO](https://github.com/minio/minio) | Excluded | Object-storage reference | Archived upstream and AGPLv3 are outside HAI's adoption bar. |

The API includes the source URL, verification date, activation requirements, safety disposition, and task recommendation rationale for every entry. This lets the frontend show the difference between a capable project, a configured integration, and an executable runtime.

## AutoGen compatibility profile

AutoGen is not HAI's execution foundation and is never selected for generic
coding or autonomous work. Its compatibility profile is limited to existing
AutoGen assets that need a controlled migration or interoperability plan.

The profile translates useful documented patterns into HAI-owned controls:

| AutoGen pattern | HAI control | Hard boundary |
| --- | --- | --- |
| Event-driven agent messages | Task events, workflow state, and audit records | HAI owns lifecycle and completion decisions. |
| Agent teams and delegation | Planner recommendations and approval-gated assignments | No upstream agent can self-authorize an action. |
| MCP Workbench | Trusted-only runtime registry with tool, folder, and network allowlists | A reviewed adapter and risk gate are required. |
| Code execution | Controlled runtime executor | The catalog exposes no generic executor. |

This is deliberately a protocol and control mapping, not an AutoGen SDK
integration. The upstream project warns that MCP servers must be trusted
because they may execute commands or expose sensitive data; HAI keeps that
boundary explicit.

## Next adapter work

1. Continue: a read-only check adapter that can report findings into HAI verification without repository writes.
2. OpenHands: a locally containerized adapter with per-workspace and per-network allowlists plus a durable stop handle.
3. CrewAI: an operator-hosted, local-model service adapter with a narrow task schema; HAI continues to own approvals and execution.
4. Aider: a review-first adapter that produces a patch proposal and validation evidence before any write is permitted.
5. SearXNG: an operator-managed local source-discovery endpoint. The built-in adapter is ready, but is disabled until its local instance, JSON format, search-engine policy, and AGPL deployment are explicitly reviewed.

Do not add a generic `run arbitrary agent` endpoint. That would collapse the safety boundary this catalog exists to preserve.

## MCP preflight profile

The built-in preflight mirrors the useful review stage of MCP Inspector without
embedding its broad proxy/process-launch capability. Enable it only for a
reviewed local Streamable HTTP server:

```dotenv
HAI_MCP_PREFLIGHT_ENABLED=true
HAI_MCP_PREFLIGHT_SERVERS=local-docs=http://host.docker.internal:3001/mcp
HAI_MCP_PREFLIGHT_TIMEOUT_SECONDS=5
```

`GET /api/v1/mcp-preflight/overview` reports configuration and the most recent
operator check. `POST /api/v1/mcp-preflight/local-docs/run` is admin-only and
performs `initialize`, `notifications/initialized`, and `tools/list`. It
accepts only `localhost`, loopback IPs, and `host.docker.internal`; rejects
URL credentials, query strings, external hosts, redirects, response bodies
over 1 MiB, and non-JSON responses. It returns a bounded tool name inventory
only. It does not execute a listed tool, retain schemas/descriptions, expose
headers, accept bearer tokens, or enable an HAI runtime.

## OR-Tools planning profile

The optional `optimization` Compose profile runs a private OR-Tools CP-SAT
service without a host port. HAI exposes the following owner-scoped routes:

- `GET /api/v1/planning-optimizer/status`
- `POST /api/v1/planning-optimizer/probe` (admin-only, read-only health check)
- `GET /api/v1/planning-optimizer/runs`
- `POST /api/v1/planning-optimizer/proposals`

The proposal request accepts at most 100 opaque IDs plus bounded integer minute
windows, durations, priorities, and optional fixed starts. HAI rejects remote
solver URLs, redirects, URL credentials, query strings, oversized request and
response bodies, unknown solver statuses, unexpected job IDs, altered
durations/priorities/windows, overlapping output, and incomplete job
accounting. It persists only the request digest and bounded proposal result.
No route applies a proposal to a workflow, task, calendar, source, file, tool,
or external account.

## Temporal durability profile

The optional `durability` Compose profile provisions a private local Temporal
server and separate PostgreSQL volume. HAI's only registered Temporal workflow
is an owner-scoped due-open-loop check that calls the existing HAI proposal
service. Its payload carries an opaque HAI run ID rather than owner identity or
source content. It cannot invoke a connector, runtime, browser, script, or
external action, and it does not alter HAI approval or completion state.

The owner-scoped routes are `GET /api/v1/temporal/status`,
`GET /api/v1/temporal/follow-up-runs`, admin-only
`POST /api/v1/temporal/worker/start`, and approval-gated
`POST /api/v1/temporal/follow-up-runs`. See
[Temporal durability](temporal-durability.md) for the full controls.

## OSS Insight curation scope

OSS Insight currently indexes more than one hundred repository collections.
HAI reviewed the collections that map to its real control planes: AI agent
frameworks, AI gateways, MCP clients, GraphRAG, vector stores, workflow
schedulers, LLM developer tools, and monitoring. The resulting entries are
recorded in the authenticated read-only API together with their exact source
collection. This is a curation snapshot, not a claim that all repositories in
the database are suitable, installed, or safe to run.

The full 102-collection screen and its per-category disposition are maintained
in [the OSS Insight screening ledger](ossinsight-screening-ledger.md).
