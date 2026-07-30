package grype

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceScansOnlyConfiguredWorkspaceAndReturnsAggregate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = io.WriteString(w, `{"status":"ok","engine":"grype 0.116.0","configured":true}`)
		case "/v1/scan":
			if r.Header.Get("X-HAI-Grype-Token") != "runner-token-1234" {
				t.Fatalf("runner token missing")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"workspaceId":"review-snapshot"}` {
				t.Fatalf("scan body = %s", body)
			}
			_, _ = io.WriteString(w, `{"status":"completed","engine":"grype 0.116.0","workspaceId":"review-snapshot","vulnerabilityCount":2,"fixAvailableCount":1,"severities":[{"severity":"high","count":2}],"durationMs":14,"resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
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
	if result.VulnerabilityCount != 2 || result.FixAvailableCount != 1 || len(result.Severities) != 1 || result.Severities[0].Severity != "high" || strings.Contains(result.Scope, "CVE") {
		t.Fatalf("unexpected aggregate result: %#v", result)
	}
}

func TestServiceRejectsUnapprovedWorkspaceAndUnsafeRunnerURL(t *testing.T) {
	service := NewService(Config{Enabled: true, RunnerURL: "https://grype.example.test", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if service.Status().Configured {
		t.Fatalf("public runner URL must not configure the service: %#v", service.Status())
	}
	service = NewService(Config{Enabled: true, RunnerURL: "http://127.0.0.1:8080", Token: "runner-token-1234", Workspaces: []string{"review-snapshot"}, Timeout: time.Minute}, nil)
	if _, err := service.Scan(context.Background(), "other"); err != ErrWorkspace {
		t.Fatalf("Scan unapproved workspace error = %v", err)
	}
}

func TestResultValidationRejectsPackageDetailsAndInconsistentMetadata(t *testing.T) {
	result := ScanResult{Status: "completed", Engine: "grype 0.116.0", WorkspaceID: "review-snapshot", VulnerabilityCount: 1, FixAvailableCount: 1, Severities: []SeverityCount{{Severity: "high", Count: 1}}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	if !validResult(result, "review-snapshot") {
		t.Fatal("expected bounded aggregate to be valid")
	}
	result.Severities[0].Severity = "github.com/example/project"
	if validResult(result, "review-snapshot") {
		t.Fatal("arbitrary package-like severity must be rejected")
	}
}
