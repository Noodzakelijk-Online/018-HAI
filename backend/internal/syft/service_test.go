package syft

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
	packageCount, ecosystemCount           int
	err                                    error
}

func (s *workflowLinkerStub) AttachSBOMInventory(ownerIdentity, workflowID, workspaceID, resultDigest string, packageCount, ecosystemCount int) error {
	s.owner, s.workflowID, s.workspaceID, s.digest = ownerIdentity, workflowID, workspaceID, resultDigest
	s.packageCount, s.ecosystemCount = packageCount, ecosystemCount
	return s.err
}

func TestServiceInventoriesOnlyConfiguredWorkspaceAndReturnsAggregate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = io.WriteString(w, `{"status":"ok","engine":"syft 1.48.0","configured":true}`)
		case "/v1/inventory":
			if r.Header.Get("X-HAI-Syft-Token") != "runner-token-1234" {
				t.Fatalf("runner token missing")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"workspaceId":"review-snapshot"}` {
				t.Fatalf("inventory body = %s", body)
			}
			_, _ = io.WriteString(w, `{"status":"completed","engine":"syft 1.48.0","workspaceId":"review-snapshot","packageCount":2,"ecosystems":[{"id":"npm","count":1},{"id":"go-module","count":1}],"durationMs":14,"resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(Config{Enabled: true, RunnerURL: server.URL, Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, server.Client())
	if _, err := service.Probe(context.Background()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	result, err := service.Inventory(context.Background(), "review-snapshot")
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if result.PackageCount != 2 || len(result.Ecosystems) != 2 || !strings.Contains(result.Scope, "aggregate redacted metadata") {
		t.Fatalf("unexpected aggregate result: %#v", result)
	}
}

func TestServiceRejectsUnapprovedWorkspaceAndUnsafeRunnerURL(t *testing.T) {
	service := NewService(Config{Enabled: true, RunnerURL: "https://syft.example.test", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if service.Status().Configured {
		t.Fatalf("public runner URL must not configure the service: %#v", service.Status())
	}
	service = NewService(Config{Enabled: true, RunnerURL: "http://127.0.0.1:8080", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if _, err := service.Inventory(context.Background(), "other"); err != ErrWorkspace {
		t.Fatalf("Inventory unapproved workspace error = %v", err)
	}
}

func TestCompletedInventoryCanLinkAnAggregateWorkflowSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/inventory" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"status":"completed","engine":"syft 1.48.0","workspaceId":"review-snapshot","packageCount":2,"ecosystems":[{"id":"npm","count":1},{"id":"go-module","count":1}],"durationMs":14,"resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	}))
	defer server.Close()
	linker := &workflowLinkerStub{}
	service := NewService(Config{Enabled: true, RunnerURL: server.URL, Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, server.Client(), linker)
	workflowInventory, ok := service.(WorkflowInventoryService)
	if !ok {
		t.Fatal("configured Syft service must expose workflow inventory linkage")
	}
	result, err := workflowInventory.InventoryWithWorkflow(context.Background(), "owner@example.test", "review-snapshot", "7f4b2da3-4678-47dc-9558-feb9540c3a3a")
	if err != nil || result.WorkflowLinkStatus != "linked_review_signal" || linker.owner != "owner@example.test" || linker.packageCount != 2 || linker.ecosystemCount != 2 {
		t.Fatalf("result=%#v linker=%#v err=%v", result, linker, err)
	}
	linker.err = errors.New("workflow not found")
	result, err = workflowInventory.InventoryWithWorkflow(context.Background(), "owner@example.test", "review-snapshot", "11111111-2222-3333-4444-555555555555")
	if err != nil || result.WorkflowLinkStatus != "link_failed" || result.WorkflowLinkError == "" {
		t.Fatalf("link failure must preserve inventory: result=%#v err=%v", result, err)
	}
}

func TestResultValidationRejectsPackageBearingOrInconsistentMetadata(t *testing.T) {
	result := InventoryResult{Status: "completed", Engine: "syft 1.48.0", WorkspaceID: "review-snapshot", PackageCount: 1, Ecosystems: []EcosystemCount{{ID: "npm", Count: 1}}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	if !validResult(result, "review-snapshot") {
		t.Fatal("expected bounded aggregate to be valid")
	}
	result.Ecosystems[0].ID = "private package name"
	if validResult(result, "review-snapshot") {
		t.Fatal("ecosystem identifiers with arbitrary content must be rejected")
	}
}
