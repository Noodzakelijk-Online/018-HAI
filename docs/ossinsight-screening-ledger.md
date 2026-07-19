# OSS Insight Collection Screening Ledger

## Scope and decision rule

This is the complete collection-level screening pass over the 102 OSS Insight
collections visible on 2026-07-19. It is deliberately not a bulk-import list:
the collections contain hundreds of repositories, many of which overlap with
HAI, introduce a second control plane, require a separate security review, or
have no role in a local-first personal operations system.

An entry is added to the authenticated HAI brain catalog only when it has a
clear control-plane role and a bounded activation path. A catalog profile is
never an installed dependency, a grant of credentials, or permission to run
code. The source catalog is recorded at `GET /api/v1/brain-catalog/`.

## Shortlisted projects

| Project | Collection | Disposition | HAI role | Activation boundary |
| --- | --- | --- | --- | --- |
| LiteLLM | ai-gateways | Integrated profile | Keyed local provider gateway | Loopback-only profile; manual approval required because proxy upstream billing is not inferable. |
| llama.cpp | ChatGPT Alternatives | Candidate | Local GGUF inference | Loopback endpoint, model provenance, and health review required. |
| pgvector | Vector Database & Vector Store | Integrated, opt-in | Local semantic retrieval in existing Postgres | Pinned Postgres 17 pgvector image; local-only embedding endpoint; owner-scoped SQL query; keyword fallback. |
| Temporal | Workflow Scheduler | Candidate | Durable retries and follow-ups | One named local worker at a time; HAI owns approvals. |
| Prometheus | Monitoring Tool | Integrated profile | Authenticated HTTP request telemetry | Opt-in token-protected exporter; local collector and retention configuration remain operator-managed. |
| MCP Inspector | Model Context Protocol (MCP) Client | Integrated profile | Local-only MCP preflight | HAI-owned initialize + tools/list review of configured local Streamable HTTP servers; no process spawn or tool execution. |
| Playwright | Testing Tools | Candidate | Browser workflow verification | Named flows, origin allowlist, no secret capture, and approval gates. |
| Wasmtime | WebAssembly Runtime | Candidate | Bounded WASI helper execution | Reviewed content-addressed modules, no inherited network, explicit capabilities. |
| OR-Tools | Optimization Solvers | Integrated, opt-in | Internal deterministic CP-SAT schedule proposals | Bounded opaque task inputs only; returns audited suggestions and deferred work without workflow, calendar, filesystem, tool, or external-network apply capability. |
| Continue, OpenHands, CrewAI, Aider | AI Agent Frameworks / LLM DevTools / MCP | Candidate | Reviewed coding and orchestration profiles | Existing catalog controls apply; no generic agent-execution endpoint. |
| AutoGen | AI Agent Frameworks | Compatibility only | Migration and protocol translation | Dedicated bridge and approval required; no new foundation work. |
| Activepieces | Zapier Alternatives | Reference only | Connector and workflow-pattern research | No second automation control plane by default. |
| Mem0 | LLM Tools | Reference only | Memory-consolidation reference | HAI remains the sole memory/provenance authority. |
| OpenMetadata | Open Source Data Catalogs | Reference only | Data lineage and governance reference | Too large for current local-first source registry. |
| LangChain, LlamaIndex, Cognee, Qdrant, Grafana | Agent / GraphRAG / Vector / Monitoring | Reference only | Pattern or future scale option | Revisit only after a measured native gap. |
| n8n | Zapier Alternatives | License review | Workflow-platform comparison | Sustainable Use License and architecture overlap need a decision first. |
| MinIO | Distributed File Storage | Excluded | Storage reference only | Archived upstream and AGPLv3 do not meet the current adoption bar. |

## Complete collection screen

Each listed collection was classified by its suitability for HAI's thinking,
memory, operations, execution, verification, or governance planes. “No direct
adoption” means the collection is either out of scope, already covered by the
existing stack, too broad to introduce without a demonstrated gap, or requires
a separate project-level review.

| Page | Collections screened | Outcome |
| --- | --- | --- |
| 1 | ai-gateways; Mocking and Stubbing tools; Documentation Generator; Anomaly Detection Software; BaaS; Model Context Protocol (MCP) Client; AI Agent Frameworks; AI Training Observability; Project Management; GraphRAG - Knowledge Graph based RAG; Vector Database & Vector Store; 3D Physics Engines | Added gateway, MCP, agent, retrieval profiles; project-management/GraphRAG remain reference patterns; test tooling is covered by the existing verification path; the rest has no direct adoption. |
| 2 | Browser Extension Frameworks; Go Logging Libraries; Go Web Frameworks; Relational Database; WebRTC; LLM DevTools; Reactive Monolith Frameworks; Open Source Data Catalogs; ML in Rust; Programming Language of China; Web Scanner; Cloud Financial Management and Resource Optimization | Added coding-tool and data-governance references. Browser extensions, web scanners, and framework replacements are not adopted; financial-optimisation tools need a separate finance scope. |
| 3 | Networking for Games; Stable Diffusion Ecosystem; ChatGPT Apps; Vector Search Engine; LLM Tools; ChatGPT Alternatives; Zapier Alternatives; Cpp CLI Parsing; Business Management; Ansible DevTools; Approximate Nearest Neighbor Library; Optimization Solvers | Added llama.cpp, OR-Tools, workflow-platform dispositions, and memory reference. Image, business-suite, and infrastructure tooling remain separate optional projects. |
| 4 | X as Code; Robotics; Virtual Reality; javascript ORM; Javascript Build Tool; Kubernetes Tooling; Serverless Framework; Slack Alternative; iOS Framework; Key Value Database; MLOps Tools; Workflow Scheduler | Added Temporal. Infrastructure-as-code, Kubernetes, serverless, chat, mobile, and robotics are not HAI brain dependencies; Redis remains existing infrastructure rather than a new brain store. |
| 5 | Data Integration; Password Manager; Monitoring Tool; Configuration Management Tools; Golang ORM; Security Tool; Open Source Forum Software; Computer Science Courses; UI Framework and UIkit; Terminal; TUI Framework; Modern Data Stack | Added Prometheus/Grafana dispositions. Password/security tools are not imported due to credential and attack-surface concerns; data integration is handled through HAI connectors. |
| 6 | Go Database; Rust Database; Segment Alternative; API tool for developer; Hyperledger Fabric; Hyperledger Besu; Hyperledger Foundation; WYSIWYG Editor; PaaS; Diagram as Code; Identity Server; Message and Streaming | No direct adoption. HAI already has Postgres, bundled identity, and message infrastructure. Blockchain, PaaS, and editor frameworks do not power the HAI brain. |
| 7 | Web3; Finance; Cross Platform GUI Tool; Remote Desktop Tool; Testing Tools; WebAssembly Runtime; Distributed File Storage; Programming Language; Javascript Charting; CICD; React Framework; APM Tool | Added Playwright and Wasmtime. Remote desktop, Web3, finance, and CI/CD execution are deliberately out of scope; MinIO is excluded; APM remains Prometheus-first. |
| 8 | Chaos Engineering; Search Engine; Text Editor; Javascript Game Engine; Game Engine; Headless CMS; Artificial Intelligence; Github Alternative; Graph Database; Time Series Database; Business Intelligence; Javascript Framework | No direct adoption. Search/graph/time-series options stay deferred behind pgvector/Prometheus; AI libraries require a concrete model-serving or evaluation gap; development/UI ecosystems do not replace HAI's stack. |
| 9 | Web Framework; Low Code Development Tool; Google Analytics Alternative; CSS Framework; Open Source Database; Static Site Generator | No direct adoption. Low-code offerings overlap with HAI workflow ownership; analytics/CSS/framework/database alternatives cannot be introduced without a measured migration case. |

## Adoption gates

1. Upstream availability, license, maintenance, and project fit are checked before a catalog entry is added.
2. A candidate must have a concrete adapter contract, owner, local configuration, health check, audit event, failure behaviour, and rollback path before it can do work.
3. Recommendation is not activation: task planning exposes the option, while tool routing reports it as unavailable until its reviewed adapter exists.
4. Runtime-capable candidates stay behind workspace, network, tool, and approval allowlists. Browser automation, local execution, sending, publishing, deletion, credential use, and financial actions never inherit permission from the catalog.
5. HAI does not adopt another product as a second memory, automation, source, or policy authority without an explicit architecture decision.

## Re-screen triggers

Revisit an entry when its license or maintenance changes, a real HAI metric demonstrates a capability gap, or a proposed adapter has a complete safety and rollback design. Re-run this collection-level screen before treating newly added OSS Insight categories as adoption candidates.
