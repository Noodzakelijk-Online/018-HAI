package llm

import (
	"automation-hub-backend/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	if decision.Tier != TierLocal {
		t.Fatalf("selected tier %q, want %q", decision.Tier, TierLocal)
	}
	if decision.Classification.TaskType != "coding" {
		t.Fatalf("classified task as %q, want coding", decision.Classification.TaskType)
	}
}

func TestRouteIncludesTaskSpecificTokenAndCostEstimate(t *testing.T) {
	policy := Policy{
		DailyPaidBudgetEUR:             5,
		PaidCallsAllowed:               true,
		LocalModelsAllowed:             true,
		FreeCloudQuotaAllowed:          true,
		LocalFirst:                     true,
		RequireApprovalBeforePaidUsage: false,
		TierOrder:                      []string{TierLocal, TierFree, TierCheap, TierAcceptable, TierHigh, TierPremium, TierExpensive},
		Providers: []Provider{
			{
				ID:             "priced-test",
				Name:           "Priced test provider",
				Enabled:        true,
				Paid:           true,
				EndpointURL:    "http://localhost:9999",
				DailyBudgetEUR: 5,
				Models: []Model{
					{
						ID:                            "priced-coder",
						Name:                          "Priced coder",
						Tier:                          TierCheap,
						Capabilities:                  []string{"general", "coding"},
						MaxDifficulty:                 5,
						MaxReasoning:                  "high",
						InputCostPerMillionTokensEUR:  2,
						OutputCostPerMillionTokensEUR: 6,
						PricingSource:                 "test price sheet",
						Enabled:                       true,
					},
				},
			},
		},
	}
	service := &Service{policy: policy}

	decision, err := service.Route(RouteRequest{Task: "Fix a Go API bug and explain the compile failure"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.SelectedModelID != "priced-coder" {
		t.Fatalf("selected model = %q, want priced-coder", decision.SelectedModelID)
	}
	if decision.EstimatedInputTokens == 0 || decision.EstimatedOutputTokens == 0 {
		t.Fatalf("missing token estimates: %#v", decision)
	}
	if decision.EstimatedCostEUR <= 0 {
		t.Fatalf("estimated cost = %f, want > 0", decision.EstimatedCostEUR)
	}
	if decision.PricingSource != "test price sheet" {
		t.Fatalf("pricing source = %q, want test price sheet", decision.PricingSource)
	}
	if !strings.Contains(decision.Reason, "input") || !strings.Contains(decision.Reason, "output") {
		t.Fatalf("reason should include token estimate, got %q", decision.Reason)
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

func TestGenerateStrictLiveProbePolicyBlocksExplicitRouteDecision(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "must not be called"})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = server.URL
	policy.RequireRecentLiveProviderProbe = true
	policy.ProviderProbeMaxAgeSeconds = 300
	service := &Service{policy: policy, probeHistory: &fakeProbeHistoryRepository{}}

	result, err := service.Generate(GenerateRequest{
		Task: "Summarize this short note",
		RouteDecision: &RouteDecision{
			SelectedProviderID: "ollama",
			SelectedModelID:    "phi3:mini",
			SelectedModelName:  "Phi small local",
			Tier:               TierLocal,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if called {
		t.Fatalf("explicit route decision bypassed strict live-probe policy")
	}
	if result.Status != "skipped" || !strings.Contains(result.Reason, "persisted live") {
		t.Fatalf("result = %#v, want strict readiness skip", result)
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

func TestProbeAndRecordProvidersPersistsRedactedLastSuccess(t *testing.T) {
	healthy := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]string{{"name": "phi3:mini"}},
			})
			return
		}
		http.Error(w, "token=super-secret-provider-token", http.StatusBadGateway)
	}))
	defer server.Close()

	repository := &fakeProbeHistoryRepository{}
	service := &Service{
		policy:       Policy{Providers: []Provider{{ID: "ollama", Name: "Ollama", Enabled: true, Local: true, EndpointURL: server.URL}}},
		probeHistory: repository,
	}

	first, err := service.ProbeAndRecordProviders()
	if err != nil {
		t.Fatalf("ProbeAndRecordProviders first run: %v", err)
	}
	if len(first) != 1 || !first[0].Live || first[0].LastSuccessfulAt == nil {
		t.Fatalf("first probe = %#v, want live persisted result with last success", first)
	}
	firstSuccess := *first[0].LastSuccessfulAt

	healthy = false
	second, err := service.ProbeAndRecordProviders()
	if err != nil {
		t.Fatalf("ProbeAndRecordProviders failed run: %v", err)
	}
	if len(second) != 1 || second[0].Live || second[0].LastSuccessfulAt == nil {
		t.Fatalf("second probe = %#v, want failed result retaining last success", second)
	}
	if !second[0].LastSuccessfulAt.Equal(firstSuccess) {
		t.Fatalf("last success = %s, want %s", second[0].LastSuccessfulAt, firstSuccess)
	}
	if strings.Contains(second[0].Reason, "super-secret-provider-token") {
		t.Fatalf("probe result leaked secret: %s", second[0].Reason)
	}
	if strings.Contains(repository.probes[1].Reason, "super-secret-provider-token") {
		t.Fatalf("persisted probe leaked secret: %#v", repository.probes[1])
	}

	history, err := service.ProviderProbeHistory(10)
	if err != nil {
		t.Fatalf("ProviderProbeHistory: %v", err)
	}
	if len(history) != 2 || history[0].Status != "failed" || history[1].Status != "live" {
		t.Fatalf("probe history = %#v, want newest failed then live", history)
	}
}

func TestRouteStrictLiveProbePolicyRequiresRecentLiveEvidence(t *testing.T) {
	policy := testPolicyWithoutEndpoints()
	policy.Providers[0].EndpointURL = "http://localhost:11434"
	policy.RequireRecentLiveProviderProbe = true
	policy.ProviderProbeMaxAgeSeconds = 300
	repository := &fakeProbeHistoryRepository{}
	service := &Service{policy: policy, probeHistory: repository}

	decision, err := service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route without probe: %v", err)
	}
	if decision.SelectedModelID != "" {
		t.Fatalf("selected %q without a persisted live probe", decision.SelectedModelID)
	}
	if !skippedReasonContains(decision.Skipped, "ollama", "has not passed a persisted live") {
		t.Fatalf("missing strict no-probe skip reason: %#v", decision.Skipped)
	}

	now := time.Now().UTC()
	repository.probes = append(repository.probes, models.LLMProviderProbe{ProviderID: "ollama", Live: true, CheckedAt: now})
	decision, err = service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route with live probe: %v", err)
	}
	if decision.SelectedModelID == "" {
		t.Fatalf("expected a route after a recent live probe: %#v", decision.Skipped)
	}

	repository.probes = append(repository.probes, models.LLMProviderProbe{ProviderID: "ollama", Live: false, CheckedAt: now.Add(time.Second)})
	decision, err = service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route after failed probe: %v", err)
	}
	if decision.SelectedModelID != "" || !skippedReasonContains(decision.Skipped, "ollama", "latest readiness check is not live") {
		t.Fatalf("failed latest probe must block route: %#v", decision)
	}

	repository.probes = []models.LLMProviderProbe{{ProviderID: "ollama", Live: true, CheckedAt: now.Add(-6 * time.Minute)}}
	decision, err = service.Route(RouteRequest{Task: "Summarize this short note"})
	if err != nil {
		t.Fatalf("Route after stale probe: %v", err)
	}
	if decision.SelectedModelID != "" || !skippedReasonContains(decision.Skipped, "ollama", "readiness check is stale") {
		t.Fatalf("stale live probe must block route: %#v", decision)
	}
}

func skippedReasonContains(skipped []SkippedModel, providerID, text string) bool {
	for _, item := range skipped {
		if item.ProviderID == providerID && strings.Contains(item.Reason, text) {
			return true
		}
	}
	return false
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

func providerIndex(t *testing.T, policy Policy, providerID string) int {
	t.Helper()
	for index, provider := range policy.Providers {
		if provider.ID == providerID {
			return index
		}
	}
	t.Fatalf("provider %q not found in policy", providerID)
	return -1
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
	paidIndex := providerIndex(t, policy, "paid-provider")
	policy.Providers[paidIndex].Enabled = true
	policy.Providers[paidIndex].EndpointURL = "http://example.invalid"
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

func TestDefaultPolicyIncludesOdysseusWhenConfigured(t *testing.T) {
	t.Setenv("ODYSSEUS_BASE_URL", "http://localhost:8080")
	t.Setenv("ODYSSEUS_API_TOKEN", "test-token")

	policy := defaultPolicy()
	odysseusIndex := providerIndex(t, policy, "odysseus")
	provider := policy.Providers[odysseusIndex]

	if provider.EndpointURL != "http://localhost:8080" {
		t.Fatalf("endpoint = %q, want configured Odysseus URL", provider.EndpointURL)
	}
	if provider.APIKeyEnv != "ODYSSEUS_API_TOKEN" {
		t.Fatalf("apiKeyEnv = %q, want ODYSSEUS_API_TOKEN", provider.APIKeyEnv)
	}
	if len(provider.Models) != 1 || !provider.Models[0].RequiresApproval {
		t.Fatalf("Odysseus workspace model must be approval-required: %#v", provider.Models)
	}
}

func TestProbeProvidersChecksOdysseusHealthWithOptionalToken(t *testing.T) {
	t.Setenv("ODYSSEUS_API_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Fatalf("path = %s, want /api/health", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	odysseusIndex := providerIndex(t, policy, "odysseus")
	policy.Providers[odysseusIndex].EndpointURL = server.URL
	policy.Providers[odysseusIndex].APIKeyEnv = "ODYSSEUS_API_TOKEN"
	service := &Service{policy: policy}

	results := service.ProbeProviders()
	var odysseusProbe ProviderProbeResult
	for _, result := range results {
		if result.ProviderID == "odysseus" {
			odysseusProbe = result
			break
		}
	}
	if odysseusProbe.ProviderID == "" {
		t.Fatalf("Odysseus probe not returned: %#v", results)
	}
	if odysseusProbe.Status != "live" {
		t.Fatalf("status = %q, want live: %s", odysseusProbe.Status, odysseusProbe.Reason)
	}
	if !strings.Contains(odysseusProbe.Reason, "execution remains approval-gated") {
		t.Fatalf("probe reason should document execution gate: %s", odysseusProbe.Reason)
	}
}

func TestGenerateBlocksOdysseusExecutionEvenWhenInternallyApproved(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "should not execute"})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	odysseusIndex := providerIndex(t, policy, "odysseus")
	policy.Providers[odysseusIndex].EndpointURL = server.URL
	service := &Service{policy: policy}

	result, err := service.Generate(GenerateRequest{
		Task:              "Let Odysseus run this autonomous agent task",
		AllowPaidApproved: true,
		RouteDecision: &RouteDecision{
			SelectedProviderID: "odysseus",
			SelectedModelID:    "odysseus-workspace-agent",
			SelectedModelName:  "Odysseus workspace agent",
			Tier:               TierFree,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if called {
		t.Fatalf("Odysseus endpoint was called despite execution guard")
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !strings.Contains(result.Reason, "discovery and health probing only") {
		t.Fatalf("reason = %q, want discovery-only guard", result.Reason)
	}
}

func TestPolicyKeepsDualPathDisabledWithoutVerifiedInfrastructure(t *testing.T) {
	t.Setenv("LLM_KV_CACHE_LOAD_STRATEGY", "dual")
	t.Setenv("LLM_DUALPATH_INFRASTRUCTURE_VERIFIED", "false")
	service := &Service{policy: defaultPolicy()}

	infrastructure := service.Policy().InferenceInfrastructure
	if infrastructure.KVCacheLoadStrategy != "dual" {
		t.Fatalf("strategy = %q, want dual", infrastructure.KVCacheLoadStrategy)
	}
	if infrastructure.DualPathInfrastructureAvailable {
		t.Fatalf("expected DualPath to remain unavailable without verified infrastructure")
	}
}

func TestPolicyRejectsUnknownKVCacheStrategy(t *testing.T) {
	t.Setenv("LLM_KV_CACHE_LOAD_STRATEGY", "pretend-fast")
	service := &Service{policy: defaultPolicy()}

	if got := service.Policy().InferenceInfrastructure.KVCacheLoadStrategy; got != "disabled" {
		t.Fatalf("strategy = %q, want disabled", got)
	}
}

func TestDefaultPolicyIncludesNousPortalCatalog(t *testing.T) {
	t.Setenv("NOUS_PORTAL_BASE_URL", "https://portal.example.test/v1")
	t.Setenv("NOUS_PORTAL_API_KEY", "test-token")

	policy := defaultPolicy()
	nousIndex := providerIndex(t, policy, "nous-portal")
	provider := policy.Providers[nousIndex]

	if provider.EndpointURL != "https://portal.example.test/v1" {
		t.Fatalf("endpoint = %q, want configured Nous Portal URL", provider.EndpointURL)
	}
	if provider.APIKeyEnv != "NOUS_PORTAL_API_KEY" {
		t.Fatalf("apiKeyEnv = %q, want NOUS_PORTAL_API_KEY", provider.APIKeyEnv)
	}
	if !provider.Paid {
		t.Fatalf("Nous Portal should be marked paid/approval-gated")
	}
	if len(provider.Models) != 24 {
		t.Fatalf("models = %d, want 24: %#v", len(provider.Models), provider.Models)
	}

	wantTiers := map[string]string{
		"opus-4.8":                   TierExpensive,
		"gpt-5.5-pro":                TierExpensive,
		"gpt-5.5":                    TierPremium,
		"qwen3.7-max":                TierPremium,
		"gemini-3-pro-preview":       TierPremium,
		"hy3-preview":                TierPremium,
		"nemotron-3-super-120b-a12b": TierPremium,
		"sonnet-4.6":                 TierHigh,
		"deepseek-v4-pro":            TierHigh,
		"gemini-3.1-pro-preview":     TierHigh,
		"grok-4.3":                   TierHigh,
		"kimi-k2.7-code":             TierHigh,
		"minimax-m3":                 TierHigh,
		"glm-5.2":                    TierHigh,
		"mimo-v2.5-pro":              TierHigh,
		"haiku-4.5":                  TierAcceptable,
		"qwen3.7-plus":               TierAcceptable,
		"glm-5.1":                    TierAcceptable,
		"gpt-5.4-mini":               TierCheap,
		"gemini-3.5-flash":           TierCheap,
		"deepseek-v4-flash":          TierCheap,
		"qwen3.6-35b-a3b":            TierCheap,
		"step-3.7-flash":             TierCheap,
		"step-3.7-flash-free":        TierFree,
	}
	for modelID, tier := range wantTiers {
		model, ok := findModel(provider.Models, modelID)
		if !ok {
			t.Fatalf("model %q not found", modelID)
		}
		if model.Tier != tier {
			t.Fatalf("model %s tier = %q, want %q", modelID, model.Tier, tier)
		}
		if !model.RequiresApproval {
			t.Fatalf("model %s should require approval", modelID)
		}
		if modelID == "step-3.7-flash-free" {
			if model.InputCostPerMillionTokensEUR != 0 || model.OutputCostPerMillionTokensEUR != 0 {
				t.Fatalf("free model %s should have zero token pricing: %#v", modelID, model)
			}
			continue
		}
		if model.InputCostPerMillionTokensEUR <= 0 || model.OutputCostPerMillionTokensEUR <= 0 {
			t.Fatalf("model %s missing token pricing: %#v", modelID, model)
		}
		if !strings.Contains(model.PricingSource, "default estimate") {
			t.Fatalf("model %s pricing source = %q, want default estimate warning", modelID, model.PricingSource)
		}
	}
}

func TestDefaultPolicyIncludesMixtureAndOpenAICodexCatalogs(t *testing.T) {
	t.Setenv("MIXTURE_OF_AGENTS_BASE_URL", "https://moa.example.test/v1")
	t.Setenv("MIXTURE_OF_AGENTS_API_KEY", "test-token")
	t.Setenv("OPENAI_CODEX_BASE_URL", "https://codex.example.test/v1")
	t.Setenv("OPENAI_CODEX_API_KEY", "test-token")

	policy := defaultPolicy()
	mixture := policy.Providers[providerIndex(t, policy, "mixture-of-agents")]
	codex := policy.Providers[providerIndex(t, policy, "openai-codex")]

	if mixture.EndpointURL != "https://moa.example.test/v1" || mixture.APIKeyEnv != "MIXTURE_OF_AGENTS_API_KEY" {
		t.Fatalf("mixture provider env mapping is wrong: %#v", mixture)
	}
	if len(mixture.Models) != 1 || mixture.Models[0].ID != "moa-default" || mixture.Models[0].Tier != TierPremium {
		t.Fatalf("unexpected mixture catalog: %#v", mixture.Models)
	}
	if codex.EndpointURL != "https://codex.example.test/v1" || codex.APIKeyEnv != "OPENAI_CODEX_API_KEY" {
		t.Fatalf("codex provider env mapping is wrong: %#v", codex)
	}
	wantTiers := map[string]string{
		"gpt-5.5":             TierPremium,
		"gpt-5.4":             TierHigh,
		"gpt-5.4-mini":        TierCheap,
		"gpt-5.3-codex-spark": TierCheap,
	}
	for modelID, tier := range wantTiers {
		model, ok := findModel(codex.Models, modelID)
		if !ok {
			t.Fatalf("codex model %q not found", modelID)
		}
		if model.Tier != tier {
			t.Fatalf("codex model %s tier = %q, want %q", modelID, model.Tier, tier)
		}
		if !model.RequiresApproval {
			t.Fatalf("codex model %s should require approval", modelID)
		}
	}
}

func TestGenerateTracksModelLevelUsageAndTokenPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "priced draft response"}},
			},
		})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	policy.Providers[1].EndpointURL = server.URL
	policy.Providers[1].Models[0].InputCostPerMillionTokensEUR = 10
	policy.Providers[1].Models[0].OutputCostPerMillionTokensEUR = 20
	service := &Service{policy: policy}

	result, err := service.Generate(GenerateRequest{
		Task: "Plan this work item",
		RouteDecision: &RouteDecision{
			SelectedProviderID: "lm-studio",
			SelectedModelID:    "openai-compatible-local",
			SelectedModelName:  "OpenAI-compatible local endpoint",
			Tier:               TierLocal,
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	policyWithUsage := service.Policy()
	provider := policyWithUsage.Providers[1]
	model := provider.Models[0]
	if provider.InputTokensUsed == 0 || provider.OutputTokensUsed == 0 {
		t.Fatalf("provider usage not tracked: %#v", provider)
	}
	if model.InputTokensUsed != provider.InputTokensUsed || model.OutputTokensUsed != provider.OutputTokensUsed {
		t.Fatalf("model usage %#v did not match provider usage %#v", model, provider)
	}
	if model.BudgetUsedEUR <= 0 || provider.BudgetUsedEUR <= 0 || policyWithUsage.DailyBudgetUsedEUR <= 0 {
		t.Fatalf("priced usage not accumulated: provider=%#v model=%#v policy=%#v", provider, model, policyWithUsage)
	}
}

func findModel(models []Model, id string) (Model, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

type fakeProbeHistoryRepository struct {
	probes []models.LLMProviderProbe
}

func (r *fakeProbeHistoryRepository) RecordProviderProbe(probe *models.LLMProviderProbe) (*models.LLMProviderProbe, error) {
	copy := *probe
	if copy.Live {
		lastSuccess := copy.CheckedAt
		copy.LastSuccessfulAt = &lastSuccess
	} else {
		for index := len(r.probes) - 1; index >= 0; index-- {
			previous := r.probes[index]
			if previous.ProviderID == copy.ProviderID && previous.LastSuccessfulAt != nil {
				lastSuccess := *previous.LastSuccessfulAt
				copy.LastSuccessfulAt = &lastSuccess
				break
			}
		}
	}
	if copy.CheckedAt.IsZero() {
		copy.CheckedAt = time.Now().UTC()
	}
	r.probes = append(r.probes, copy)
	return &copy, nil
}

func (r *fakeProbeHistoryRepository) FindRecentProviderProbes(limit int) ([]models.LLMProviderProbe, error) {
	if limit <= 0 || limit > len(r.probes) {
		limit = len(r.probes)
	}
	result := make([]models.LLMProviderProbe, 0, limit)
	for index := len(r.probes) - 1; index >= 0 && len(result) < limit; index-- {
		result = append(result, r.probes[index])
	}
	return result, nil
}

func (r *fakeProbeHistoryRepository) FindLatestProviderProbe(providerID string) (*models.LLMProviderProbe, error) {
	var latest *models.LLMProviderProbe
	for index := range r.probes {
		probe := r.probes[index]
		if probe.ProviderID == providerID && (latest == nil || probe.CheckedAt.After(latest.CheckedAt)) {
			latest = &probe
		}
	}
	return latest, nil
}
