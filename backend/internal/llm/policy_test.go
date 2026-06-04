package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteSkipsWeakFreeModelForCodingTask(t *testing.T) {
	service := &Service{policy: testPolicyWithLocalEndpoints()}

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
	service := &Service{policy: testPolicyWithLocalEndpoints()}
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
	service := &Service{policy: testPolicyWithLocalEndpoints()}

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

func TestRouteSkipsProvidersWithoutConfiguredEndpoints(t *testing.T) {
	service := &Service{policy: testPolicyWithoutEndpoints()}

	decision, err := service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.SelectedModelID != "" {
		t.Fatalf("selected %q even though no provider endpoint is configured", decision.SelectedModelID)
	}
	if len(decision.Skipped) == 0 {
		t.Fatalf("expected skipped models to explain missing provider endpoints")
	}
}

func TestPolicyMarksUnconfiguredProviders(t *testing.T) {
	service := &Service{policy: testPolicyWithoutEndpoints()}
	policy := service.Policy()

	if policy.Providers[0].Configured {
		t.Fatalf("ollama provider should not be configured without OLLAMA_BASE_URL")
	}
	if policy.Providers[0].ReadinessStatus != "not_configured" {
		t.Fatalf("readiness = %q, want not_configured", policy.Providers[0].ReadinessStatus)
	}
}

func TestRouteBlocksLinkLocalProviderEndpointByDefault(t *testing.T) {
	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = "http://169.254.169.254"
	service := &Service{policy: annotatePolicyReadiness(policy)}

	decision, err := service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if decision.SelectedModelID != "" {
		t.Fatalf("selected %q for blocked link-local provider", decision.SelectedModelID)
	}
	foundBlocked := false
	for _, skipped := range decision.Skipped {
		if skipped.ProviderID == "ollama" && skipped.Reason == "provider endpoint uses link-local, metadata, or unspecified address space" {
			foundBlocked = true
			break
		}
	}
	if !foundBlocked {
		t.Fatalf("expected link-local provider skip reason, got %#v", decision.Skipped)
	}
}

func TestLocalModelsAllowedPolicyIsEnforced(t *testing.T) {
	policy := testPolicyWithLocalEndpoints()
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

	policy := testPolicyWithoutEndpoints()
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

func TestGenerateDoesNotFollowProviderRedirect(t *testing.T) {
	redirectCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalled = true
	}))
	defer redirectTarget.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
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
	if redirectCalled {
		t.Fatalf("redirect target was called; provider calls must not follow redirects")
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

func TestProbeProvidersChecksOllamaTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path = %s, want /api/tags", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]string{{"name": "phi3:mini"}},
		})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = server.URL
	service := &Service{policy: policy}

	results := service.ProbeProviders()
	if results[0].Status != "live" {
		t.Fatalf("status = %q, want live: %s", results[0].Status, results[0].Reason)
	}
	if results[0].ModelsSeen != 1 {
		t.Fatalf("models seen = %d, want 1", results[0].ModelsSeen)
	}
}

func TestProbeProvidersDoesNotFollowRedirects(t *testing.T) {
	redirectCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalled = true
	}))
	defer redirectTarget.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = server.URL
	service := &Service{policy: policy}

	results := service.ProbeProviders()
	if redirectCalled {
		t.Fatalf("probe followed redirect target")
	}
	if results[0].Status != "failed" || !results[0].RequiresReview {
		t.Fatalf("probe status/review = %q/%v, want failed/review", results[0].Status, results[0].RequiresReview)
	}
}

func TestGenerateRedactsProviderErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token=super-secret-token", http.StatusBadGateway)
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
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
	if strings.Contains(result.Reason, "super-secret-token") {
		t.Fatalf("provider error leaked secret: %s", result.Reason)
	}
}

func TestGenerateBlocksWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "should not run"})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
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
	if called {
		t.Fatalf("provider endpoint was called while emergency stop was active")
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
}

func testPolicyWithLocalEndpoints() Policy {
	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = "http://localhost:11434"
	policy.Providers[1].EndpointURL = "http://localhost:1234"
	return annotatePolicyReadiness(policy)
}

func testPolicyWithoutEndpoints() Policy {
	policy := defaultPolicy()
	for index := range policy.Providers {
		policy.Providers[index].EndpointURL = ""
	}
	return annotatePolicyReadiness(policy)
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

	policy := testPolicyWithoutEndpoints()
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
	policy := testPolicyWithoutEndpoints()
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
