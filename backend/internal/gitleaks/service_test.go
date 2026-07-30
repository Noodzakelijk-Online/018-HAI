package gitleaks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type workflowLinkerStub struct {
	owner, workflowID, workspaceID, digest string
	findingCount, affectedFiles            int
	err                                    error
}

func (s *workflowLinkerStub) AttachSecretScan(ownerIdentity, workflowID, workspaceID, resultDigest string, findingCount, affectedFiles int) error {
	s.owner, s.workflowID, s.workspaceID, s.digest = ownerIdentity, workflowID, workspaceID, resultDigest
	s.findingCount, s.affectedFiles = findingCount, affectedFiles
	return s.err
}

func TestServiceScansOnlyConfiguredWorkspaceAndReturnsAggregate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = io.WriteString(w, `{"status":"ok","engine":"gitleaks 8.30.1","configured":true}`)
		case "/v1/scan":
			if r.Header.Get("X-HAI-Gitleaks-Token") != "runner-token-1234" {
				t.Fatalf("runner token missing")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"workspaceId":"review-snapshot"}` {
				t.Fatalf("scan body = %s", body)
			}
			_, _ = io.WriteString(w, `{"status":"completed","engine":"gitleaks 8.30.1","workspaceId":"review-snapshot","findingCount":2,"affectedFiles":1,"rules":[{"id":"github-pat","count":2}],"durationMs":14,"resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(Config{Enabled: true, RunnerURL: server.URL, Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, server.Client())
	if _, err := service.Probe(context.Background()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	result, err := service.Scan(context.Background(), "review-snapshot")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.FindingCount != 2 || result.AffectedFiles != 1 || len(result.Rules) != 1 || result.Rules[0].ID != "github-pat" || !strings.Contains(result.Scope, "aggregate redacted metadata") {
		t.Fatalf("unexpected aggregate result: %#v", result)
	}
}

func TestServiceRejectsUnapprovedWorkspaceAndUnsafeRunnerURL(t *testing.T) {
	service := NewService(Config{Enabled: true, RunnerURL: "https://gitleaks.example.test", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if service.Status().Configured {
		t.Fatalf("public runner URL must not configure the service: %#v", service.Status())
	}
	service = NewService(Config{Enabled: true, RunnerURL: "http://127.0.0.1:8080", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if _, err := service.Scan(context.Background(), "other"); err != ErrWorkspace {
		t.Fatalf("Scan unapproved workspace error = %v", err)
	}
}

func TestCompletedSecretScanCanLinkAnAggregateWorkflowSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scan" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"status":"completed","engine":"gitleaks 8.30.1","workspaceId":"review-snapshot","findingCount":0,"affectedFiles":0,"rules":[],"durationMs":14,"resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	}))
	defer server.Close()
	linker := &workflowLinkerStub{}
	service := NewService(Config{Enabled: true, RunnerURL: server.URL, Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, server.Client(), linker)
	workflowScanner, ok := service.(WorkflowScanService)
	if !ok {
		t.Fatal("configured Gitleaks service must expose workflow scan linkage")
	}
	result, err := workflowScanner.ScanWithWorkflow(context.Background(), "owner@example.test", "review-snapshot", "7f4b2da3-4678-47dc-9558-feb9540c3a3a")
	if err != nil || result.WorkflowLinkStatus != "linked_security_signal" || linker.owner != "owner@example.test" || linker.findingCount != 0 {
		t.Fatalf("result=%#v linker=%#v err=%v", result, linker, err)
	}
	linker.err = errors.New("workflow not found")
	result, err = workflowScanner.ScanWithWorkflow(context.Background(), "owner@example.test", "review-snapshot", "11111111-2222-3333-4444-555555555555")
	if err != nil || result.WorkflowLinkStatus != "link_failed" || result.WorkflowLinkError == "" {
		t.Fatalf("link failure must preserve aggregate scan: result=%#v err=%v", result, err)
	}
}

func TestResultValidationRejectsSecretBearingOrInconsistentMetadata(t *testing.T) {
	result := ScanResult{Status: "completed", Engine: "gitleaks 8.30.1", WorkspaceID: "review-snapshot", FindingCount: 1, AffectedFiles: 1, Rules: []RuleCount{{ID: "github-pat", Count: 1}}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	if !validResult(result, "review-snapshot") {
		t.Fatal("expected bounded aggregate to be valid")
	}
	result.Rules[0].ID = "secret value"
	if validResult(result, "review-snapshot") {
		t.Fatal("rule identifiers with arbitrary content must be rejected")
	}
}
