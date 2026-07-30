package runtimelab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/operations"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	broker := executionbroker.NewBroker(t.TempDir())
	ops := operations.NewService(operations.NewMemoryRepository())
	return NewService(broker, ops, "local-operator", "local")
}

func TestOverviewTruthfulRuntimeStates(t *testing.T) {
	s := newTestService(t)
	byID := map[string]RuntimeSummary{}
	for _, r := range s.Overview(context.Background()) {
		byID[r.Info.ID] = r
	}
	// The safe worker is the only executable runtime.
	if !byID[executionbroker.LocalSafeWorkerID].CanExecute {
		t.Fatalf("local safe worker must be executable")
	}
	// External runtimes are not executable and carry setup requirements.
	for _, id := range []string{"hermes", "openclaw", "odysseus", "openhands"} {
		r := byID[id]
		if r.CanExecute {
			t.Fatalf("%s must not be executable without configuration", id)
		}
		if len(r.SetupRequirements) == 0 {
			t.Fatalf("%s must expose exact setup requirements", id)
		}
	}
	// Contracts are not executors.
	if byID["browser-runtime"].CanExecute || byID["local-script-runtime"].CanExecute {
		t.Fatalf("contract runtimes must not be executable")
	}
}

func TestSafeWorkerSelfTestThroughLedger(t *testing.T) {
	s := newTestService(t)
	attempt, ok := s.SelfTest(context.Background(), executionbroker.LocalSafeWorkerID)
	if !ok {
		t.Fatalf("safe worker self-test must be found")
	}
	if attempt.Status != AttemptSucceeded || !attempt.VerificationPassed {
		t.Fatalf("safe worker self-test must succeed and verify, got %s verified=%v (%s)", attempt.Status, attempt.VerificationPassed, attempt.Detail)
	}
	if attempt.OperationID == "" {
		t.Fatalf("self-test must run through the Operation Ledger (operationId set)")
	}
	// A completed operation must exist in the ledger.
	completed, _ := s.ops.List(operations.Filter{OwnerUserID: "local-operator", WorkspaceID: "local", Status: operations.StatusCompleted})
	if len(completed) != 1 {
		t.Fatalf("self-test must produce one completed ledger operation, got %d", len(completed))
	}
}

func TestExternalRuntimeSelfTestNeverFakes(t *testing.T) {
	s := newTestService(t)
	for _, id := range []string{"hermes", "openclaw", "odysseus", "openhands"} {
		attempt, ok := s.SelfTest(context.Background(), id)
		if !ok {
			t.Fatalf("%s self-test must be found", id)
		}
		if attempt.Status == AttemptSucceeded {
			t.Fatalf("%s must never report a successful self-test without a real runtime", id)
		}
		if attempt.Status != AttemptSetupRequired {
			t.Fatalf("%s self-test must be setup_required when unconfigured, got %s", id, attempt.Status)
		}
	}
}

func TestExternalRuntimeExecuteRefuses(t *testing.T) {
	s := newTestService(t)
	a, _ := s.reg.Adapter("hermes")
	if _, err := a.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatalf("hermes Execute must refuse (no fake execution)")
	}
	// Browser contract refuses too, and its forbidden boundary is published.
	b, _ := s.reg.Adapter("browser-runtime")
	if _, err := b.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatalf("browser contract Execute must refuse")
	}
	foundForbidden := false
	for _, cap := range b.Capabilities() {
		if cap == "forbidden:send_message" {
			foundForbidden = true
		}
	}
	if !foundForbidden {
		t.Fatalf("browser contract must publish its forbidden boundary")
	}
}

func TestAttemptsRecorded(t *testing.T) {
	s := newTestService(t)
	s.SelfTest(context.Background(), executionbroker.LocalSafeWorkerID)
	s.SelfTest(context.Background(), executionbroker.LocalSafeWorkerID)
	if len(s.Attempts(executionbroker.LocalSafeWorkerID)) != 2 {
		t.Fatalf("both self-test attempts must be recorded")
	}
}

func TestValidateURLRejectsMetadata(t *testing.T) {
	t.Setenv(runtimeLabAllowedHostsEnv, defaultRuntimeLabAllowedHosts)
	for _, u := range []string{"http://169.254.169.254/", "ftp://x", "http://0.0.0.0/"} {
		if err := validateURL(u); err == nil {
			t.Fatalf("%q must be rejected", u)
		}
	}
	if err := validateURL("http://localhost:9000"); err != nil {
		t.Fatalf("localhost must be allowed: %v", err)
	}
}

func TestValidateURLRequiresExplicitRuntimeLabHost(t *testing.T) {
	t.Setenv(runtimeLabAllowedHostsEnv, "localhost,agent.local")
	if err := validateURL("https://agent.local/runtime"); err != nil {
		t.Fatalf("explicitly allowed host must be accepted: %v", err)
	}
	for _, raw := range []string{
		"https://unreviewed.example/runtime",
		"https://agent.local/runtime?token=secret",
		"https://user:secret@agent.local/runtime",
	} {
		if err := validateURL(raw); err == nil {
			t.Fatalf("%q must be rejected by Runtime Lab URL validation", raw)
		}
	}
}

func TestOpenHandsHealthProbeIsRealButExecutionRemainsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("OPENHANDS_BASE_URL", server.URL)
	t.Setenv("OPENHANDS_HEALTH_PATH", "/health")
	runtime := newRemoteRuntime("openhands", "OpenHands", "OPENHANDS_BASE_URL")

	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeReady {
		t.Fatalf("configured OpenHands health endpoint must be probed truthfully: %#v", probe)
	}
	if _, err := runtime.Execute(context.Background(), map[string]any{"task": "do not run"}); err == nil {
		t.Fatal("a healthy OpenHands endpoint must not enable task execution")
	}
}

func TestRemoteHealthProbeDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://unreviewed.example/health", http.StatusFound)
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("OPENHANDS_BASE_URL", server.URL)
	t.Setenv("OPENHANDS_HEALTH_PATH", "/health")
	runtime := newRemoteRuntime("openhands", "OpenHands", "OPENHANDS_BASE_URL")

	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeUnavailable || probe.Detail != "health HTTP 302" {
		t.Fatalf("redirecting health endpoint must not be followed: %#v", probe)
	}
}
