package braincatalog

import "strconv"

// CollectionDisposition is the explicit outcome of screening one OSS Insight
// collection. It is intentionally broader than Entry.Status: a collection is
// a discovery category, not a component HAI may execute.
type CollectionDisposition string

const (
	CollectionRepresented CollectionDisposition = "represented_in_catalog"
	CollectionCandidate   CollectionDisposition = "review_candidate"
	CollectionReference   CollectionDisposition = "reference_only"
	CollectionDeferred    CollectionDisposition = "not_adopted"
)

// CollectionScreening makes the complete OSS Insight collection pass
// inspectable in the authenticated back office. A screening result never
// changes a runtime, connector, credential, or approval state.
type CollectionScreening struct {
	Collection      string                `json:"collection"`
	Page            int                   `json:"page"`
	Disposition     CollectionDisposition `json:"disposition"`
	RelatedEntryIDs []string              `json:"relatedEntryIds,omitempty"`
	Rationale       string                `json:"rationale"`
	SourceURL       string                `json:"sourceUrl"`
}

type collectionDecision struct {
	disposition CollectionDisposition
	entries     []string
	rationale   string
}

var ossInsightCollectionPages = [][]string{
	{"ai-gateways", "Mocking and Stubbing tools", "Documentation Generator", "Anomaly Detection Software", "BaaS", "Model Context Protocol (MCP) Client", "AI Agent Frameworks", "AI Training Observability", "Project Management", "GraphRAG - Knowledge Graph based RAG", "Vector Database & Vector Store", "3D Physics Engines"},
	{"Browser Extension Frameworks", "Go Logging Libraries", "Go Web Frameworks", "Relational Database", "WebRTC", "LLM DevTools", "Reactive Monolith Frameworks", "Open Source Data Catalogs", "ML in Rust", "Programming Language of China", "Web Scanner", "Cloud Financial Management and Resource Optimization"},
	{"Networking for Games", "Stable Diffusion Ecosystem", "ChatGPT Apps", "Vector Search Engine", "LLM Tools", "ChatGPT Alternatives", "Zapier Alternatives", "Cpp CLI Parsing", "Business Management", "Ansible DevTools", "Approximate Nearest Neighbor Library", "Optimization Solvers"},
	{"X as Code", "Robotics", "Virtual Reality", "javascript ORM", "Javascript Build Tool", "Kubernetes Tooling", "Serverless Framework", "Slack Alternative", "iOS Framework", "Key Value Database", "MLOps Tools", "Workflow Scheduler"},
	{"Data Integration", "Password Manager", "Monitoring Tool", "Configuration Management Tools", "Golang ORM", "Security Tool", "Open Source Forum Software", "Computer Science Courses", "UI Framework and UIkit", "Terminal", "TUI Framework", "Modern Data Stack"},
	{"Go Database", "Rust Database", "Segment Alternative", "API tool for developer", "Hyperledger Fabric", "Hyperledger Besu", "Hyperledger Foundation", "WYSIWYG Editor", "PaaS", "Diagram as Code", "Identity Server", "Message and Streaming"},
	{"Web3", "Finance", "Cross Platform GUI Tool", "Remote Desktop Tool", "Testing Tools", "WebAssembly Runtime", "Distributed File Storage", "Programming Language", "Javascript Charting", "CICD", "React Framework", "APM Tool"},
	{"Chaos Engineering", "Search Engine", "Text Editor", "Javascript Game Engine", "Game Engine", "Headless CMS", "Artificial Intelligence", "Github Alternative", "Graph Database", "Time Series Database", "Business Intelligence", "Javascript Framework"},
	{"Web Framework", "Low Code Development Tool", "Google Analytics Alternative", "CSS Framework", "Open Source Database", "Static Site Generator"},
}

var ossInsightCollectionDecisions = map[string]collectionDecision{
	"ai-gateways":                          {CollectionRepresented, []string{"litellm"}, "A local-first provider gateway profile is already registered; every configured upstream still needs its own budget and approval review."},
	"Model Context Protocol (MCP) Client":  {CollectionRepresented, []string{"mcp-inspector", "opencode"}, "HAI has a local preflight profile and a separate review-first terminal-agent candidate; no MCP tool is enabled by discovery."},
	"AI Agent Frameworks":                  {CollectionCandidate, []string{"continue", "openhands", "crewai", "autogen"}, "Only reviewed, narrow adapters may use these orchestration patterns; HAI retains policy, audit, and final execution authority."},
	"GraphRAG - Knowledge Graph based RAG": {CollectionReference, []string{"langchain", "llamaindex", "cognee"}, "Useful retrieval patterns, but HAI keeps a single source-linked memory plane until a measured graph-retrieval gap exists."},
	"Vector Database & Vector Store":       {CollectionRepresented, []string{"pgvector", "qdrant"}, "The local Postgres pgvector profile is preferred; a second active vector database is held as a reference."},
	"LLM DevTools":                         {CollectionCandidate, []string{"cline", "aider", "continue", "langfuse", "promptfoo"}, "Coding and LLM-quality tools require workspace confinement, trace redaction, provider selection, and an explicit adapter review."},
	"Open Source Data Catalogs":            {CollectionReference, []string{"openmetadata"}, "Data-lineage patterns are retained for future scale; a second metadata authority is not adopted."},
	"ChatGPT Apps":                         {CollectionReference, []string{"autogpt"}, "Chat applications are not a substitute for HAI's controlled task and approval plane."},
	"Vector Search Engine":                 {CollectionReference, []string{"qdrant"}, "HAI avoids another active vector service while the existing local retrieval profile meets the demonstrated need."},
	"LLM Tools":                            {CollectionReference, []string{"langchain", "mem0", "promptfoo"}, "Useful implementation patterns remain visible without splitting HAI's memory, verification, or routing authority."},
	"ChatGPT Alternatives":                 {CollectionRepresented, []string{"llama-cpp"}, "The local inference profile supports operator-configured loopback model servers and remains disabled until live readiness is proven."},
	"Zapier Alternatives":                  {CollectionReference, []string{"activepieces", "n8n"}, "HAI does not silently introduce a second workflow control plane; licensing and ownership are separate reviews."},
	"Business Management":                  {CollectionCandidate, []string{"odoo"}, "A scoped, read-first business-system adapter can be reviewed, but HAI will not import or replace an ERP."},
	"Optimization Solvers":                 {CollectionRepresented, []string{"ortools"}, "The internal profile returns audited proposals only; it cannot apply schedule, calendar, filesystem, or network changes."},
	"Workflow Scheduler":                   {CollectionRepresented, []string{"temporal"}, "A durable local workflow profile is available only through a named, governed worker and existing approvals."},
	"Data Integration":                     {CollectionCandidate, []string{"airbyte"}, "A read-first connector bridge is a possible future adapter, but credentials, scopes, cursors, retention, and deletion must be reviewed first."},
	"Monitoring Tool":                      {CollectionRepresented, []string{"prometheus", "langfuse"}, "Operational metrics are Prometheus-first; LLM trace/evaluation is a separate, opt-in review candidate."},
	"Testing Tools":                        {CollectionRepresented, []string{"playwright"}, "The registered profile is read-only verification with an origin allowlist and no secret capture."},
	"WebAssembly Runtime":                  {CollectionRepresented, []string{"wasmtime"}, "Only approved content-addressed WASI modules can use the bounded local runner."},
	"Search Engine":                        {CollectionRepresented, []string{"searxng"}, "The local search profile is discovery-only: it never fetches pages, stores facts, or upgrades evidence verification."},
	"APM Tool":                             {CollectionReference, []string{"prometheus"}, "Prometheus remains the current local observability profile; broader APM is deferred pending a measured operational gap."},
	"Distributed File Storage":             {CollectionDeferred, []string{"minio"}, "No storage platform is adopted. The previous MinIO candidate remains excluded under its recorded upstream and licence review."},
	"Low Code Development Tool":            {CollectionDeferred, []string{"n8n"}, "Low-code platforms overlap HAI's workflow authority and remain held behind licensing and architecture review."},
	"Open Source Database":                 {CollectionDeferred, []string{"pgvector", "qdrant"}, "HAI retains its existing Postgres base; a database migration requires a demonstrated reliability or scale case."},
}

const defaultCollectionRationale = "No direct HAI adoption: this category is outside the current local-first control plane, already covered by the stack, or needs a demonstrated product gap before project-level review."

// CollectionScreenings returns a copy of the complete 102-category screen.
func CollectionScreenings() []CollectionScreening {
	result := make([]CollectionScreening, 0, 102)
	for pageIndex, collections := range ossInsightCollectionPages {
		page := pageIndex + 1
		for _, collection := range collections {
			decision, ok := ossInsightCollectionDecisions[collection]
			if !ok {
				decision = collectionDecision{disposition: CollectionDeferred, rationale: defaultCollectionRationale}
			}
			result = append(result, CollectionScreening{
				Collection: collection, Page: page, Disposition: decision.disposition,
				RelatedEntryIDs: append([]string(nil), decision.entries...), Rationale: decision.rationale,
				SourceURL: "https://ossinsight.io/collections?page=" + strconv.Itoa(page),
			})
		}
	}
	return result
}

// CollectionScreeningSummary keeps the UI honest about how much of the
// discovery source was screened and how many categories have a tangible HAI
// catalog relationship.
type CollectionScreeningSummary struct {
	Total       int                   `json:"total"`
	Represented int                   `json:"represented"`
	Candidates  int                   `json:"candidates"`
	Reference   int                   `json:"reference"`
	Deferred    int                   `json:"deferred"`
	Entries     []CollectionScreening `json:"entries"`
}

func OSSInsightScreening() CollectionScreeningSummary {
	entries := CollectionScreenings()
	summary := CollectionScreeningSummary{Total: len(entries), Entries: entries}
	for _, entry := range entries {
		switch entry.Disposition {
		case CollectionRepresented:
			summary.Represented++
		case CollectionCandidate:
			summary.Candidates++
		case CollectionReference:
			summary.Reference++
		default:
			summary.Deferred++
		}
	}
	return summary
}
