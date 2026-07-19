# Curated Agent Tool Catalog

HAI uses [e2b-dev/awesome-ai-agents](https://github.com/e2b-dev/awesome-ai-agents) and [OSS Insight Collections](https://ossinsight.io/collections) as discovery sources, not installation sources. A ranking or awesome-list is not a security review, a stable API contract, or permission to run third-party code on Robert's device.

## Operating rule

- The catalog is read-only at `GET /api/v1/brain-catalog/`.
- Listing a project never downloads, installs, enables, or executes it.
- Task planning can recommend a project capability, but does not select it as an executable tool.
- A project becomes executable only after a dedicated adapter has been reviewed, configured, health-checked, and routed through HAI's existing approval and audit controls.
- HAI remains the policy owner: an external framework cannot bypass the local-first policy, paid budget, source controls, folder allowlist, emergency stop, or approval queue.

## Curation snapshot: 2026-07-19

| Project | HAI disposition | Intended role | Why |
| --- | --- | --- | --- |
| [Continue](https://github.com/continuedev/continue) | Candidate | Source-controlled coding checks and review | Active Apache-2.0 project with a focused review/CI surface. Requires a check-only adapter before HAI uses it. |
| [OpenHands](https://github.com/OpenHands/OpenHands) | Candidate | Isolated development-agent runtime | Active project, but workspace and tool access are high-risk. It requires a local container, workspace/network allowlists, and an approval-gated adapter. |
| [CrewAI](https://github.com/crewAIInc/crewAI) | Candidate | Planning and multi-agent orchestration patterns | Active MIT framework. HAI retains the policy, audit, verification, and execution gates. |
| [Aider](https://github.com/Aider-AI/aider) | Candidate | Review-first coding assistance | Available Apache-2.0 coding tool. Any write-capable use needs a confined workspace and explicit approval. |
| [E2B](https://github.com/e2b-dev/E2B) | Reference only | External sandbox design | Its hosted execution model is not local-first and can involve external credentials/billing. Disabled unless separately approved. |
| [AutoGPT](https://github.com/Significant-Gravitas/AutoGPT) | License review | Workflow platform reference | The repository is active but includes differently licensed areas. HAI does not vendor or integrate it until a per-directory license review is complete. |
| [AutoGen](https://github.com/microsoft/autogen) | Compatibility only | Existing AutoGen workload migration, structured agent-event translation, and guarded MCP compatibility | The official project is maintenance mode. HAI does not install or execute AutoGen code, and a reviewed bridge plus approval is required. |
| [MetaGPT](https://github.com/FoundationAgents/MetaGPT) | Excluded | Architecture reference only | Still available, but its release and substantive push activity were older than the active candidates at curation time. |
| [LiteLLM](https://github.com/BerriAI/litellm) | Candidate | Local provider-gateway normalization | Must remain behind HAI's local-first routing, EUR 0 paid policy, endpoint allowlist, and approval review. |
| [pgvector](https://github.com/pgvector/pgvector) | Candidate | Local semantic retrieval inside HAI Postgres | Requires a reversible extension migration, local embeddings, retention policy, and backfill review. |
| [Temporal](https://github.com/temporalio/temporal) | Candidate | Durable retries, follow-ups, and long-running work | Requires a local service plus narrow Go worker. HAI retains all approval and completion gates. |
| [Prometheus](https://github.com/prometheus/prometheus) | Candidate | Source-backed service and queue metrics | Requires local scrape configuration; it does not replace HAI's action-oriented system status. |
| [Grafana](https://github.com/grafana/grafana) | Reference only | Optional advanced metrics visualization | Deferred until real Prometheus metrics justify a second dashboard. |
| [MCP Inspector](https://github.com/modelcontextprotocol/inspector) | Candidate | Pre-activation MCP server inspection | Operator-only test tool for allowlisted MCP servers; never execution approval. |
| [LangChain](https://github.com/langchain-ai/langchain) | Reference only | Retrieval and tool-orchestration patterns | HAI will not add a parallel agent stack without a documented gap. |
| [LlamaIndex](https://github.com/run-llama/llama_index) | Reference only | Connected-source and retrieval patterns | Deferred while HAI's native extraction, search, and pgvector path mature. |
| [Cognee](https://github.com/topoteretes/cognee) | Reference only | Evidence-graph and entity-linking patterns | Deferred until a graph-query need, provenance model, and retention plan are proven. |
| [Qdrant](https://github.com/qdrant/qdrant) | Reference only | Future dedicated vector-store option | Deferred to avoid a second vector store before pgvector has a measured limit. |

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

Do not add a generic `run arbitrary agent` endpoint. That would collapse the safety boundary this catalog exists to preserve.

## OSS Insight curation scope

OSS Insight currently indexes more than one hundred repository collections.
HAI reviewed the collections that map to its real control planes: AI agent
frameworks, AI gateways, MCP clients, GraphRAG, vector stores, workflow
schedulers, LLM developer tools, and monitoring. The resulting entries are
recorded in the authenticated read-only API together with their exact source
collection. This is a curation snapshot, not a claim that all repositories in
the database are suitable, installed, or safe to run.
