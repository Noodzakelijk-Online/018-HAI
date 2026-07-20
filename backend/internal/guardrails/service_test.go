package guardrails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuardrailsBridgeUsesOnlyLocalFixedSchemaRunner(t *testing.T) {
	proposal := `{"title":"Review evidence","summary":"Compare opaque source identifiers.","risk":"medium","requiresApproval":true,"nextAction":"Open review queue.","sourceRefs":["source_1"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","engine":"guardrails-ai 0.10.2"}`))
		case "/v1/validate":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-Guardrails-Validation/1.0" {
				t.Fatalf("unexpected validation request: %s auth=%q ua=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("User-Agent"))
			}
			_, _ = w.Write([]byte(`{"status":"valid","engine":"guardrails-ai 0.10.2","schema":"action_proposal","valid":true,"violationCount":0,"proposalDigest":"` + proposalDigest(proposal) + `"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	service := NewService(true, server.URL, 0, nil)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable {
		t.Fatalf("unexpected probe: %#v %v", probe, err)
	}
	result, err := service.Validate(context.Background(), Request{Schema: schemaName, Proposal: proposal})
	if err != nil || !result.Valid || result.Status != "valid" {
		t.Fatalf("unexpected validation result: %#v %v", result, err)
	}
}

func TestGuardrailsBridgeRejectsExternalDisabledAndSensitiveRequests(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	_, err := disabled.Validate(context.Background(), Request{Schema: schemaName, Proposal: `{"title":"Review","summary":"safe","risk":"low","requiresApproval":false,"nextAction":"Inspect."}`})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
	configured := NewService(true, "http://127.0.0.1:8080", 0, nil)
	_, err = configured.Validate(context.Background(), Request{Schema: schemaName, Proposal: `{"title":"Email","summary":"Email test@example.com","risk":"low","requiresApproval":false,"nextAction":"Inspect."}`})
	if !errors.Is(err, ErrUnsafeProposal) {
		t.Fatalf("sensitive proposal must be rejected before runner call: %v", err)
	}
}

func proposalDigest(proposal string) string {
	digest := sha256.Sum256([]byte(proposal))
	return hex.EncodeToString(digest[:])
}
