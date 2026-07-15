package llm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerateHandlerIgnoresClientPaidApprovalFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "paid answer"}},
			},
		})
	}))
	defer server.Close()

	policy := testPolicyWithoutEndpoints()
	paidIndex := providerIndex(t, policy, "paid-provider")
	policy.Providers[paidIndex].Enabled = true
	policy.Providers[paidIndex].EndpointURL = server.URL
	policy.PaidCallsAllowed = true
	policy.DailyPaidBudgetEUR = 1
	handler := &Handler{service: &Service{policy: policy}}

	body, err := json.Marshal(GenerateRequest{
		Task:              "Handle a difficult verification task",
		AllowPaidApproved: true,
		RouteDecision: &RouteDecision{
			SelectedProviderID: "paid-provider",
			SelectedModelID:    "paid-high-capability",
			SelectedModelName:  "Paid high capability model",
			Tier:               TierExpensive,
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	handler.Generate(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if called {
		t.Fatalf("paid provider endpoint was called from client-supplied approval")
	}
	var result GenerationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
}

func TestProviderProbeHandlersRecordAndReturnHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]string{{"name": "phi3:mini"}},
		})
	}))
	defer server.Close()

	service := &Service{
		policy:       Policy{Providers: []Provider{{ID: "ollama", Name: "Ollama", Enabled: true, Local: true, EndpointURL: server.URL}}},
		probeHistory: &fakeProbeHistoryRepository{},
	}
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/probes", handler.ProviderProbes)
	router.GET("/probes/history", handler.ProviderProbeHistory)

	probeResponse := httptest.NewRecorder()
	router.ServeHTTP(probeResponse, httptest.NewRequest(http.MethodGet, "/probes", nil))
	if probeResponse.Code != http.StatusOK {
		t.Fatalf("probe status = %d, body=%s", probeResponse.Code, probeResponse.Body.String())
	}

	historyResponse := httptest.NewRecorder()
	router.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/probes/history?limit=5", nil))
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("history status = %d, body=%s", historyResponse.Code, historyResponse.Body.String())
	}
	var history []ProviderProbeResult
	if err := json.Unmarshal(historyResponse.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode probe history: %v", err)
	}
	if len(history) != 1 || !history[0].Live || history[0].LastSuccessfulAt == nil {
		t.Fatalf("history = %#v, want persisted live probe", history)
	}
}
