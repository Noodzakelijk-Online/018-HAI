package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		PreviousModelID:  "phi3:mini",
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
		Task:              "Handle a legal financial medical decision with verification",
		Difficulty:        5,
		RequiredReasoning: "very_high",
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

func TestGenerateCallsOllamaEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("path = %s, want /api/generate", r.URL.Path)
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["model"] != "phi3:mini" {
			t.Fatalf("model = %v, want phi3:mini", request["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "grounded draft"})
	}))
	defer server.Close()

	policy := defaultPolicy()
	policy.Providers[0].EndpointURL = server.URL
	service := &Service{policy: policy}

	result, err := service.Generate(GenerateRequest{
		Task: "Summarize this short note",
		RouteDecision: &RouteDecision{
			SelectedProviderID: "ollama",
			SelectedModelID:    "phi3:mini",
			SelectedModelName:  "Phi small local",
			Tier:               TierFree,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Output != "grounded draft" {
		t.Fatalf("output = %q, want grounded draft", result.Output)
	}
}

func TestGenerateCallsOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "local chat draft"}},
			},
		})
	}))
	defer server.Close()

	policy := defaultPolicy()
	policy.Providers[1].EndpointURL = server.URL
	service := &Service{policy: policy}

	result, err := service.Generate(GenerateRequest{
		Task: "Plan the work",
		RouteDecision: &RouteDecision{
			SelectedProviderID: "lm-studio",
			SelectedModelID:    "openai-compatible-local",
			SelectedModelName:  "OpenAI-compatible local endpoint",
			Tier:               TierFree,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Output != "local chat draft" {
		t.Fatalf("output = %q, want local chat draft", result.Output)
	}
}

func TestGenerateBlocksPaidWithoutApproval(t *testing.T) {
	policy := defaultPolicy()
	policy.Providers[3].Enabled = true
	policy.Providers[3].EndpointURL = "http://example.invalid"
	policy.PaidCallsAllowed = true
	policy.DailyPaidBudgetEUR = 1
	service := &Service{policy: policy}

	result, err := service.Generate(GenerateRequest{
		Task: "Handle a difficult verification task",
		RouteDecision: &RouteDecision{
			SelectedProviderID: "paid-provider",
			SelectedModelID:    "paid-high-capability",
			SelectedModelName:  "Paid high capability model",
			Tier:               TierExpensive,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
}
