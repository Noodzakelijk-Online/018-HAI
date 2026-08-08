package runtimelab

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FeatureDisposition records the explicit result of reviewing an upstream
// feature. A reviewed feature is never silently dropped or called integrated
// merely because its upstream project is reachable.
type FeatureDisposition string

const (
	DispositionIntegratedDirectly          FeatureDisposition = "integrated_directly"
	DispositionAdaptedForHAI               FeatureDisposition = "adapted_for_hai"
	DispositionHAINative                   FeatureDisposition = "hai_native_reimplementation"
	DispositionAlreadyPresent              FeatureDisposition = "already_present"
	DispositionConsolidated                FeatureDisposition = "consolidated_existing"
	DispositionConstrainedUnsafe           FeatureDisposition = "constrained_unsafe"
	DispositionExcludedIrrelevant          FeatureDisposition = "excluded_irrelevant"
	DispositionExcludedIncompatibleLicense FeatureDisposition = "excluded_incompatible_license"
	DispositionDeferred                    FeatureDisposition = "deferred"
	DispositionBlockedExternal             FeatureDisposition = "blocked_external"
)

// RuntimeFeature is one source-reviewed upstream feature group and its exact
// HAI disposition. Feature groups are used deliberately: the coverageAreas
// field makes the required analysis surface machine-checkable without copying
// an upstream repository or pretending every internal file is a product feature.
type RuntimeFeature struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Purpose              string             `json:"purpose"`
	Behavior             string             `json:"behavior"`
	CoverageAreas        []string           `json:"coverageAreas"`
	Dependencies         []string           `json:"dependencies"`
	License              string             `json:"license"`
	SecurityImplications string             `json:"securityImplications"`
	HAIEquivalent        string             `json:"haiEquivalent,omitempty"`
	IntegrationApproach  string             `json:"integrationApproach"`
	Disposition          FeatureDisposition `json:"disposition"`
	ImplementationStatus string             `json:"implementationStatus"`
	TestStatus           string             `json:"testStatus"`
	DocumentationStatus  string             `json:"documentationStatus"`
	ExclusionReason      string             `json:"exclusionReason,omitempty"`
	BacklogPriority      string             `json:"backlogPriority,omitempty"`
	Requirements         []string           `json:"requirements,omitempty"`
	RecommendedPath      string             `json:"recommendedPath,omitempty"`
	SourceURLs           []string           `json:"sourceUrls"`
}

// RuntimeParityInventory is the reviewed, non-executing feature inventory for
// one upstream runtime. readinessCeiling is intentionally conservative.
type RuntimeParityInventory struct {
	RuntimeID          string           `json:"runtimeId"`
	Project            string           `json:"project"`
	RepositoryURL      string           `json:"repositoryUrl"`
	DefaultBranch      string           `json:"defaultBranch"`
	ReviewedRevision   string           `json:"reviewedRevision"`
	ReviewedRelease    string           `json:"reviewedRelease,omitempty"`
	ReviewedAt         time.Time        `json:"reviewedAt"`
	License            string           `json:"license"`
	LicensePolicy      string           `json:"licensePolicy"`
	ReadinessCeiling   string           `json:"readinessCeiling"`
	CanonicalAuthority string           `json:"canonicalAuthority"`
	Features           []RuntimeFeature `json:"features"`
}

// RuntimeParityOverview gives the dashboard auditable counts and the complete
// required-analysis taxonomy.
type RuntimeParityOverview struct {
	RequiredCoverageAreas []string                 `json:"requiredCoverageAreas"`
	Inventories           []RuntimeParityInventory `json:"inventories"`
	DispositionCounts     map[string]int           `json:"dispositionCounts"`
	ImplementationCounts  map[string]int           `json:"implementationCounts"`
	GeneratedAt           time.Time                `json:"generatedAt"`
}

var requiredRuntimeCoverageAreas = []string{
	"agent_runtimes", "agent_definitions", "goal_handling", "planning", "replanning",
	"reasoning_patterns", "tool_use", "mcp", "skills", "plugins", "extensions", "memory",
	"context_management", "knowledge_retrieval", "multi_agent_orchestration", "delegation",
	"parallel_execution", "background_tasks", "scheduling", "events_and_triggers",
	"browser_interaction", "computer_interaction", "coding_capabilities", "shell_and_script_execution",
	"file_operations", "communication_integrations", "model_providers", "model_routing", "local_models",
	"cost_management", "permissions", "approval_systems", "sandboxing", "credential_handling", "recovery",
	"retry_behaviour", "checkpointing", "observability", "logs", "metrics", "user_interfaces",
	"configuration", "installation", "deployment", "updates", "testing", "documentation",
}

var validDispositions = map[FeatureDisposition]bool{
	DispositionIntegratedDirectly: true, DispositionAdaptedForHAI: true,
	DispositionHAINative: true, DispositionAlreadyPresent: true,
	DispositionConsolidated: true, DispositionConstrainedUnsafe: true,
	DispositionExcludedIrrelevant: true, DispositionExcludedIncompatibleLicense: true,
	DispositionDeferred: true, DispositionBlockedExternal: true,
}

func feature(
	id, name, purpose, behavior string,
	coverage []string,
	dependencies []string,
	license, security, equivalent, approach string,
	disposition FeatureDisposition,
	implementation, tests, docs string,
	sources []string,
) RuntimeFeature {
	return RuntimeFeature{
		ID: id, Name: name, Purpose: purpose, Behavior: behavior,
		CoverageAreas: coverage, Dependencies: dependencies, License: license,
		SecurityImplications: security, HAIEquivalent: equivalent,
		IntegrationApproach: approach, Disposition: disposition,
		ImplementationStatus: implementation, TestStatus: tests,
		DocumentationStatus: docs, SourceURLs: sources,
	}
}

func deferred(f RuntimeFeature, priority, path string, requirements ...string) RuntimeFeature {
	f.BacklogPriority = priority
	f.RecommendedPath = path
	f.Requirements = append([]string(nil), requirements...)
	return f
}

func excluded(f RuntimeFeature, reason string) RuntimeFeature {
	f.ExclusionReason = reason
	return f
}

func blocked(f RuntimeFeature, priority, path string, requirements ...string) RuntimeFeature {
	f.BacklogPriority = priority
	f.RecommendedPath = path
	f.Requirements = append([]string(nil), requirements...)
	return f
}

// RuntimeFeatureParity returns the immutable source-reviewed inventory. It has
// no network, install, runtime, or execution side effect.
func RuntimeFeatureParity(now time.Time) (RuntimeParityOverview, error) {
	inventories := reviewedRuntimeInventories()
	for _, inventory := range inventories {
		if err := validateRuntimeInventory(inventory); err != nil {
			return RuntimeParityOverview{}, err
		}
	}
	counts := map[string]int{}
	implementationCounts := map[string]int{}
	for _, inventory := range inventories {
		for _, item := range inventory.Features {
			counts[string(item.Disposition)]++
			implementationCounts[item.ImplementationStatus]++
		}
	}
	return RuntimeParityOverview{
		RequiredCoverageAreas: append([]string(nil), requiredRuntimeCoverageAreas...),
		Inventories:           inventories,
		DispositionCounts:     counts,
		ImplementationCounts:  implementationCounts,
		GeneratedAt:           now.UTC(),
	}, nil
}

func validateRuntimeInventory(inventory RuntimeParityInventory) error {
	if strings.TrimSpace(inventory.RuntimeID) == "" || strings.TrimSpace(inventory.Project) == "" ||
		strings.TrimSpace(inventory.RepositoryURL) == "" || strings.TrimSpace(inventory.ReviewedRevision) == "" ||
		strings.TrimSpace(inventory.License) == "" || inventory.ReadinessCeiling != "declared" {
		return fmt.Errorf("runtimelab: incomplete parity metadata for %q", inventory.RuntimeID)
	}
	seenIDs := map[string]bool{}
	covered := map[string]bool{}
	for _, item := range inventory.Features {
		if item.ID == "" || item.Name == "" || item.Purpose == "" || item.Behavior == "" ||
			item.SecurityImplications == "" || item.IntegrationApproach == "" ||
			item.ImplementationStatus == "" || item.TestStatus == "" || item.DocumentationStatus == "" ||
			len(item.CoverageAreas) == 0 || len(item.SourceURLs) == 0 {
			return fmt.Errorf("runtimelab: incomplete feature %q for %s", item.ID, inventory.RuntimeID)
		}
		if seenIDs[item.ID] {
			return fmt.Errorf("runtimelab: duplicate feature %q for %s", item.ID, inventory.RuntimeID)
		}
		seenIDs[item.ID] = true
		if !validDispositions[item.Disposition] {
			return fmt.Errorf("runtimelab: invalid disposition %q", item.Disposition)
		}
		for _, area := range item.CoverageAreas {
			covered[area] = true
		}
		if item.Disposition == DispositionDeferred || item.Disposition == DispositionBlockedExternal {
			if item.BacklogPriority == "" || len(item.Requirements) == 0 || item.RecommendedPath == "" {
				return fmt.Errorf("runtimelab: %s feature %q has no actionable backlog", item.Disposition, item.ID)
			}
		}
		if item.Disposition == DispositionExcludedIrrelevant || item.Disposition == DispositionExcludedIncompatibleLicense || item.Disposition == DispositionConstrainedUnsafe {
			if item.ExclusionReason == "" {
				return fmt.Errorf("runtimelab: excluded/constrained feature %q has no reason", item.ID)
			}
		}
	}
	missing := make([]string, 0)
	for _, area := range requiredRuntimeCoverageAreas {
		if !covered[area] {
			missing = append(missing, area)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("runtimelab: %s parity inventory is missing coverage: %s", inventory.RuntimeID, strings.Join(missing, ", "))
	}
	return nil
}
