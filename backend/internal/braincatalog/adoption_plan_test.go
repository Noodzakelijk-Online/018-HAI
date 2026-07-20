package braincatalog

import "testing"

func TestAdoptionPlanIncludesEveryImplementableCatalogEntryAndNoHeldEntries(t *testing.T) {
	plan := AdoptionPlanReport()
	if plan.Message == "" || len(plan.Items) == 0 {
		t.Fatalf("adoption plan must explain its bounded purpose: %#v", plan)
	}
	items := map[string]AdoptionPlanItem{}
	for _, item := range plan.Items {
		if item.ID == "" || item.Name == "" || item.Priority == 0 || len(item.Planes) == 0 || item.RecommendedAction == "" || len(item.RequiredGates) == 0 {
			t.Fatalf("adoption item lacks inspectable review context: %#v", item)
		}
		if item.Status != StatusCandidate && item.Status != StatusCompatibility && item.Status != StatusIntegrated {
			t.Fatalf("held project leaked into adoption plan: %#v", item)
		}
		items[item.ID] = item
	}
	for _, entry := range Entries() {
		shouldAppear := entry.Status == StatusCandidate || entry.Status == StatusCompatibility || entry.Status == StatusIntegrated
		_, appears := items[entry.ID]
		if appears != shouldAppear {
			t.Fatalf("entry %s appearance = %t, want %t", entry.ID, appears, shouldAppear)
		}
	}
}

func TestAdoptionPlanPrioritizesLocalFirstCoverageGapsWithoutClaimingActivation(t *testing.T) {
	plan := AdoptionPlanReport()
	var cloudQuery AdoptionPlanItem
	for _, item := range plan.Items {
		if item.ID == "cloudquery" {
			cloudQuery = item
			break
		}
	}
	if cloudQuery.ID == "" || cloudQuery.Priority < 55 || !cloudQuery.LocalFirst || cloudQuery.Status != StatusIntegrated {
		t.Fatalf("CloudQuery must be visible as an opt-in local summary adapter: %#v", cloudQuery)
	}
	if cloudQuery.RecommendedAction == "" || cloudQuery.RequiredGates[0] == "" {
		t.Fatalf("roadmap must preserve manual review gates: %#v", cloudQuery)
	}
}
