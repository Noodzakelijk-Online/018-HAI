package pydanticai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type maintenanceGateStub struct {
	endpoint string
	modelID  string
	err      error
}

func (s *maintenanceGateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error {
	s.endpoint, s.modelID = endpointURL, modelID
	return s.err
}

func TestPydanticAIBridgeUsesOnlyLocalTypedProposalRunner(t *testing.T) {
	input := Request{Request: "Prepare a source-grounded plan", SuccessCriteria: []string{"Use relevant sources", "Do not send messages"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","configured":true,"modelId":"qwen-local","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
		case "/v1/probe":
			if r.Method != http.MethodPost || r.Header.Get("User-Agent") != "HAI-PydanticAI-Proposal/1.0" {
				t.Fatalf("unexpected probe request")
			}
			_, _ = w.Write([]byte(`{"status":"ok","engine":"pydantic-ai 2.13.0","modelId":"qwen-local"}`))
		case "/v1/propose":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-PydanticAI-Proposal/1.0" {
				t.Fatalf("unexpected proposal request")
			}
			var received Request
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil || received.Request != input.Request {
				t.Fatalf("unexpected input: %#v %v", received, err)
			}
			result := Response{Engine: "pydantic-ai 2.13.0", ModelID: "qwen-local", RequestDigest: requestDigest(input), Proposal: Proposal{Goal: "Prepare a source-grounded plan", SuccessCriteria: []string{"Use relevant sources"}, NextSteps: []string{"Review the relevant evidence"}, Risk: "low", RequiresApproval: false, Reasons: []string{"No external action is proposed"}, Uncertainties: []string{}}}
			_ = json.NewEncoder(w).Encode(result)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	gate := &maintenanceGateStub{}
	service := WithModelMaintenance(NewService(true, server.URL, 0, nil), gate)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable || probe.ModelID != "qwen-local" {
		t.Fatalf("unexpected probe: %#v %v", probe, err)
	}
	result, err := service.Propose(context.Background(), input)
	if err != nil || result.Proposal.Risk != "low" || result.Proposal.NextSteps[0] != "Review the relevant evidence" {
		t.Fatalf("unexpected proposal: %#v %v", result, err)
	}
	if gate.endpoint != "http://127.0.0.1:11434/v1" || gate.modelID != "qwen-local" {
		t.Fatalf("model maintenance gate received %#v", gate)
	}
}

func TestPydanticAIBridgeRejectsExternalDisabledAndUnboundedRequests(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	_, err := disabled.Propose(context.Background(), Request{Request: "Make a plan"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
	_, err = NewService(true, "http://127.0.0.1:8080", 0, nil).Propose(context.Background(), Request{Request: "\n"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("multiline request must be rejected: %v", err)
	}
}
