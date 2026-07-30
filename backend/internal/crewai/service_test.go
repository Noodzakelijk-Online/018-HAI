package crewai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type maintenanceGateStub struct { endpoint string; modelID string; err error }
func (s *maintenanceGateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error { s.endpoint, s.modelID = endpointURL, modelID; return s.err }

func TestCrewAIBridgeUsesOnlyTheLocalPlanningRunner(t *testing.T) {
	input := Request{Request: "Prepare a source-grounded plan", SuccessCriteria: []string{"Use relevant sources", "Do not send messages"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","configured":true,"modelId":"qwen-local","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
		case "/v1/probe":
			if r.Method != http.MethodPost || r.Header.Get("User-Agent") != "HAI-CrewAI-Planning/1.0" { t.Fatalf("unexpected probe request") }
			_, _ = w.Write([]byte(`{"status":"ok","engine":"crewai 1.15.5","modelId":"qwen-local"}`))
		case "/v1/propose":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-CrewAI-Planning/1.0" { t.Fatalf("unexpected proposal request") }
			var received Request
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil || received.Request != input.Request { t.Fatalf("unexpected input: %#v %v", received, err) }
			_ = json.NewEncoder(w).Encode(Response{Engine: "crewai 1.15.5", ModelID: "qwen-local", RequestDigest: requestDigest(input), Proposal: Proposal{Goal: "Prepare a source-grounded plan", SuccessCriteria: []string{"Use relevant sources"}, NextSteps: []string{"Review relevant evidence"}, Risk: "low", RequiresApproval: false, Reasons: []string{"No external action is proposed"}}})
		default: t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	gate := &maintenanceGateStub{}
	service := WithModelMaintenance(NewService(true, server.URL, 0, nil), gate)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable || probe.ModelID != "qwen-local" { t.Fatalf("unexpected probe: %#v %v", probe, err) }
	if result, err := service.Propose(context.Background(), input); err != nil || result.Proposal.NextSteps[0] != "Review relevant evidence" { t.Fatalf("unexpected proposal: %#v %v", result, err) }
	if gate.endpoint != "http://127.0.0.1:11434/v1" || gate.modelID != "qwen-local" { t.Fatalf("model maintenance gate received %#v", gate) }
}

func TestCrewAIBridgeRejectsExternalDisabledAndUnboundedRequests(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" { t.Fatalf("external runner must be rejected: %#v", external.Status()) }
	if _, err := NewService(false, "http://127.0.0.1:8080", 0, nil).Propose(context.Background(), Request{Request: "Make a plan"}); !errors.Is(err, ErrNotConfigured) { t.Fatalf("disabled runner must not be contacted: %v", err) }
	if _, err := NewService(true, "http://127.0.0.1:8080", 0, nil).Propose(context.Background(), Request{Request: "\n"}); !errors.Is(err, ErrInvalidRequest) { t.Fatalf("multiline request must be rejected: %v", err) }
}

func TestRequestDigestRetainsAnEmptyCriteriaArrayForRunnerParity(t *testing.T) {
	input := Request{Request: "Prepare a bounded plan"}
	if got, want := requestDigest(input), ""; got == want {
		t.Fatal("request digest must be populated")
	}
	encoded, err := json.Marshal(struct {
		Request         string   `json:"request"`
		SuccessCriteria []string `json:"successCriteria"`
	}{Request: input.Request, SuccessCriteria: []string{}})
	if err != nil {
		t.Fatalf("encode canonical request: %v", err)
	}
	if got, want := requestDigest(input), sha256Hex(encoded); got != want {
		t.Fatalf("digest = %s, want canonical empty-array digest %s", got, want)
	}
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
