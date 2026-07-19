// Package braincatalog owns HAI's curated view of external agent projects.
//
// It is deliberately a catalog, not a package manager: HAI must not download,
// install, or execute third-party agent code merely because it appears in an
// awesome-list. Entries become usable only through a reviewed adapter and the
// existing approval-gated runtime path.
package braincatalog

import "strings"

// Status describes HAI's adoption decision for an external project.
type Status string

const (
	StatusCandidate     Status = "candidate"
	StatusCompatibility Status = "compatibility_only"
	StatusReferenceOnly Status = "reference_only"
	StatusExcluded      Status = "excluded"
	StatusLicenseReview Status = "license_review"
)

// ControlMapping records how an upstream pattern is translated into an
// HAI-owned control. A recommendation therefore never delegates safety,
// policy, or execution authority to a third-party project.
type ControlMapping struct {
	SourcePattern string `json:"sourcePattern"`
	HAIControl    string `json:"haiControl"`
	Boundary      string `json:"boundary"`
}

// Entry is a transparent, source-backed integration decision. Verification is
// a curation snapshot rather than a claim that a runtime has been installed.
type Entry struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	UpstreamURL          string           `json:"upstreamUrl"`
	SourceCatalogURL     string           `json:"sourceCatalogUrl"`
	Status               Status           `json:"status"`
	Category             string           `json:"category"`
	IntegrationMode      string           `json:"integrationMode"`
	Capabilities         []string         `json:"capabilities"`
	RecommendedFor       []string         `json:"recommendedFor"`
	RequiresApproval     bool             `json:"requiresApproval"`
	LocalFirstCompatible bool             `json:"localFirstCompatible"`
	Activation           string           `json:"activation"`
	Rationale            string           `json:"rationale"`
	VerifiedAt           string           `json:"verifiedAt"`
	VerificationNote     string           `json:"verificationNote"`
	ControlMappings      []ControlMapping `json:"controlMappings,omitempty"`
}

// Recommendation makes a candidate visible to the planner without claiming it
// is installed or selected for execution.
type Recommendation struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Status           Status           `json:"status"`
	Role             string           `json:"role"`
	Rationale        string           `json:"rationale"`
	RequiresApproval bool             `json:"requiresApproval"`
	Activation       string           `json:"activation"`
	ControlMappings  []ControlMapping `json:"controlMappings,omitempty"`
}

const sourceCatalogURL = "https://github.com/e2b-dev/awesome-ai-agents"
const verifiedAt = "2026-07-19"

var entries = []Entry{
	{
		ID: "continue", Name: "Continue", UpstreamURL: "https://github.com/continuedev/continue", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "coding review", IntegrationMode: "operator-configured CLI or CI check",
		Capabilities: []string{"source-controlled coding checks", "PR review", "local CLI"}, RecommendedFor: []string{"coding", "repository review", "verification"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Install and configure Continue outside HAI, then add a reviewed check-only adapter. HAI will not install it or grant repository write access.",
		Rationale:  "Active Apache-2.0 project with a Windows CLI path and a narrow check/review role that complements HAI verification.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "openhands", Name: "OpenHands", UpstreamURL: "https://github.com/OpenHands/OpenHands", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "sandboxed development agent", IntegrationMode: "operator-configured container or service adapter",
		Capabilities: []string{"coding agent", "sandboxed workspace", "skills", "MCP integration"}, RecommendedFor: []string{"coding", "repository work", "sandboxed task execution"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Run OpenHands in an isolated local container, define a workspace and network allowlist, then complete a real approval-gated adapter review.",
		Rationale:  "Actively released development-agent project, but it can modify workspaces and invoke tools, so HAI keeps it disabled pending a specific adapter and operator verification.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "crewai", Name: "CrewAI", UpstreamURL: "https://github.com/crewAIInc/crewAI", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "multi-agent orchestration", IntegrationMode: "operator-hosted service adapter",
		Capabilities: []string{"role-based agents", "task orchestration", "flows"}, RecommendedFor: []string{"planning", "research", "multi-step workflows"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Host a reviewed CrewAI service with explicit tools and a local model route; bind only an allowlisted task API to HAI.",
		Rationale:  "Actively maintained MIT project that can contribute orchestration patterns, but HAI must own policy, approval, audit, and final execution decisions.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "aider", Name: "Aider", UpstreamURL: "https://github.com/Aider-AI/aider", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "interactive coding agent", IntegrationMode: "operator-configured workspace CLI adapter",
		Capabilities: []string{"repository editing", "git-aware code changes", "model-assisted coding"}, RecommendedFor: []string{"coding", "small repository changes"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Configure a confined workspace and an explicit model provider, then implement a read-only/review-first adapter before any write-enabled workflow is considered.",
		Rationale:  "Apache-2.0 project suitable for controlled code-assistance experiments; direct repository edits remain high-risk and are not enabled by this catalog.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository availability and maintenance activity checked on 2026-07-19.",
	},
	{
		ID: "e2b", Name: "E2B", UpstreamURL: "https://github.com/e2b-dev/E2B", SourceCatalogURL: sourceCatalogURL,
		Status: StatusReferenceOnly, Category: "remote execution sandbox", IntegrationMode: "external sandbox SDK/service",
		Capabilities: []string{"isolated code sandbox", "agent execution environment"}, RecommendedFor: []string{"sandbox design", "isolated execution"},
		RequiresApproval: true, LocalFirstCompatible: false,
		Activation: "Do not enable by default. A separate budget, data-egress, and API-key review is required before HAI may call an external E2B sandbox.",
		Rationale:  "Its SDK is active and useful as a sandbox reference, but a hosted sandbox conflicts with HAI's local-first and EUR 0 paid-default policy until explicitly approved.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "autogpt", Name: "AutoGPT", UpstreamURL: "https://github.com/Significant-Gravitas/AutoGPT", SourceCatalogURL: sourceCatalogURL,
		Status: StatusLicenseReview, Category: "agent platform", IntegrationMode: "separate platform deployment",
		Capabilities: []string{"agent workflows", "continuous agents", "platform UI"}, RecommendedFor: []string{"workflow patterns"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not vendor or integrate platform code until its per-directory licensing, hosting, and security model are reviewed.",
		Rationale:  "The repository is active, but it contains differently licensed areas; HAI will not import code under an unreviewed licensing assumption.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and licensing notice checked on 2026-07-19.",
	},
	{
		ID: "autogen", Name: "AutoGen", UpstreamURL: "https://github.com/microsoft/autogen", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCompatibility, Category: "legacy multi-agent compatibility", IntegrationMode: "operator-hosted bridge or protocol translation",
		Capabilities: []string{"event-driven agent messaging", "team and delegation patterns", "MCP workbench compatibility", "structured task events"}, RecommendedFor: []string{"existing AutoGen workloads", "MCP compatibility", "migration planning"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Use only behind a dedicated operator-hosted bridge with a narrow task schema. HAI does not install or execute AutoGen project code. New capability must use a HAI-native adapter or undergo a separate successor-framework review.",
		Rationale:  "AutoGen is maintenance mode but documents useful interoperability patterns. HAI maps those patterns into its native task, workflow, approval, and audit pipeline rather than using AutoGen as a new execution foundation.",
		VerifiedAt: verifiedAt, VerificationNote: "Official maintenance notice and MCP trusted-server warning checked on 2026-07-19.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "event-driven agent messages", HAIControl: "task events, workflow state, and immutable audit records", Boundary: "HAI owns task lifecycle and completion decisions"},
			{SourcePattern: "agent teams and delegation", HAIControl: "planner recommendations and approval-gated workflow assignments", Boundary: "no AutoGen agent can self-authorize an action"},
			{SourcePattern: "MCP Workbench", HAIControl: "agent runtime registry with trusted-server, tool, folder, and network allowlists", Boundary: "MCP tools require a reviewed adapter and the existing risk gate"},
			{SourcePattern: "code execution", HAIControl: "controlled runtime executor with workspace and approval constraints", Boundary: "no generic executor is exposed through this catalog"},
		},
	},
	{
		ID: "metagpt", Name: "MetaGPT", UpstreamURL: "https://github.com/FoundationAgents/MetaGPT", SourceCatalogURL: sourceCatalogURL,
		Status: StatusExcluded, Category: "multi-agent software workflow", IntegrationMode: "reference only",
		Capabilities: []string{"role-based software workflow"}, RecommendedFor: []string{"architecture reference"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Keep as a reference only until a new upstream review establishes current maintenance and an integration need.",
		Rationale:  "The repository remains available but its latest release and substantive push activity are materially older than the active candidates.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository activity checked on 2026-07-19.",
	},
}

// Entries returns a deep copy so callers cannot mutate the registry.
func Entries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		entry.Capabilities = append([]string(nil), entry.Capabilities...)
		entry.RecommendedFor = append([]string(nil), entry.RecommendedFor...)
		entry.ControlMappings = append([]ControlMapping(nil), entry.ControlMappings...)
		out = append(out, entry)
	}
	return out
}

func EntryByID(id string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == strings.ToLower(strings.TrimSpace(id)) {
			entry.Capabilities = append([]string(nil), entry.Capabilities...)
			entry.RecommendedFor = append([]string(nil), entry.RecommendedFor...)
			entry.ControlMappings = append([]ControlMapping(nil), entry.ControlMappings...)
			return entry, true
		}
	}
	return Entry{}, false
}

// Recommend maps a task to relevant external capabilities. It never marks a
// candidate as configured, selected for execution, or safe to invoke.
func Recommend(taskType, request string) []Recommendation {
	text := strings.ToLower(taskType + " " + request)
	ids := []string{}
	if containsAny(text, "code", "coding", "repository", "repo", "pull request", "test", "build", "bug", "commit") {
		ids = append(ids, "continue", "aider", "openhands")
	}
	if containsAny(text, "plan", "research", "workflow", "delegate", "multi-agent", "orchestr") {
		ids = append(ids, "crewai")
	}
	if containsAny(text, "sandbox", "isolate", "untrusted code") {
		ids = append(ids, "e2b")
	}
	if containsAny(text, "autogen", "agentchat", "magentic", "mcp workbench", "autogen migration") {
		ids = append(ids, "autogen")
	}
	seen := map[string]bool{}
	out := []Recommendation{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		entry, ok := EntryByID(id)
		if !ok {
			continue
		}
		role := "optional capability"
		if entry.Status == StatusCompatibility {
			role = "legacy compatibility only"
		} else if entry.Status != StatusCandidate {
			role = "reference or review only"
		}
		out = append(out, Recommendation{
			ID: id, Name: entry.Name, Status: entry.Status, Role: role,
			Rationale: entry.Rationale, RequiresApproval: entry.RequiresApproval, Activation: entry.Activation,
			ControlMappings: append([]ControlMapping(nil), entry.ControlMappings...),
		})
	}
	return out
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
