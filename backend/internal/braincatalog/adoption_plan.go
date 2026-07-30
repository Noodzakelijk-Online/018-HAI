package braincatalog

import (
	"sort"
	"strings"
)

// AdoptionPlan is a read-only implementation order derived from HAI's
// reviewed catalog. It is not a package manifest, an install plan, or an
// activation queue. Every item still needs the listed gates before it can
// affect a runtime, source, memory record, or external action.
type AdoptionPlan struct {
	Items   []AdoptionPlanItem `json:"items"`
	Message string             `json:"message"`
}

type AdoptionPlanItem struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Status            Status            `json:"status"`
	Category          string            `json:"category"`
	Planes            []CapabilityPlane `json:"planes"`
	Priority          int               `json:"priority"`
	PriorityReason    string            `json:"priorityReason"`
	IntegrationMode   string            `json:"integrationMode"`
	LocalFirst        bool              `json:"localFirst"`
	RequiresApproval  bool              `json:"requiresApproval"`
	Activation        string            `json:"activation"`
	RequiredGates     []string          `json:"requiredGates"`
	UpstreamURL       string            `json:"upstreamUrl"`
	SourceCatalogURL  string            `json:"sourceCatalogUrl"`
	SourceCollection  string            `json:"sourceCollection,omitempty"`
	VerificationNote  string            `json:"verificationNote"`
	RecommendedAction string            `json:"recommendedAction"`
}

// AdoptionPlanReport returns every catalog entry that has a possible HAI
// implementation path: integrated profiles that still need live readiness,
// review-first candidates, and compatibility bridges. Held, excluded, and
// licence-review entries are deliberately absent because they have no current
// implementation path.
func AdoptionPlanReport() AdoptionPlan {
	coverage := CapabilityPlaneCoverageReport()
	integratedByPlane := map[CapabilityPlane]int{}
	for _, item := range coverage {
		integratedByPlane[item.Plane] = item.Integrated
	}

	plan := AdoptionPlan{}
	for _, entry := range Entries() {
		if entry.Status != StatusIntegrated && entry.Status != StatusCandidate && entry.Status != StatusCompatibility {
			continue
		}
		planes := capabilityPlanesForEntry(entry.ID)
		priority, reason := adoptionPriority(entry, planes, integratedByPlane)
		plan.Items = append(plan.Items, AdoptionPlanItem{
			ID:                entry.ID,
			Name:              entry.Name,
			Status:            entry.Status,
			Category:          entry.Category,
			Planes:            planes,
			Priority:          priority,
			PriorityReason:    reason,
			IntegrationMode:   entry.IntegrationMode,
			LocalFirst:        entry.LocalFirstCompatible,
			RequiresApproval:  entry.RequiresApproval,
			Activation:        entry.Activation,
			RequiredGates:     requiredAdapterGates(entry),
			UpstreamURL:       entry.UpstreamURL,
			SourceCatalogURL:  entry.SourceCatalogURL,
			SourceCollection:  entry.SourceCollection,
			VerificationNote:  entry.VerificationNote,
			RecommendedAction: adoptionRecommendedAction(entry.Status),
		})
	}
	sort.SliceStable(plan.Items, func(i, j int) bool {
		if plan.Items[i].Priority != plan.Items[j].Priority {
			return plan.Items[i].Priority > plan.Items[j].Priority
		}
		if plan.Items[i].Status != plan.Items[j].Status {
			return plan.Items[i].Status == StatusCandidate
		}
		return plan.Items[i].Name < plan.Items[j].Name
	})
	plan.Message = "The roadmap is derived from HAI's reviewed catalog and capability-plane coverage. Priority orders investigation and profile-readiness work only; it does not install, configure, approve, or execute an upstream project."
	return plan
}

func capabilityPlanesForEntry(id string) []CapabilityPlane {
	planes := make([]CapabilityPlane, 0, 2)
	for _, plane := range capabilityPlaneOrder {
		for _, entryID := range capabilityPlaneDefinitions[plane].entryIDs {
			if entryID == id {
				planes = append(planes, plane)
				break
			}
		}
	}
	return planes
}

func adoptionPriority(entry Entry, planes []CapabilityPlane, integratedByPlane map[CapabilityPlane]int) (int, string) {
	priority := 0
	reasons := []string{}
	switch entry.Status {
	case StatusCandidate:
		priority = 70
		reasons = append(reasons, "review-first capability")
	case StatusCompatibility:
		priority = 55
		reasons = append(reasons, "compatibility bridge")
	case StatusIntegrated:
		priority = 45
		reasons = append(reasons, "existing profile needs local readiness")
	}
	if entry.LocalFirstCompatible {
		priority += 10
		reasons = append(reasons, "local-first compatible")
	}
	for _, plane := range planes {
		if integratedByPlane[plane] == 0 {
			priority += 8
			reasons = append(reasons, "no integrated profile in "+string(plane))
		}
	}
	if priority > 100 {
		priority = 100
	}
	return priority, strings.Join(reasons, "; ")
}

func adoptionRecommendedAction(status Status) string {
	switch status {
	case StatusIntegrated:
		return "Open the existing HAI profile, configure only a local approved endpoint or worker, then run its live health check."
	case StatusCompatibility:
		return "Define a narrow compatibility envelope and approval boundary before designing any bridge."
	default:
		return "Verify upstream metadata and create a manual adapter review; do not install or configure the project from this roadmap."
	}
}
