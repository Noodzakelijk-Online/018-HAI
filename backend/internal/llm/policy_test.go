package llm

import "testing"

func TestRouteSkipsWeakFreeModelForCodingTask(t *testing.T) {
	service := &Service{policy: defaultPolicy()}

	decision, err := service.Route(RouteRequest{Task: "Fix a Go API bug and explain the compile failure"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.SelectedModelID == "phi3:mini" {
		t.Fatalf("selected weak model %q for coding task", decision.SelectedModelID)
	}
	if decision.Tier != TierFree {
		t.Fatalf("selected tier %q, want %q", decision.Tier, TierFree)
	}
	if decision.Classification.TaskType != "coding" {
		t.Fatalf("classified task as %q, want coding", decision.Classification.TaskType)
	}
}

func TestRouteMovesPastPreviousModelAfterValidationFailure(t *testing.T) {
	service := &Service{policy: defaultPolicy()}
	validationPassed := false

	decision, err := service.Route(RouteRequest{
		Task:             "Summarize and classify these short notes",
		ValidationPassed: &validationPassed,
		PreviousModelID:   "phi3:mini",
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.SelectedModelID == "phi3:mini" {
		t.Fatalf("selected previous failed model")
	}
	if len(decision.Skipped) == 0 {
		t.Fatalf("expected skipped models to explain validation fallback")
	}
}

func TestPaidProviderDisabledByDefault(t *testing.T) {
	service := &Service{policy: defaultPolicy()}

	decision, err := service.Route(RouteRequest{
		Task:               "Handle a legal financial medical decision with verification",
		Difficulty:         5,
		RequiredReasoning:  "very_high",
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.RequiresApproval {
		t.Fatalf("default route should not select paid or expensive models")
	}
	for _, skipped := range decision.Skipped {
		if skipped.ProviderID == "paid-provider" && skipped.Reason == "" {
			t.Fatalf("paid provider skip should include a reason")
		}
	}
}

func TestLocalModelsAllowedPolicyIsEnforced(t *testing.T) {
	policy := defaultPolicy()
	policy.LocalModelsAllowed = false
	service := &Service{policy: policy}

	decision, err := service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.SelectedModelID != "" {
		t.Fatalf("selected %q even though local models are disabled and no free cloud provider is enabled", decision.SelectedModelID)
	}
	if len(service.Logs()) != 1 {
		t.Fatalf("expected no-selection decision to be logged")
	}
}
