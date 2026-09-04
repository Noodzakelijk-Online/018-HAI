package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoopbackURLRejectsPublicAndCredentialedEndpoints(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8092",
		"http://localhost:8092",
		"http://[::1]:8092",
	} {
		endpoint, err := url.Parse(raw)
		if err != nil || !isLoopbackURL(endpoint) {
			t.Fatalf("%q must be accepted", raw)
		}
	}
	for _, raw := range []string{
		"http://example.com:8092",
		"http://127.0.0.1:8092?token=unsafe",
		"http://token@127.0.0.1:8092",
	} {
		endpoint, _ := url.Parse(raw)
		if isLoopbackURL(endpoint) {
			t.Fatalf("%q must be rejected", raw)
		}
	}
}

func TestConfirmLeaseSendsTokenAndRejectsNonNoContentResponses(t *testing.T) {
	var received confirmRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/host-runtime/leases/job-1/confirm" {
			t.Fatalf("unexpected confirmation request: %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatalf("decode confirmation: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	configuration := config{baseURL: endpoint, token: strings.Repeat("a", 32)}
	leased := lease{Token: "lease-token"}
	leased.Job.ID = "job-1"
	if err := confirmLease(context.Background(), server.Client(), configuration, leased); err != nil {
		t.Fatalf("confirmLease: %v", err)
	}
	if received.LeaseToken != leased.Token {
		t.Fatalf("confirmation token = %q, want %q", received.LeaseToken, leased.Token)
	}
}

func TestLeasePayloadDecodesGatewayJob(t *testing.T) {
	var leased lease
	if err := json.Unmarshal([]byte(`{"job":{"id":"job-1","runtimeId":"deepseek-harness","prompt":"inspect","workspaceKey":"deepseek-harness"},"leaseToken":"token"}`), &leased); err != nil {
		t.Fatalf("decode lease: %v", err)
	}
	if leased.Job.ID != "job-1" || leased.Job.RuntimeID != "deepseek-harness" || leased.Token != "token" {
		t.Fatalf("lease = %#v", leased)
	}
}

func TestSafeEnvironmentDropsUnapprovedVariables(t *testing.T) {
	t.Setenv("HAI_DSH_UNAPPROVED", "must-not-pass")
	t.Setenv("DEEPSEEK_API_KEY", "allowed-for-test")
	t.Setenv("USERPROFILE", `C:\Users\operator`)
	t.Setenv("LOCALAPPDATA", `C:\Users\operator\AppData\Local`)
	t.Setenv("HOME", `C:\Users\operator`)
	t.Setenv("DSH_HOME", `C:\untrusted-profile-state`)
	values := safeEnvironment([]string{"DEEPSEEK_API_KEY"}, map[string]string{"DSH_HOME": "C:\\state"})
	joined := strings.Join(values, "\n")
	for _, disallowed := range []string{"HAI_DSH_UNAPPROVED", "USERPROFILE", "LOCALAPPDATA", "HOME"} {
		for _, pair := range values {
			if strings.HasPrefix(pair, disallowed+"=") {
				t.Fatalf("safe environment leaked %q: %q", disallowed, joined)
			}
		}
	}
	if strings.Contains(joined, `C:\untrusted-profile-state`) {
		t.Fatalf("safe environment retained the ambient DSH profile: %q", joined)
	}
	if !strings.Contains(joined, "DEEPSEEK_API_KEY=allowed-for-test") || strings.Count(joined, "DSH_HOME=") != 1 || !strings.Contains(joined, "DSH_HOME=C:\\state") {
		t.Fatalf("safe environment = %q", joined)
	}
}

func TestInvalidPromptRejectsCLIControlValues(t *testing.T) {
	for _, prompt := range []string{"-unsafe", "web", "plugin", "\x00"} {
		if invalidPrompt(prompt) == "" {
			t.Fatalf("%q must be rejected", prompt)
		}
	}
	if invalidPrompt("inspect this approved workspace") != "" {
		t.Fatal("ordinary prompt was rejected")
	}
}

func TestMonitorExecutionCancelsRunningWorkWhenEmergencyStopStarts(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	result := monitorExecution(context.Background(), 5*time.Millisecond, func(context.Context) error {
		return errEmergencyStop
	}, func(ctx context.Context) completion {
		close(started)
		<-ctx.Done()
		close(stopped)
		return completion{ExitCode: 0, Output: "partial output"}
	})
	select {
	case <-started:
	default:
		t.Fatal("execution did not start")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("execution was not cancelled")
	}
	if result.ExitCode != -1 || result.Output != "partial output" || result.Error != "DeepSeek Harness execution was stopped because HAI emergency stop is active" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMonitorExecutionFailsClosedWhenLeaseCannotBeReconfirmed(t *testing.T) {
	confirmationFailure := errors.New("gateway unavailable")
	result := monitorExecution(context.Background(), 5*time.Millisecond, func(context.Context) error {
		return confirmationFailure
	}, func(ctx context.Context) completion {
		<-ctx.Done()
		return completion{ExitCode: 1, Error: "process cancelled"}
	})
	if result.ExitCode != -1 || !strings.Contains(result.Error, "could not reconfirm the execution lease") || !strings.Contains(result.Error, "gateway unavailable") {
		t.Fatalf("result = %#v", result)
	}
}
