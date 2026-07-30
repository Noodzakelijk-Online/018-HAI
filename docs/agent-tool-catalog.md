# Curated Agent Tool Catalog

HAI uses [e2b-dev/awesome-ai-agents](https://github.com/e2b-dev/awesome-ai-agents) and [OSS Insight Collections](https://ossinsight.io/collections) as discovery sources, not installation sources. A ranking or awesome-list is not a security review, a stable API contract, or permission to run third-party code on Robert's device.

## Operating rule

- The catalog is read-only at `GET /api/v1/brain-catalog/`.
- Listing a project never downloads, installs, enables, or executes it.
- The back-office **Start review** action creates a normal, owner-scoped HAI pursuit with the catalog provenance and adapter-review gates; it does not activate the project.
- OSS Insight discovery scans every repository row returned for HAI's eligible categories (currently 33 candidate or represented categories). It labels the upstream result as a ranked collection response, never as an exhaustive GitHub inventory; the public endpoint currently returns a fixed 20-row list and does not honor ordinary paging parameters.
- An owner-admin can run `POST /api/v1/brain-catalog/:id/revalidate` to retrieve bounded public GitHub metadata for one fixed catalog entry. The recheck never fetches source code or changes an adoption decision. It records GitHub's canonical repository identity so a rename or transfer is visible, but does not rewrite the configured upstream automatically.
- The optional local SearXNG profile is a discovery adapter, not an answer engine. `GET /api/v1/research/status` shows its configuration, admin-only `POST /api/v1/research/probe` checks only its configured local `/healthz` endpoint, and `POST /api/v1/research/search` returns bounded, unverified source candidates only when the owner has enabled a local endpoint. **Grounded Answers** can also explicitly request a bounded public-source candidate preview in non-action modes. It never fetches a candidate page or persists/uses its snippet as evidence; an operator must attach one deliberately and re-run verification before it can influence claims or memory. The probe does not validate JSON output, engine policy, external-source behavior, provenance, or evidence quality.
- The optional local RAGFlow bridge is a fixed-dataset candidate-evidence adapter, not HAI memory or an agent runtime. `GET /api/v1/ragflow/status` reports non-secret configuration, `POST /api/v1/ragflow/probe` checks endpoint and reported dependency health, and `POST /api/v1/ragflow/retrieve` can query only configured local dataset IDs and returns only chunks with stable provenance IDs. **Grounded Answers** can retrieve candidates manually or explicitly request a bounded candidate-preview pass while planning a non-action answer. The Task Blueprint can also request at most three local candidates with an explicit per-plan opt-in. Those chunks may be shown to the draft model only with an `UNVERIFIED RAGFLOW CANDIDATE` instruction, but remain a separate non-persisted response type: they cannot support claims, update memory, create workflow work, authorize an action, or complete a task until independently grounded and re-verified. The bridge cannot ingest, delete, call an agent/MCP/code-executor, or update HAI state automatically.
- The optional local AnythingLLM bridge is a fixed-workspace candidate-evidence adapter, not HAI memory, chat, or an agent runtime. `GET /api/v1/anythingllm/status` exposes non-secret configuration, `POST /api/v1/anythingllm/probe` checks only authenticated access to the fixed workspaces, and `POST /api/v1/anythingllm/retrieve` calls only the upstream workspace vector-search endpoint. It cannot chat, send attachments, read history, ingest/delete documents, change workspace settings, or trigger tools. Retrieval remains disabled until the operator explicitly confirms that the configured workspace embeddings are local.
- The optional CloudQuery source adapter consumes only a fixed, local, operator-produced JSONL sync summary. It never runs CloudQuery or reads CloudQuery credentials, configuration, plugin data, raw source data, or destination data. HAI accepts completed newline-terminated rows under a read-only mounted folder, keeps a bounded incremental cursor, and routes the resulting health summaries through normal provenance, review, workflow, and deletion controls.
- The optional OpenSpec source adapter is a selected-folder, read-only planning-artifact reader. It groups only active `openspec/changes` proposal, design, task, and specification Markdown into one source-linked bundle per change. It skips archived changes and all code outside that tree; it never installs/runs OpenSpec, writes files, commits, creates branches/pull requests, or authorizes execution.
- The optional `project-instructions` source adapter reads only root `AGENTS.md` and `CLAUDE.md` from one selected local project folder. Both records are marked untrusted and source-linked. They are not automatically included in any model prompt, runtime command, task, workflow, or action; an operator must explicitly review and attach them as planning context. They cannot override HAI policy, approvals, tool allowlists, workspace limits, or emergency stop.
- The optional `fabric-patterns` source adapter reads only immediate-child `system.md` files from one selected locally installed Fabric patterns folder. It imports at most 24 bounded files as untrusted, provenance-linked manual-review records. It does not install or run Fabric, configure a provider, call a model, execute a pattern, or automatically include pattern text in a task, workflow, runtime command, model prompt, memory update, claim, or action. HAI policy, source grounding, approvals, routing, tool allowlists, workspace limits, and emergency stop always win.
- Task planning can recommend a project capability, but does not select it as an executable tool.
- A project becomes executable only after a dedicated adapter has been reviewed, configured, health-checked, and routed through HAI's existing approval and audit controls.
- HAI remains the policy owner: an external framework cannot bypass the local-first policy, paid budget, source controls, folder allowlist, emergency stop, or approval queue.

### whisper.cpp local transcription

The opt-in `local-transcription` Compose profile builds one pinned local
`whisper.cpp` runner. It is disabled unless `HAI_WHISPER_CPP_ENABLED=true`, the
runner profile is started, and a reviewed GGML model is manually placed under
`./whisper-models`. HAI does not download a model or start a microphone.

Create an owner-scoped `whisper-audio` connected source with `localOnly: true`
and an explicit subfolder of `./connected-sources`, for example
`voice-notes/2026-07`. `POST /api/v1/sources/:id/transcribe` accepts no body:
it can inspect only that registered subfolder, with its model, language, file,
size, and timeout limits set by the local operator. The internal runner mounts
the intake and model folders read-only, has no published host port, no network
attachment beyond its internal bridge, and returns text plus bounded model
metadata only. HAI turns that text into normal, owner-scoped, uncertain source
extractions with `audio://` provenance. Existing source correction, archive,
deletion, audit, workflow, and approval paths then apply.

This is not ambient recording, audio uploading, speech-driven action execution,
or evidence verification. A transcript may be wrong; it must be reviewed
against the original audio before it supports a consequential claim or action.

### Docling local document extraction

The opt-in `local-document-extraction` Compose profile builds one pinned
[Docling](https://github.com/docling-project/docling) runner for manual,
selected-folder intake. It is disabled unless `HAI_DOCLING_ENABLED=true`, the
profile is started, and the backend and runner share a dedicated internal token.
The runner reads only the registered relative subfolder under
`./connected-sources`; it has no published host port, mounts the intake folder
read-only, and has no external network attachment.

Create an owner-scoped `docling-documents` connected source with
`localOnly: true` and a specific subfolder, for example
`legal/vivare/evidence`. `POST /api/v1/sources/:id/extract-documents` accepts
no body and processes only DOCX, PPTX, XLSX, HTML, Markdown, and text files
from that source. It creates normal owner-scoped source extractions with stable
`document://` provenance and the existing correction, archive, deletion, audit,
workflow, and approval paths.

PDF extraction is deliberately off by default. It requires an operator to place
reviewed Docling artifacts under `./docling-artifacts` and set
`HAI_DOCLING_PDF_ENABLED=true`; HAI never downloads artifacts or uses remote
parsing services. OCR, table processing, external plugins, telemetry, automatic
folder scanning, and uploads are not exposed. Extracted text is uncertain
evidence, never a verified fact or an execution instruction.

## Curation snapshot: 2026-07-20

| Project | HAI disposition | Intended role | Why |
| --- | --- | --- | --- |
| [Continue](https://github.com/continuedev/continue) | Excluded | Former coding-assistant reference | Rechecked on 2026-07-21: the upstream README says the repository is read-only and no longer actively maintained after its final 2.0.0 release. HAI does not install, connect, or recommend it. |
| [Microsoft JARVIS (HuggingGPT)](https://github.com/microsoft/JARVIS) | Excluded | Historical multi-model orchestration research | Rechecked on 2026-07-21: its documented stack requires Ubuntu 16.04, Python 3.8, `text-davinci-003`, Hugging Face credentials, or up to 284 GB of model storage. HAI retains no package, service, credential, workspace, or model endpoint from it. |
| [Cline](https://github.com/cline/cline) | Candidate | Review-first interactive coding assistance | Active Apache-2.0 LLM-devtool. Any HAI bridge needs a confined workspace, explicit model provider, tool/network allowlists, and approval before write-capable work. |
| [OpenCode](https://github.com/anomalyco/opencode) | Candidate | Review-first terminal coding assistance | Active MIT MCP-client/terminal project. Any HAI bridge needs a confined workspace, explicit model provider, tool/network allowlists, and approval before write-capable work. |
| [OpenHands](https://github.com/OpenHands/OpenHands) | Integrated, health-only | External coding-agent readiness | Rechecked on 2026-07-21: active beta Agent Canvas, but GitHub reports `NOASSERTION` for its licence and the agent/agent-server source is moving to `OpenHands/software-agent-sdk`. HAI retains only an allowlisted local health probe; it cannot start agents, mount/read workspaces, select models, call tools, create automations, or execute tasks. |
| [CrewAI](https://github.com/crewAIInc/crewAI) | Integrated, opt-in | Local two-role review-only planning draft | One isolated local-model runner accepts a short owner request and up to eight criteria, uses fixed no-tool planner/reviewer roles, and returns a schema-checked draft. HAI retains validation, audit, approval, and all execution authority. |
| [Aider](https://github.com/Aider-AI/aider) | Candidate | Review-first coding assistance | Available Apache-2.0 coding tool. Any write-capable use needs a confined workspace and explicit approval. |
| [E2B](https://github.com/e2b-dev/E2B) | Reference only | External sandbox design | Its hosted execution model is not local-first and can involve external credentials/billing. Disabled unless separately approved. |
| [SWE-ReX](https://github.com/SWE-agent/SWE-ReX) | Reference only | Local sandbox-session architecture | Active MIT upstream checked 2026-07-21. Its server exposes arbitrary command, session, and file APIs, so HAI does not install, start, connect, or delegate to it. Revisit only after a measured gap in HAI's approved runtime registry, disposable mini-SWE worker, and WASI runner, with a separate workspace/network/command/approval design. |
| [AutoGPT](https://github.com/Significant-Gravitas/AutoGPT) | License review | Workflow platform reference | The repository is active but includes differently licensed areas. HAI does not vendor or integrate it until a per-directory license review is complete. |
| [AutoGen](https://github.com/microsoft/autogen) | Compatibility only | Existing AutoGen workload migration, structured agent-event translation, and guarded MCP compatibility | HAI has a transient event migration preview for bounded redacted workload samples. It does not install or execute AutoGen code; any real bridge still needs a separate approval. |
| [Microsoft Agent Framework](https://github.com/microsoft/agent-framework) | Integrated, opt-in | Local sequential planner/reviewer plus AutoGen migration planning | An isolated runner pins Agent Framework core 1.11.0 and compatible OpenAI client 1.10.1, accepts only a short task and up to eight criteria through a local OpenAI-compatible endpoint, and returns one schema-checked review draft. It has no tools, MCP, skills, memory, sessions, checkpoints, A2A, workflow host, source, filesystem, credential, approval, or execution authority. HAI retains policy, routing, audit, emergency stop, persistence, and completion ownership. |
| [MetaGPT](https://github.com/FoundationAgents/MetaGPT) | Excluded | Architecture reference only | Still available, but its release and substantive push activity were older than the active candidates at curation time. |
| [LiteLLM](https://github.com/BerriAI/litellm) | Integrated profile | Keyed loopback provider-gateway normalization | Requires explicit enablement, a local endpoint, model alias, virtual key, probe, and manual generation approval; HAI's EUR 0 policy remains authoritative. |
| [pgvector](https://github.com/pgvector/pgvector) | Integrated profile | Local semantic retrieval inside HAI Postgres | Opt-in `vector` extension plus local embeddings for source extraction and HAI's editable context memory. Source and memory ownership remain separate authorities; keyword retrieval remains the truthful fallback. |
| [Temporal](https://github.com/temporalio/temporal) | Integrated, opt-in | Durable governed follow-up checks | Local-only service plus one narrow Go worker. It creates HAI proposals only; HAI retains all approval and completion gates. |
| [Prometheus](https://github.com/prometheus/prometheus) | Integrated profile | Token-protected HTTP metrics export | Opt-in exporter with no raw-data labels; a local collector remains separately configured. |
| [Grafana](https://github.com/grafana/grafana) | Reference only | Optional advanced metrics visualization | Deferred until real Prometheus metrics justify a second dashboard. |
| [MCP Inspector](https://github.com/modelcontextprotocol/inspector) | Integrated profile | Local-only pre-activation MCP inspection | HAI performs only a bounded Streamable HTTP handshake and tool inventory for configured local endpoints; it never spawns a process or calls a tool. |
| [llama.cpp](https://github.com/ggml-org/llama.cpp) | Integrated, opt-in | Local GGUF model inference | Loopback-only model server through HAI's existing local-provider, provenance, live-probe, and approval policy. |
| [Playwright](https://github.com/microsoft/playwright) | Integrated, opt-in | Read-only local browser verification | Named allowlisted local routes only; no clicks, forms, downloads, retained state, public origins, sending, publishing, purchasing, or account changes. |
| [Docling](https://github.com/docling-project/docling) | Integrated, opt-in | Manual local structured-document intake | Direct upstream capability-gap review on 2026-07-22: HAI's internal runner reads one owner-scoped local-only selected folder and returns source-linked uncertain DOCX/PPTX/XLSX/HTML/Markdown/text extraction. PDF remains disabled until reviewed local artifacts are supplied. It has no uploads, automatic scans, cloud parsing, OCR/table processing, remote services, source authority, memory authority, approval authority, or execution path. |
| [SearXNG](https://github.com/searxng/searxng) | Integrated, opt-in | Local public-source discovery | HAI supplies an optional `research-discovery` Compose profile with no published host port. Only the backend can reach it through an internal network; SearXNG alone has outbound search-engine access. HAI uses bounded queries and returns source candidates, never fetches result pages or accepts snippets as verified facts. AGPL-3.0 requires a separate license and deployment review. |
| [Wasmtime](https://github.com/bytecodealliance/wasmtime) | Integrated, opt-in | Bounded local WASI helper runtime | Reviewed modules only, with no inherited network and explicit resource/capability limits; every run remains approval-gated. |
| [OR-Tools](https://github.com/google/or-tools) | Integrated profile | Internal deterministic CP-SAT schedule proposals | Opt-in `optimization` Compose profile accepts bounded opaque jobs and returns an audited proposal only; it has no workflow, calendar, filesystem, tool, or external-network apply endpoint. |
| [Odoo](https://github.com/odoo/odoo) | Integrated, opt-in | Read-only business-system source ingestion | One operator-owned Odoo JSON-2 endpoint, API key, optional database, and fixed model allowlist only. HAI calls only bounded `search_read`, preserves source links and cursors, and cannot write, invoke generic RPC methods, or expose the key. |
| [CloudQuery](https://github.com/cloudquery/cloudquery) | Integrated, opt-in | Local CloudQuery sync-summary intake | HAI reads only a fixed, operator-produced local `cloudquery sync --summary-location` JSONL file. It never starts CloudQuery, accesses its credentials/configuration, or receives raw source/destination data. Rows become bounded provenance-linked sync-health signals, not facts or execution authority. |
| [OpenSpec](https://github.com/Fission-AI/OpenSpec) | Integrated, opt-in | Local spec-driven planning artifact intake | HAI reads only active `openspec/changes` Markdown under an explicitly selected local project folder, grouping proposal/design/tasks/specs into a reviewable source bundle. It never invokes OpenSpec, reads repository code outside those files, writes changes, or authorizes coding execution. |
| [Claude Code project instructions](https://github.com/anthropics/claude-code) | Integrated, opt-in | Untrusted local project-guidance intake | HAI reads only root `AGENTS.md` and `CLAUDE.md` from one allowlisted local project as source-linked planning context. It does not install or run Claude Code, automatically prompt a model with the files, execute their instructions, or allow them to override policy, approval, tool, workspace, or emergency-stop controls. |
| [LangChain](https://github.com/langchain-ai/langchain) | Reference only | Retrieval and tool-orchestration patterns | HAI will not add a parallel agent stack without a documented gap. |
| [LlamaIndex](https://github.com/run-llama/llama_index) | Reference only | Connected-source and retrieval patterns | Deferred while HAI's native extraction, search, and pgvector path mature. |
| [Cognee](https://github.com/topoteretes/cognee) | Reference only | Evidence-graph and entity-linking patterns | Deferred until a graph-query need, provenance model, and retention plan are proven. |
| [Microsoft GraphRAG](https://github.com/microsoft/graphrag) | Architecture reference; HAI-native capability integrated | Source-linked candidate graph and timeline inspection | HAI now derives bounded entity co-occurrence and date-candidate views from owner-scoped source extractions. It does not install GraphRAG, run a graph database, treat a co-occurrence as a fact, or allow graph output to update memory, support claims, create workflows, or trigger actions. |
| [Qdrant](https://github.com/qdrant/qdrant) | Reference only | Future dedicated vector-store option | Deferred to avoid a second vector store before pgvector has a measured limit. |
| [Activepieces](https://github.com/activepieces/activepieces) | Reference only | Connector and workflow-pattern reference | Do not introduce a competing workflow, secrets, or approval control plane by default. |
| [Mem0](https://github.com/mem0ai/mem0) | Reference only | Memory-consolidation reference | HAI remains the sole personal-memory and provenance authority. |
| [Omega Memory](https://github.com/omega-memory/omega-memory) | Reference only | Local-first memory health and consolidation patterns | Not installed or connected. HAI remains the sole memory/provenance authority; the native owner-scoped `GET /api/v1/memory/health` review surfaces stale, ungrounded, dormant, and possible duplicate records without automatic consolidation. |
| [OpenMetadata](https://github.com/open-metadata/OpenMetadata) | Reference only | Source-governance reference | Defer its independent metadata control plane until an enterprise-scale gap is measured. |
| [n8n](https://github.com/n8n-io/n8n) | License review | Workflow-platform comparison | Sustainable Use License restrictions and workflow overlap require an explicit decision. |
| [MinIO](https://github.com/minio/minio) | Excluded | Object-storage reference | Archived upstream and AGPLv3 are outside HAI's adoption bar. |

## Newly reviewed capability candidates

The following candidates were reviewed against their public upstream records.
They appear in HAI's Brain Catalog, capability matcher, and adoption roadmap;
none is installed, configured, or executable through HAI.

| Project | HAI disposition | Intended role | Hard boundary |
| --- | --- | --- | --- |
| [Evidently](https://github.com/evidentlyai/evidently) | Integrated, opt-in | Internal offline quality report over synthetic/redacted fixtures | The optional `evaluation` profile runs a bounded local Evidently DataSummary report. HAI rejects detected PII/secrets before sending, returns metadata only, and cannot export fixture text, call providers, verify completion, alter routing/policy, or trigger actions. |
| [Whylogs](https://github.com/whylabs/whylogs) | Reference only | Compact data-quality profiling and constraint patterns | It is not installed or connected: its latest public package release is from 2024, it overlaps HAI's report-only Evidently bridge, and its documented anonymous analytics default would need disabling. Any future review must retain profiles locally and prevent data export, memory updates, routing changes, verification claims, or action authorization. |
| [Guardrails AI](https://github.com/guardrails-ai/guardrails) | Integrated, opt-in | Fixed-schema action-proposal validation | The optional `validation` profile accepts one bounded redacted JSON proposal, returns metadata only, and cannot invoke a model, fetch Hub validators, store data, approve, or execute. |
| [PydanticAI](https://github.com/pydantic/pydantic-ai) | Integrated, opt-in | Local schema-validated planning draft | The optional `typed-planning` profile pins `pydantic-ai-slim[openai]` 2.13.0 and accepts only a short task request plus success criteria for one operator-reviewed loopback model. It has no tools, MCP, web, file, source, memory, persistence, retries, provider selection, approval, or execution authority. The result remains a HAI-validated draft. |
| [A2A Protocol](https://github.com/a2aproject/A2A) | Integrated, opt-in | Local authenticated planning interoperability | HAI exposes a disabled-by-default A2A 1.0-shaped Agent Card and a narrow `SendMessage` JSON-RPC profile for one named local bearer-token peer. It requires `A2A-Version: 1.0`, accepts only standalone `ROLE_USER` text with a `messageId`, and returns one bounded non-executable proposal artifact. It is not a full A2A task-lifecycle server: task persistence/polling, source refresh, approval, execution, peer discovery, streaming, file input, memory/source disclosure, and tool invocation are unavailable. |
| [FastMCP](https://github.com/jlowin/fastmcp) | Integrated, opt-in | Authenticated local read-only HAI MCP bridge | The optional `mcp-bridge` profile pins FastMCP 3.4.4 and exposes only workflow aggregate, bounded actionable-summary, and owner-scoped GitHub repository sync-context tools to one local bearer-token client. It uses a second bridge token to read one configured owner's HAI state, binds to loopback only, and has no task, approval, execution, source-content, memory, policy, filesystem, process, or secret-returning tool. |
| [Promptfoo](https://github.com/promptfoo/promptfoo) | Integrated, opt-in | Fixed synthetic local safety regression | The optional `safety-evaluation` profile runs a shipped six-case local prompt-injection and high-risk-action suite against one configured OpenAI-compatible endpoint. Its health probe requires that model and suite configuration, the runner clears inherited proxy variables and runs unprivileged, and it returns aggregate pass/fail metadata only. It cannot accept real prompts, sources, models, providers, endpoints, commands, or alter HAI decisions. |
| [garak](https://github.com/NVIDIA/garak) | Integrated, opt-in | Fixed synthetic local prompt-injection regression | The optional `garak-evaluation` profile pins garak 0.15.1 and runs one four-case `PromptInject` probe against one configured local OpenAI-compatible endpoint. The runner clears inherited provider credentials and proxy variables, accepts no caller-selected target/model/probe/input, deletes raw JSONL/hit/HTML reports, and returns aggregate metadata only. It cannot target HAI, connected sources, accounts, runtimes, or actions, and the result cannot change HAI decisions. |
| [Gitleaks](https://github.com/gitleaks/gitleaks) | Integrated, opt-in | Read-only local aggregate secret-scan evidence | The optional `secret-scan` profile exposes a Gitleaks v8.30.1 runner only to the backend over an internal network. It scans one configured named read-only snapshot and returns only finding count, affected-file count, rule counts, duration, and a digest. Matched text, secret values, paths, lines, commits, authors, raw reports, and source files are never returned or retained. A finding can attach an owner-scoped, redacted `needs_review` security signal to an existing workflow, but cannot transition it, alter memory or approvals, choose routing, execute work, or prove completion. |
| [Gosec](https://github.com/securego/gosec) | Integrated, opt-in | Read-only local aggregate Go source security evidence | The optional `go-security-scan` profile exposes a Gosec v2.28.0 runner only to the backend over an internal network. It scans one configured named, vendored Go snapshot and forces module/network resolution off. It returns only finding total, severity/confidence counts, duration, and a digest. Source, paths, findings, rules, CWEs, raw reports, and remediation details are never returned or retained. A result is owner review context only; it cannot modify source, transition a workflow, alter memory or approvals, choose routing, execute remediation, or prove completion. |
| [Trivy](https://github.com/aquasecurity/trivy) | Integrated, opt-in | Read-only local aggregate configuration-security evidence | The optional `configuration-security-scan` profile exposes a Trivy v0.72.0 runner only to the backend over an internal network. It scans one configured named, read-only configuration snapshot with the offline configuration scanner only, with policy updates and proxy egress disabled. It returns only finding total, severity counts, duration, and a digest. Source, paths, findings, rules, policy details, raw reports, image/repository/cloud scanning, and remediation commands are never returned or retained. A result is owner review context only; it cannot modify infrastructure, transition a workflow, alter memory or approvals, choose routing, execute remediation, or prove completion. |
| [Syft](https://github.com/anchore/syft) | Integrated, opt-in | Read-only local aggregate SBOM inventory evidence | The optional `sbom-inventory` profile exposes a Syft v1.48.0 runner only to the backend over an internal network. It inventories one configured named read-only snapshot and returns only package total, package ecosystem counts, duration, and a digest. The SBOM, package names, versions, licences, PURLs, paths, and source files are never returned or retained. Inventory can attach an owner-scoped, redacted `needs_review` signal to an existing workflow, but cannot transition it, alter memory or approvals, choose routing, execute work, approve dependencies, or prove completion. |
| [Grype](https://github.com/anchore/grype) | Integrated, opt-in | Read-only local aggregate vulnerability evidence | The optional `vulnerability-scan` profile exposes a Grype v0.116.0 runner only to the backend over an internal network. It scans one configured named read-only snapshot against an operator-supplied, read-only local advisory database with Grype update checks and proxy egress disabled. It returns only total, severity counts, fix-availability count, duration, and a digest. CVEs, package names, versions, advisories, paths, raw reports, source files, and remediation commands are never returned or retained. A result is owner review context only; it cannot modify dependencies, transition a workflow, alter memory or approvals, choose routing, execute remediation, or prove completion. |
| [DeepEval](https://github.com/confident-ai/deepeval) | Integrated, opt-in | Fixed synthetic source-grounding regression | The optional `deepeval-evaluation` profile pins DeepEval 4.1.1 and evaluates only three shipped synthetic evidence/answer pairs with `FaithfulnessMetric` through one configured local OpenAI-compatible judge. It returns aggregate evaluator accuracy only; no real HAI answer, source, prompt, model output, metric reason, routing, verification, policy, memory, workflow, approval, or action is accessible. |
| [Langfuse](https://github.com/langfuse/langfuse) | Integrated, opt-in | Local aggregate control-plane observability | The local-only bridge checks self-hosted health and readiness, then an owner can explicitly export one fixed OTLP/HTTP JSON span containing only static aggregate control-plane metadata. It cannot export prompts, source text, workflow records, model data, tokens, files, or caller-selected content; traces cannot change HAI decisions. |
| [LiveKit Agents](https://github.com/livekit/agents) | Candidate | Explicitly opt-in real-time voice and multimodal intake | No microphone, call, MCP tool, or external contact is activated without session consent, configured local/self-hosted service, and HAI approval. |
| [mistral.rs](https://github.com/ericlbuehler/mistral.rs) | Integrated, opt-in | Loopback OpenAI-compatible local model serving and multimodal evaluation | HAI has an operator-configured, loopback-only `/v1/models` and `/v1/chat/completions` provider profile with live probing and the existing EUR 0 router. The upstream's UI, agent, shell, web, file, MCP, Skills, and code tools are not integrated. |
| [SGLang](https://github.com/sgl-project/sglang) | Integrated, opt-in | Loopback OpenAI-compatible local high-throughput inference | HAI has an operator-configured, loopback-only `/v1/models` and `/v1/chat/completions` provider profile with daily exact-model availability checks and the existing EUR 0 router. HAI does not start SGLang, pull images or weights, replace the configured model, or inherit any upstream tool surface. |
| [AG2](https://github.com/ag2ai/ag2) | Compatibility only | Existing AG2 / AutoGen-era workload migration and pattern review | It cannot become a second agent control plane. Any bridge must use a fixed schema and HAI-owned model policy, audit, approvals, workspace limits, and tool allowlist. |
| [RAGFlow](https://github.com/infiniflow/ragflow) | Integrated, opt-in | Complex document parsing, evidence-linked retrieval, and reranking | HAI has a disabled-by-default, local-only retrieval bridge with an explicit dataset allowlist. It remains an external retrieval index, not HAI memory or truth; its optional agent/code executor is disabled and any deployment first needs a measured gap, source allowlist, resource budget, provenance, and deletion review. |
| [MLflow](https://github.com/mlflow/mlflow) | Integrated, opt-in | Local evaluation-run evidence | HAI queries only recent runs from explicit local experiment IDs and returns only explicit metric keys. It never requests prompts, parameters, tags, datasets, artifacts, models, traces, or credentials and has no mutation or routing authority. Returned metrics are manual model-review context, not an automatic model-selection signal. |
| [Airbyte](https://github.com/airbytehq/airbyte) | Integrated, opt-in | Approved-workspace source and connection inventory | HAI's local-only `airbyte-inventory` connector reads a fixed one-page list of source and connection metadata from allowlisted workspaces. It excludes credentials, connector configuration, selected fields, records, sync results, and all mutation or sync-control actions. |
| [AnythingLLM](https://github.com/Mintplex-Labs/anything-llm) | Integrated, opt-in | Local workspace vector-search evidence retrieval | HAI has a disabled-by-default local bridge to the documented vector-search endpoint for fixed workspace slugs. It requires local-embedding confirmation and never calls AnythingLLM chat, history, attachments, agents, tools, or mutation APIs. Returned chunks remain manually selected, unverified evidence. |
| [Presidio](https://github.com/data-privacy-stack/presidio) | Integrated, opt-in | Local PII detection second-pass | The maintained project moved from the Microsoft GitHub namespace. HAI has a disabled-by-default local Analyzer bridge with fixed language/entity allowlists and bounded metadata-only results; it cannot anonymize, persist, replay, delete, or prove content safe. HAI's deterministic privacy controls remain authoritative for known secrets. |
| [Serena](https://github.com/oraios/serena) | Integrated, opt-in | Read-only semantic repository context | HAI exposes a disabled loopback-only bridge for one self-started project-pinned Serena endpoint. It calls only `find_symbol` with source bodies and hover data disabled, returns bounded symbol metadata, and never starts Serena, changes projects, or exposes shell, file, editing, memory, diagnostic, or generic MCP tools. |
| [Microsoft UFO](https://github.com/microsoft/UFO) | Reference only | Windows and multi-device execution architecture | It exposes GUI, UIA, Win32, WinCOM, and cross-device agent capabilities. HAI will not connect it to a Windows session, screen, device, provider, or tool surface without a separate execution-safety design. |
| [Goose](https://github.com/aaif-goose/goose) | Reference only | General-purpose local-agent and MCP interoperability patterns | Its historic `block/goose` slug now redirects here. Its desktop, CLI, API, provider, extension, and execution surfaces would create a second control plane, so it is not embedded, installed, or run by HAI. |
| [SWE-agent](https://github.com/SWE-agent/SWE-agent) | Reference only | Superseded coding-agent architecture | Its own upstream now recommends mini-SWE-agent. HAI will not add a legacy code-worker, repository mount, provider credential, or agent loop from SWE-agent. |
| [SWE-bench](https://github.com/SWE-bench/SWE-bench) | Excluded | Resource-intensive code-agent benchmark | Its official evaluation setup uses Docker and documents approximately 120 GB free storage, 16 GB RAM, and 8 CPU cores. HAI must not install its package, datasets, benchmark images, Docker socket, workspace mount, or cloud evaluation. The bounded mini-SWE profile remains the review-only local patch-proposal path. |
| [Prefect](https://github.com/PrefectHQ/prefect) | Reference only | Data workflow orchestration | Active Apache-2.0 upstream checked 2026-07-21. Its server, deployments, work pools, and scheduler would create a parallel workflow-control plane beside HAI's bounded Temporal layer, so no Prefect package, server, worker, cloud account, or credential is configured. |
| [Dagster](https://github.com/dagster-io/dagster) | Reference only | Data asset orchestration | Active Apache-2.0 upstream checked 2026-07-21. Its daemon, webserver, catalog, and integrations would overlap HAI's source provenance, audit, and task state, so no Dagster package, daemon, webserver, asset catalog, source exposure, or credential is configured. |
| [AgentOps](https://github.com/AgentOps-AI/agentops) | Reference only | Agent observability platform | Active MIT upstream checked 2026-07-21. HAI will not install its SDK, configure an API key, export traces, or start a service while native audit records and optional Prometheus, Langfuse, OpenLIT, and MLflow profiles remain sufficient. |
| [Prompt flow](https://github.com/microsoft/promptflow) | Reference only | LLM-flow lifecycle toolkit | Active MIT upstream checked 2026-07-21. Its flow, connection, telemetry, and deployment surfaces must not become a competing HAI provider, execution, or workflow control plane. |
| [mini-SWE-agent](https://github.com/SWE-agent/mini-swe-agent) | Integrated, opt-in | Disposable local patch proposal worker | The optional `mini-swe` profile pins mini-SWE-agent 2.4.5. HAI permits it only for an owner-scoped workflow already in `ready` with explicit approval and a named sanitized snapshot. The worker copies the read-only snapshot into temporary storage, has no host/Docker/Git/credential/public-network surface, returns only a complete bounded diff and digest for human review, persists metadata only, and attaches only the opaque digest plus a `needs_review` signal to the originating workflow. It cannot apply, commit, push, open a pull request, or satisfy a completion gate. A truncated response fails closed and is not retained as a patch artifact. |
| [OpenCode (opencode-ai legacy)](https://github.com/opencode-ai/opencode) | Excluded | Archived same-name terminal agent | This is a distinct archived project, not an alias for HAI's active `anomalyco/opencode` candidate. It cannot inherit that profile's review status or receive a workspace, model provider, MCP server, credential, or runtime adapter. |
| [Daytona](https://github.com/daytonaio/daytona) | Excluded | Unmaintained public sandbox upstream | The public repository states that core development moved private in June 2026. HAI must not install, connect, or recommend it as a runtime, sandbox, account integration, or execution adapter. |

### RAGFlow capacity gate

RAGFlow's own self-hosting guidance calls for at least 4 CPU cores, 16 GB RAM,
50 GB disk, Docker Compose, and gVisor when its optional code executor is
used. Its standard Compose HTTP service uses port `80`; operators must map it
to a dedicated port that does not collide with HAI before setting the bridge
base URL. HAI does not provision it automatically. The implemented retrieval
bridge is disabled until `HAI_RAGFLOW_ENABLED`, its API key, and at least one approved
dataset ID are configured. A local deployment review must
record its resource reservation, document folder/connector allowlist, model and
embedding endpoint, retention/deletion/export rules, and proof that its code
executor is disabled before the HAI adapter review can begin.

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

## Microsoft Agent Framework local planning profile

Microsoft now positions Agent Framework as AutoGen's successor. HAI records it
as a candidate for a future local, fixed-schema orchestration bridge, not as a
second control plane. The owner-authenticated
`POST /api/v1/autogen-compat/migration-plan` endpoint now turns a bounded,
redacted AutoGen-style event sample into a staged Microsoft Agent Framework
migration plan. The plan maps event ingress, provider middleware, tool intent,
checkpointing, and terminal events back onto HAI's own task, routing,
approval, runtime, verification, audit, and recovery controls.

The migration endpoint remains transient planning only: it does not install,
probe, start, contact, configure, or execute Microsoft Agent Framework or
create a task, approval, source, memory, workflow, audit record, or completion
claim. Its useful patterns are checkpointing, human-in-the-loop workflow steps,
provider-neutral middleware, and A2A/MCP interoperability.

The separate `agent-framework-planning` profile is the only actual framework
integration. It pins Agent Framework core 1.11.0 and compatible OpenAI client
1.10.1, connects only to one reviewed local OpenAI-compatible endpoint, and
runs two fixed no-tool roles to return one bounded planning proposal. The
runner has no HAI credentials or source access and no tools, MCP, skills,
memory, sessions, checkpoints, A2A, hosted-agent, workflow-host, retry, or
execution path. HAI validates the proposal and keeps all routing, policy,
approval, audit, emergency-stop, persistence, workflow, and completion
authority.

HAI rechecked the official upstream metadata on 2026-07-21: Microsoft Agent
Framework is active on its main branch, not archived, MIT licensed, and had a
push on 2026-07-20. This establishes maintenance availability only; it is not
an activation grant. AutoGen remains compatibility-only: its current GitHub
metadata reports CC-BY-4.0, and HAI does not import or run its code.

Any activation must be locally hosted, name explicit peers and allowed tools,
emit HAI-owned audit events, and hand every protected action back to HAI's
approval and verification layers. Cloud Foundry hosting, credential discovery,
framework-owned provider routing, and automatic peer/tool discovery are out of
scope for this profile.

## Next adapter work

1. Aider: a review-first adapter that produces a patch proposal and validation evidence before any write is permitted.

OpenHands is not in the immediate execution-adapter queue. Its current Agent
Canvas code transition, licence metadata, and explicit unsandboxed host-access
warning require a fresh, per-repository licence and sandbox review before HAI
could consider a task transport or workspace integration.

Do not add a generic `run arbitrary agent` endpoint. That would collapse the safety boundary this catalog exists to preserve.

## MCP preflight profile

The built-in preflight mirrors the useful review stage of MCP Inspector without
embedding its broad proxy/process-launch capability. Enable it only for a
reviewed local Streamable HTTP server:

```dotenv
HAI_MCP_PREFLIGHT_ENABLED=true
HAI_MCP_PREFLIGHT_SERVERS=local-docs@mcp-inspector=http://host.docker.internal:3001/mcp
HAI_MCP_PREFLIGHT_TIMEOUT_SECONDS=5
```

For a self-started Serena HTTP server, `HAI_SERENA_ENABLED=true`,
`HAI_SERENA_BASE_URL=http://host.docker.internal:9121/mcp`, and a stable
non-path `HAI_SERENA_PROJECT_ID` expose `GET /api/v1/serena/status`, an
owner-admin `POST /api/v1/serena/probe`, and owner `POST /api/v1/serena/symbols`.
The probe performs only the MCP handshake and `tools/list`; symbol lookup calls
only the reviewed `find_symbol` tool with source-body and hover data disabled.
HAI does not install or start Serena, activate/mount a repository, supply
credentials, or expose any other Serena tool.

`GET /api/v1/mcp-preflight/overview` reports configuration and the most recent
operator check. `POST /api/v1/mcp-preflight/local-docs/run` is admin-only and
performs `initialize`, `notifications/initialized`, and `tools/list`. It
requires each endpoint to name an eligible reviewed Brain Catalog MCP profile,
then accepts only `localhost`, loopback IPs, and `host.docker.internal`; rejects
URL credentials, query strings, external hosts, redirects, response bodies
over 1 MiB, non-JSON responses, unexpected response IDs, and protocol-version
downgrades. It returns a bounded tool name inventory only. It does not execute
a listed tool, retain schemas/descriptions, expose headers, accept bearer
tokens, or enable an HAI runtime.

### MCP Toolbox database-tool inspection profile

[MCP Toolbox](https://github.com/googleapis/mcp-toolbox) is integrated only as
an inspection profile for an owner-started, local Streamable HTTP endpoint. Its
former `googleapis/genai-toolbox` repository slug remains HAI's stable catalog
profile ID. Point HAI at one specifically reviewed toolset endpoint, not the
server root:

```dotenv
HAI_MCP_PREFLIGHT_ENABLED=true
HAI_MCP_PREFLIGHT_SERVERS=toolbox-review@google-genai-toolbox=http://host.docker.internal:5000/mcp/approved_readonly_toolset
```

HAI runs only `initialize`, `notifications/initialized`, and `tools/list`.
It does not read Toolbox configuration, receive database credentials, call a
tool, submit SQL, or accept a query from an agent. The preflight confirms only
that a local endpoint can declare a bounded tool inventory. A database query
bridge remains a separate future approval: it must pin a named read-only
toolset and provide template, parameter, row/time, audit-redaction, and
disconnect controls before any tool call is considered.

### GitHub MCP context profile

GitHub MCP is a **context-only compatibility profile**, not an HAI execution
adapter. Start it separately on the local machine with its own read-only mode
and a deliberately limited toolset, then configure only its local Streamable
HTTP endpoint in HAI:

```dotenv
HAI_MCP_PREFLIGHT_ENABLED=true
HAI_MCP_PREFLIGHT_SERVERS=github-context@github-mcp-server=http://host.docker.internal:3000/mcp
```

During preflight, HAI only performs the MCP handshake and `tools/list`; it
never sends a GitHub credential and never calls a GitHub MCP tool. For this
profile, HAI marks the endpoint ready only when every declared tool is in its
reviewed read-only repository-context allowlist: repository, commit, branch,
file, issue, pull-request, workflow-run, and search reads. A server that
declares a write or unknown tool remains blocked, even if its MCP handshake
succeeds. This verifies the declared inventory, not the server implementation;
the operator must still configure the upstream GitHub MCP server in read-only
mode. Existing HAI GitHub source synchronization remains the only supported
repository ingestion path. When the optional local FastMCP bridge is enabled,
its `hai_github_repository_context` tool can expose at most eight of the
configured owner's existing GitHub repository slugs together with HAI project
keys and sync freshness. It never returns an issue, pull request, commit, file,
source URI, raw source record, token, or write tool.

### Playwright MCP inspection profile

Playwright MCP is also a context-only compatibility profile. Its default core
toolset includes browser interaction, navigation, file upload, and unsafe code
execution, so HAI blocks that inventory. A configured local endpoint passes
preflight only when it declares HAI's compact inspection-only browser set:
accessibility snapshots, snapshot search, console messages, resolved
configuration, and the current network-route list.

HAI does not start Playwright MCP, attach to an existing browser, send browser
credentials, navigate, read cookies/storage, take screenshots, or call a
Playwright MCP tool. Browser verification remains HAI's separate named-route,
read-only worker. Any interactive Playwright MCP use requires a distinct
execution-safety design and approval flow.

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

OSS Insight's public collection API snapshot currently contains 138 repository
collections, while its older web grid presents 102 categories.
HAI reviewed the collections that map to its real control planes: AI agent
frameworks, AI gateways, MCP clients, GraphRAG, vector stores, workflow
schedulers, LLM developer tools, and monitoring. The resulting entries are
recorded in the authenticated read-only API together with their exact source
collection. This is a curation snapshot, not a claim that all repositories in
the database are suitable, installed, or safe to run.

The full 138-collection screen and its per-category disposition are maintained
in [the OSS Insight screening ledger](ossinsight-screening-ledger.md).
