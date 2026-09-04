package agentruntime

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/hostruntime"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "chat" || os.Args[1] == "agent" || os.Args[1] == "--profile") {
		for _, arg := range os.Args[1:] {
			fmt.Fprintln(os.Stdout, arg)
		}
		for _, key := range []string{
			"HERMES_HOME",
			"HERMES_PROFILE",
			"HERMES_IGNORE_USER_CONFIG",
			"DSH_HOME",
			"TERMINAL_CWD",
			"OPENCLAW_STATE_DIR",
			"OPENCLAW_HOME",
			"HAI_RUNTIME_TASK_ID",
		} {
			if value, ok := os.LookupEnv(key); ok {
				fmt.Fprintf(os.Stdout, "%s=%s\n", key, value)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRegistryRequiresApproval(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := NewRegistry(adapter)
	result := registry.Execute(context.Background(), "test", Task{
		ID:            "task-1",
		Prompt:        "do work",
		OwnerIdentity: "alice",
	})
	if result.Status != "blocked" || adapter.called {
		t.Fatalf("unapproved task was executed: %#v", result)
	}
}

func TestRegistryRejectsCallerControlledApprovalFlagWithoutProvenance(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := NewRegistry(adapter)

	for _, task := range []Task{
		{ID: "task-1", Prompt: "do work", OwnerIdentity: "alice", HumanApproved: true},
		{ID: "task-2", Prompt: "do work", OwnerIdentity: "alice", HumanApproved: true, ApprovalSourceID: "workflow-decision:forged"},
		{ID: "task-3", Prompt: "do work", HumanApproved: true, ApprovalSourceID: "task-review:11111111-1111-4111-8111-111111111111"},
	} {
		result := registry.Execute(context.Background(), "test", task)
		if result.Status != "blocked" || adapter.called {
			t.Fatalf("unproven approval task executed: task=%#v result=%#v", task, result)
		}
	}
}

func TestRegistryBlocksWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := newVerifiedTestRegistry(adapter)
	result := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "run approved work"))
	if result.Status != "blocked" || adapter.called {
		t.Fatalf("emergency stop did not prevent runtime execution: %#v", result)
	}
	if !strings.Contains(result.Message, "emergency stop") || !containsString(result.AuditEvents, "emergency stop blocked agent runtime execution") {
		t.Fatalf("emergency stop result lacks controlled audit evidence: %#v", result)
	}
}

func TestRegistryBlocksWhenPersistedEmergencyStopActive(t *testing.T) {
	restore := safety.SetEmergencyStopProvider(safety.EmergencyStopProviderFunc(func() (bool, string, error) {
		return true, "operator paused execution", nil
	}))
	defer restore()

	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := newVerifiedTestRegistry(adapter)
	result := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "run approved work"))
	if result.Status != "blocked" || adapter.called {
		t.Fatalf("persisted emergency stop did not prevent runtime execution: %#v", result)
	}
	if result.Message != "operator paused execution" {
		t.Fatalf("blocked reason = %q", result.Message)
	}
}

func TestRegistryExecutesApprovedTask(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	task := approvedRuntimeTask("task-1", "do work")
	task.ProjectKey = "project-1"
	task = withValidFinalEffectProof("test", task, adapter.info)
	var captured FinalEffectAuthorizationRequest
	var capturedProof FinalEffectAuthorizationProof
	registry := NewRegistryWithFinalEffectVerifier(
		FinalEffectProofVerifierFunc(func(_ context.Context, request FinalEffectAuthorizationRequest, proof FinalEffectAuthorizationProof) error {
			captured = request
			capturedProof = proof
			return verifyTestFinalEffectProof(request, proof)
		}),
		adapter,
	)
	result := registry.Execute(context.Background(), "test", task)
	if result.Status != "completed" || !adapter.called {
		t.Fatalf("approved task was not executed: %#v", result)
	}
	sum := sha256.Sum256([]byte(task.Prompt))
	if captured.Operation != runtimeExecuteTaskOperation ||
		captured.RuntimeID != "test" ||
		captured.TaskID != task.ID ||
		captured.OwnerIdentity != task.OwnerIdentity ||
		captured.ProjectKey != task.ProjectKey ||
		captured.ApprovalSourceID != task.ApprovalSourceID ||
		captured.PromptDigest != hex.EncodeToString(sum[:]) ||
		!captured.RequiresApproval {
		t.Fatalf("final-effect request was not exactly bound to the task: %#v", captured)
	}
	if capturedProof != task.FinalEffectProof {
		t.Fatalf("verifier did not receive the task proof: got=%#v want=%#v", capturedProof, task.FinalEffectProof)
	}
	if !containsString(result.AuditEvents, "runtime adapter invoked with verified consumed authorization proof") {
		t.Fatalf("result lacks final-effect authorization evidence: %#v", result.AuditEvents)
	}
}

func TestRegistryFailsClosedWithoutFinalEffectVerifier(t *testing.T) {
	adapter := executableFakeAdapter()
	registry := NewRegistry(adapter)

	result := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))

	if result.Status != "blocked" || adapter.called {
		t.Fatalf("registry without verifier reached adapter: result=%#v called=%t", result, adapter.called)
	}
	if !strings.Contains(result.Message, "could not be verified") ||
		!containsString(result.AuditEvents, "final-effect proof verifier failed closed before runtime adapter access") {
		t.Fatalf("fail-closed result lacks controlled evidence: %#v", result)
	}
}

func TestRegistryFinalEffectProofFailuresNeverReachAdapter(t *testing.T) {
	tests := []struct {
		name              string
		mutate            func(*FinalEffectAuthorizationProof)
		verifierErr       error
		wantVerifierCalls int
	}{
		{
			name: "missing receipt",
			mutate: func(proof *FinalEffectAuthorizationProof) {
				proof.ReceiptID = ""
			},
		},
		{
			name: "invalid authorization request digest",
			mutate: func(proof *FinalEffectAuthorizationProof) {
				proof.AuthorizationRequestDigest = "not-a-sha256-digest"
			},
		},
		{
			name: "invalid decision digest",
			mutate: func(proof *FinalEffectAuthorizationProof) {
				proof.DecisionDigest = "not-a-sha256-digest"
			},
		},
		{
			name: "runtime binding belongs to another request",
			mutate: func(proof *FinalEffectAuthorizationProof) {
				proof.RuntimeRequestDigest = strings.Repeat("b", 64)
			},
		},
		{
			name: "arbitrary syntactically valid receipt rejected by durable verifier",
			mutate: func(proof *FinalEffectAuthorizationProof) {
				proof.ReceiptID = "22222222-2222-4222-8222-222222222222"
			},
			verifierErr:       errors.New("receipt not found"),
			wantVerifierCalls: 1,
		},
		{
			name:              "verifier unavailable",
			verifierErr:       errors.New("policy database unavailable"),
			wantVerifierCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := executableFakeAdapter()
			task := approvedRuntimeTask("task-1", "do work")
			if test.mutate != nil {
				test.mutate(&task.FinalEffectProof)
			}
			verifierCalls := 0
			registry := NewRegistryWithFinalEffectVerifier(
				FinalEffectProofVerifierFunc(func(_ context.Context, request FinalEffectAuthorizationRequest, proof FinalEffectAuthorizationProof) error {
					verifierCalls++
					if test.verifierErr != nil {
						return test.verifierErr
					}
					return verifyTestFinalEffectProof(request, proof)
				}),
				adapter,
			)

			result := registry.Execute(context.Background(), "test", task)

			if result.Status != "blocked" || adapter.called {
				t.Fatalf("invalid proof reached adapter: result=%#v called=%t", result, adapter.called)
			}
			if verifierCalls != test.wantVerifierCalls {
				t.Fatalf("verifier calls=%d want=%d", verifierCalls, test.wantVerifierCalls)
			}
			if strings.Contains(result.Message, "policy database unavailable") ||
				strings.Contains(result.Message, "receipt not found") {
				t.Fatalf("internal verifier error leaked to caller: %#v", result)
			}
		})
	}
}

func TestRegistryRechecksEmergencyStopAfterFinalEffectProofVerification(t *testing.T) {
	engaged := false
	restore := safety.SetEmergencyStopProvider(safety.EmergencyStopProviderFunc(func() (bool, string, error) {
		return engaged, "operator engaged stop during proof verification", nil
	}))
	defer restore()

	adapter := executableFakeAdapter()
	registry := NewRegistryWithFinalEffectVerifier(
		FinalEffectProofVerifierFunc(func(_ context.Context, request FinalEffectAuthorizationRequest, proof FinalEffectAuthorizationProof) error {
			if err := verifyTestFinalEffectProof(request, proof); err != nil {
				return err
			}
			engaged = true
			return nil
		}),
		adapter,
	)

	result := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))

	if result.Status != "blocked" || adapter.called {
		t.Fatalf("stop engaged after authorization reached adapter: result=%#v called=%t", result, adapter.called)
	}
	if result.Message != "operator engaged stop during proof verification" ||
		!containsString(result.AuditEvents, "verified final-effect proof was not exercised") {
		t.Fatalf("post-authorization emergency-stop evidence missing: %#v", result)
	}
}

func TestRegistryCancellationDuringProofVerificationNeverReachesAdapter(t *testing.T) {
	adapter := executableFakeAdapter()
	verificationStarted := make(chan struct{})
	releaseVerification := make(chan struct{})
	registry := NewRegistryWithFinalEffectVerifier(
		FinalEffectProofVerifierFunc(func(_ context.Context, request FinalEffectAuthorizationRequest, proof FinalEffectAuthorizationProof) error {
			close(verificationStarted)
			<-releaseVerification
			return verifyTestFinalEffectProof(request, proof)
		}),
		adapter,
	)
	resultCh := make(chan Result, 1)
	go func() {
		resultCh <- registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))
	}()

	select {
	case <-verificationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("final-effect proof verification did not start")
	}
	stop := registry.StopTask(context.Background(), "test", "task-1", "alice")
	if stop.Status != "cancellation_requested" || !strings.Contains(stop.Message, "delivery is not yet verified") {
		t.Fatalf("stop while verifying proof = %#v", stop)
	}
	close(releaseVerification)

	select {
	case result := <-resultCh:
		if result.Status != "blocked" || adapter.called {
			t.Fatalf("cancelled proof verification reached adapter: result=%#v called=%t", result, adapter.called)
		}
		if !containsString(result.AuditEvents, "runtime cancellation observed after final-effect proof verification") {
			t.Fatalf("cancellation evidence missing: %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled execution did not return")
	}
}

func TestRegistryReadAPIsRemainAvailableWithoutFinalEffectVerifier(t *testing.T) {
	adapter := executableFakeAdapter()
	adapter.info.ID = "hermes"
	registry := NewRegistry(adapter)

	if got := registry.List(); len(got) != 1 || got[0].ID != "hermes" {
		t.Fatalf("runtime discovery should not require execution authorization: %#v", got)
	}
	health := registry.Health(context.Background())
	if len(health) != 1 || health[0].RuntimeID != "hermes" || health[0].Status != "ready" {
		t.Fatalf("health should not require execution authorization: %#v", health)
	}
	skills, err := registry.Skills(context.Background(), "hermes")
	if err != nil || len(skills) != 1 {
		t.Fatalf("skills should not require execution authorization: skills=%#v err=%v", skills, err)
	}
}

func TestRegistryHealthForChecksOnlyTheRequestedRuntime(t *testing.T) {
	openClaw := &fakeAdapter{info: Info{ID: "openclaw"}}
	hermes := &fakeAdapter{info: Info{ID: "hermes"}}
	registry := NewRegistry(openClaw, hermes)

	health, ok := registry.HealthFor(context.Background(), "openclaw")
	if !ok || health.RuntimeID != "openclaw" {
		t.Fatalf("HealthFor(openclaw) = (%#v, %t)", health, ok)
	}
	if openClaw.healthCalls != 1 || hermes.healthCalls != 0 {
		t.Fatalf("HealthFor must not probe unrelated runtimes: openclaw=%d hermes=%d", openClaw.healthCalls, hermes.healthCalls)
	}
}

func TestBindConsumedAuthorizationProofBindsExactTaskAndRejectsMutation(t *testing.T) {
	adapter := executableFakeAdapter()
	binder := NewRegistry(adapter)
	task := approvedRuntimeTask("task-1", "do work")
	task.FinalEffectProof = FinalEffectAuthorizationProof{}

	bound, err := binder.BindConsumedAuthorizationProof(
		"test",
		task,
		"11111111-1111-4111-8111-111111111111",
		strings.Repeat("c", 64),
		strings.Repeat("a", 64),
		"signed-runtime-handoff",
	)
	if err != nil {
		t.Fatalf("bind consumed authorization proof: %v", err)
	}
	expectedRequest := runtimeFinalEffectRequest("test", bound, adapter.info)
	if bound.FinalEffectProof.RuntimeRequestDigest != finalEffectRequestDigest(expectedRequest) ||
		bound.FinalEffectProof.RuntimeProof != "signed-runtime-handoff" {
		t.Fatalf("runtime proof was not bound exactly: %#v", bound.FinalEffectProof)
	}

	verifierCalls := 0
	registry := NewRegistryWithFinalEffectVerifier(
		FinalEffectProofVerifierFunc(func(context.Context, FinalEffectAuthorizationRequest, FinalEffectAuthorizationProof) error {
			verifierCalls++
			return nil
		}),
		adapter,
	)
	bound.Prompt = "mutated after authorization handoff"
	result := registry.Execute(context.Background(), "test", bound)
	if result.Status != "blocked" || adapter.called || verifierCalls != 0 {
		t.Fatalf("mutated task crossed proof boundary: result=%#v called=%t verifierCalls=%d", result, adapter.called, verifierCalls)
	}

	if _, err := binder.BindConsumedAuthorizationProof(
		"test",
		task,
		"arbitrary-id",
		strings.Repeat("c", 64),
		strings.Repeat("a", 64),
		"",
	); err == nil {
		t.Fatal("binder accepted malformed receipt id")
	}
}

func TestRegistryBlockMessageIncludesConfigurationReasons(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:                   "test",
		Enabled:              true,
		Configured:           false,
		ExecutionEnabled:     false,
		RequiresApproval:     true,
		MissingConfiguration: []string{"TEST_WORKSPACE", "runtime endpoint missing"},
	}}
	registry := newVerifiedTestRegistry(adapter)
	result := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))
	if result.Status != "blocked" || adapter.called {
		t.Fatalf("misconfigured runtime should not execute: %#v", result)
	}
	for _, expected := range []string{"runtime not configured", "execution disabled", "TEST_WORKSPACE", "runtime endpoint missing"} {
		if !strings.Contains(result.Message, expected) {
			t.Fatalf("block message %q missing %q", result.Message, expected)
		}
	}
	if !strings.Contains(strings.Join(result.AuditEvents, " "), "runtime registry policy blocked execution") {
		t.Fatalf("expected registry policy audit event: %#v", result.AuditEvents)
	}
}

func TestRegistryStopTaskCancelsActiveRuntimeExecution(t *testing.T) {
	adapter := &blockingAdapter{
		info: Info{
			ID:               "test",
			Enabled:          true,
			Configured:       true,
			ExecutionEnabled: true,
			RequiresApproval: true,
		},
		started: make(chan struct{}),
	}
	registry := newVerifiedTestRegistry(adapter)
	resultCh := make(chan Result, 1)
	go func() {
		resultCh <- registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))
	}()

	select {
	case <-adapter.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("runtime task did not start")
	}

	wrongOwner := registry.StopTask(context.Background(), "test", "task-1", "bob")
	if wrongOwner.Status != "blocked" || !strings.Contains(wrongOwner.Message, "different owner") {
		t.Fatalf("cross-owner stop result = %#v", wrongOwner)
	}

	stop := registry.StopTask(context.Background(), "test", "task-1", "alice")
	if stop.Status != "cancellation_requested" || !strings.Contains(stop.Message, "delivery is not yet verified") {
		t.Fatalf("stop result = %#v", stop)
	}
	if !containsString(stop.AuditEvents, "runtime cancellation delivery is not yet verified") {
		t.Fatalf("stop request must not claim a verified downstream stop: %#v", stop.AuditEvents)
	}

	select {
	case result := <-resultCh:
		if result.Status != "blocked" || !strings.Contains(result.Message, "cancelled") {
			t.Fatalf("runtime result after stop = %#v", result)
		}
		if !containsString(result.AuditEvents, "runtime registry cancellation observed") {
			t.Fatalf("runtime cancellation audit missing: %#v", result.AuditEvents)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runtime task was not cancelled")
	}
}

func TestRegistryRejectsDuplicateActiveRuntimeTaskID(t *testing.T) {
	adapter := &blockingAdapter{
		info: Info{
			ID:               "test",
			Enabled:          true,
			Configured:       true,
			ExecutionEnabled: true,
			RequiresApproval: true,
		},
		started: make(chan struct{}),
	}
	registry := newVerifiedTestRegistry(adapter)
	resultCh := make(chan Result, 1)
	go func() {
		resultCh <- registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do work"))
	}()

	select {
	case <-adapter.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("runtime task did not start")
	}

	duplicate := registry.Execute(context.Background(), "test", approvedRuntimeTask("task-1", "do duplicate work"))
	if duplicate.Status != "blocked" || !strings.Contains(duplicate.Message, "already running") {
		t.Fatalf("duplicate task result = %#v", duplicate)
	}

	_ = registry.StopTask(context.Background(), "test", "task-1", "alice")
	select {
	case <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("runtime task was not cancelled")
	}
}

func TestOdysseusAdapterUsesAgentModeWithoutBash(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/codex/capabilities" {
			_, _ = w.Write([]byte(`{"integration":"codex"}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"delta\":\"completed safely\"}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := &odysseusAdapter{
		enabled:     true,
		baseURL:     server.URL,
		token:       "test-token",
		sessionID:   "session-1",
		timeout:     30 * time.Second,
		outputLimit: defaultOutputLimit,
		allowedHost: map[string]bool{"127.0.0.1": true},
	}
	result := adapter.ExecuteTask(context.Background(), Task{Prompt: "inspect the task", HumanApproved: true})
	if result.Status != "completed" || result.Output != "completed safely" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{"mode=agent", "allow_bash=false", "allow_web_search=false"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("request body %q missing %q", body, expected)
		}
	}
}

func TestOdysseusStreamRejectsTruncatedOutput(t *testing.T) {
	if _, err := readOdysseusStream(strings.NewReader("data: {\"delta\":\"partial\"}\n\n"), 4096); err == nil {
		t.Fatalf("expected incomplete stream to be rejected")
	}
}

func TestOdysseusInfoAdvertisesEcosystemAndControls(t *testing.T) {
	adapter := &odysseusAdapter{
		enabled:                    true,
		baseURL:                    "http://127.0.0.1:7000",
		token:                      "scoped-token",
		sessionID:                  "session-1",
		timeout:                    30 * time.Second,
		outputLimit:                defaultOutputLimit,
		allowedHost:                map[string]bool{"127.0.0.1": true},
		todosEnabled:               true,
		emailEnabled:               true,
		calendarEnabled:            true,
		documentsEnabled:           true,
		memorySyncEnabled:          true,
		researchEnabled:            true,
		searchEnabled:              true,
		mcpEnabled:                 true,
		cookbookEnabled:            true,
		localModelDiscoveryEnabled: true,
		shellEnabled:               true,
		codexBridgeEnabled:         true,
		claudeBridgeEnabled:        true,
		agentMigrationEnabled:      true,
		contextBudgetEnabled:       true,
	}
	info := adapter.Info()
	if !info.Configured || !info.ExecutionEnabled {
		t.Fatalf("expected configured Odysseus adapter: %#v", info)
	}
	joinedCapabilities := strings.Join(info.Capabilities, " ")
	for _, expected := range []string{"scoped Codex API", "MCP manager", "Cookbook model-serving", "Codex and Claude bridge", "context budget"} {
		if !strings.Contains(joinedCapabilities, expected) {
			t.Fatalf("Odysseus capability %q not advertised: %#v", expected, info.Capabilities)
		}
	}
	joinedControls := strings.Join(info.Controls, " ")
	for _, expected := range []string{"server-side HAI approval", "scoped ODYSSEUS_API_TOKEN", "AGENT_RUNTIME_ALLOWED_HOSTS", "allow_bash=false"} {
		if !strings.Contains(joinedControls, expected) {
			t.Fatalf("Odysseus control %q not advertised: %#v", expected, info.Controls)
		}
	}
	if len(info.Architecture) == 0 {
		t.Fatalf("expected Odysseus architecture chain to be visible")
	}
}

func TestHermesWorkspaceMustStayInsideRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	adapter := &hermesAdapter{
		workspaceRoot: root,
		workspace:     root + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "outside",
	}
	if reason := adapter.workspaceBlockedReason(); reason == "" {
		t.Fatalf("expected workspace escape to be rejected")
	}
}

func TestHermesInfoAdvertisesEcosystemAndControls(t *testing.T) {
	adapter := &hermesAdapter{
		enabled:          true,
		executable:       "hermes",
		workspace:        t.TempDir(),
		workspaceRoot:    t.TempDir(),
		toolsets:         []string{"safe", "skills"},
		skills:           []string{"legal-drafting"},
		terminalBackends: []string{"local", "docker"},
		gatewayEnabled:   true,
		mcpEnabled:       true,
	}
	info := adapter.Info()
	joinedCapabilities := strings.Join(info.Capabilities, " ")
	for _, expected := range []string{"skills and skill learning", "MCP servers and tools", "gateway channels", "subagent delegation", "ACP adapter"} {
		if !strings.Contains(joinedCapabilities, expected) {
			t.Fatalf("Hermes capability %q not advertised: %#v", expected, info.Capabilities)
		}
	}
	joinedControls := strings.Join(info.Controls, " ")
	for _, expected := range []string{"server-side HAI approval", "AGENT_RUNTIME_WORKSPACE_ROOT", "HERMES_TOOLSETS=safe,skills", "HERMES_SKILLS=legal-drafting"} {
		if !strings.Contains(joinedControls, expected) {
			t.Fatalf("Hermes control %q not advertised: %#v", expected, info.Controls)
		}
	}
	if len(info.Architecture) == 0 {
		t.Fatalf("expected Hermes architecture chain to be visible")
	}
}

func TestHermesAdapterInvokesControlledCli(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve native test executable: %v", err)
	}
	adapter := &hermesAdapter{
		enabled:          true,
		executable:       executable,
		home:             home,
		profile:          "hai",
		workspace:        workspace,
		workspaceRoot:    root,
		maxTurns:         3,
		timeout:          30 * time.Second,
		toolsets:         []string{"safe"},
		skills:           []string{"legal-drafting"},
		outputLimit:      defaultOutputLimit,
		ignoreUserConfig: true,
		terminalBackends: []string{"local"},
	}

	result := adapter.ExecuteTask(context.Background(), Task{ID: "task-1", Prompt: "draft safely", ProjectKey: "case-1"})
	if result.Status != "completed" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{
		"chat", "-q", "draft safely", "-Q", "--source", "tool", "--max-turns", "3", "--checkpoints",
		"--toolsets", "safe", "--skills", "legal-drafting",
		"HERMES_HOME=" + home, "HERMES_PROFILE=hai", "HERMES_IGNORE_USER_CONFIG=1", "TERMINAL_CWD=" + workspace,
	} {
		if !strings.Contains(result.Output, expected) {
			t.Fatalf("output %q missing %q", result.Output, expected)
		}
	}
	if strings.Contains(result.Output, "--yolo") {
		t.Fatalf("Hermes execution must not use --yolo: %q", result.Output)
	}
}

func TestHermesExecutionFailureDoesNotEchoTaskPrompt(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	sensitivePrompt := "sensitive prompt: do-not-expose-6f7d8a"
	adapter := &hermesAdapter{
		enabled:       true,
		executable:    filepath.Join(root, "missing-hermes-executable"),
		workspace:     workspace,
		workspaceRoot: root,
		timeout:       time.Second,
		outputLimit:   defaultOutputLimit,
	}

	result := adapter.ExecuteTask(context.Background(), Task{ID: "task-1", Prompt: sensitivePrompt})
	if result.Status != "failed" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(result.Message, sensitivePrompt) {
		t.Fatalf("execution error leaked task prompt: %#v", result)
	}
	if result.Message != "Hermes process failed without diagnostic output" {
		t.Fatalf("unexpected failure message: %#v", result)
	}
}

func TestOpenClawInfoAdvertisesEcosystemAndControls(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "openclaw")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	adapter := &openClawAdapter{
		enabled:            true,
		executable:         "openclaw",
		workspace:          workspace,
		workspaceRoot:      root,
		gatewayURL:         "ws://127.0.0.1:18789",
		gatewayToken:       "gateway-token",
		thinking:           "high",
		timeout:            30 * time.Second,
		outputLimit:        defaultOutputLimit,
		allowedHost:        map[string]bool{"127.0.0.1": true},
		agentCLIEnabled:    true,
		gatewayEnabled:     true,
		messagesEnabled:    true,
		skillsEnabled:      true,
		pluginsEnabled:     true,
		mcpEnabled:         true,
		memoryEnabled:      true,
		cronEnabled:        true,
		browserEnabled:     true,
		canvasEnabled:      true,
		nodesEnabled:       true,
		voiceEnabled:       true,
		talkEnabled:        true,
		webchatEnabled:     true,
		multiAgentEnabled:  true,
		localModelsEnabled: true,
		highRiskExecution:  true,
		sandboxRequired:    true,
		sandboxMode:        "all",
		sandboxDocker:      true,
		channelsEnabled:    []string{"whatsapp", "telegram"},
		providersEnabled:   []string{"ollama", "openrouter"},
		companionApps:      []string{"windows", "android"},
	}
	info := adapter.Info()
	if !info.Configured || !info.ExecutionEnabled {
		t.Fatalf("expected configured OpenClaw adapter: %#v", info)
	}
	joinedCapabilities := strings.Join(info.Capabilities, " ")
	for _, expected := range []string{"local-first Gateway", "multi-channel inbox", "multi-agent session routing", "skills, ClawHub packages", "Live Canvas", "sandbox backends"} {
		if !strings.Contains(joinedCapabilities, expected) {
			t.Fatalf("OpenClaw capability %q not advertised: %#v", expected, info.Capabilities)
		}
	}
	joinedControls := strings.Join(info.Controls, " ")
	for _, expected := range []string{"server-side HAI approval", "openclaw agent --message", "AGENT_RUNTIME_WORKSPACE_ROOT", "OPENCLAW_GATEWAY_TOKEN"} {
		if !strings.Contains(joinedControls, expected) {
			t.Fatalf("OpenClaw control %q not advertised: %#v", expected, info.Controls)
		}
	}
	if len(info.Architecture) == 0 {
		t.Fatalf("expected OpenClaw architecture chain to be visible")
	}
}

func TestOpenClawCompanionGatewayHealthDoesNotRequireHostCLI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/health" {
			t.Fatalf("unexpected companion health request: %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	}))
	defer server.Close()

	gatewayURL := "ws" + strings.TrimPrefix(server.URL, "http")
	adapter := &openClawAdapter{
		enabled:        true,
		gatewayEnabled: true,
		gatewayURL:     gatewayURL,
		allowedHost:    map[string]bool{"127.0.0.1": true},
		timeout:        time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "available" {
		t.Fatalf("companion gateway health = %#v, want available", health)
	}
	if !strings.Contains(health.Reason, "gateway health endpoint is live") {
		t.Fatalf("companion gateway reason = %q", health.Reason)
	}
	if requests != 1 {
		t.Fatalf("companion health request count = %d, want 1", requests)
	}
}

func TestOpenClawHealthOnlyGatewayDoesNotRequireTokenForControlledCLIExecution(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve native test executable: %v", err)
	}
	adapter := &openClawAdapter{
		enabled:         true,
		executable:      executable,
		workspace:       workspace,
		workspaceRoot:   root,
		gatewayEnabled:  true,
		gatewayURL:      "ws://127.0.0.1:18789",
		allowedHost:     map[string]bool{"127.0.0.1": true},
		agentCLIEnabled: true,
		sandboxRequired: true,
		sandboxMode:     "all",
		timeout:         time.Second,
		outputLimit:     defaultOutputLimit,
	}

	info := adapter.Info()
	if !info.Configured || !info.ExecutionEnabled {
		t.Fatalf("health-only gateway should not disable separately governed CLI execution: %#v", info)
	}
	result := adapter.ExecuteTask(context.Background(), Task{ID: "task-1", Prompt: "inspect local work"})
	if result.Status != "completed" {
		t.Fatalf("health-only gateway should not block CLI execution: %#v", result)
	}
}

func TestOpenClawCompanionGatewayHealthRejectsUnexpectedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"starting"}`))
	}))
	defer server.Close()

	adapter := &openClawAdapter{
		enabled:        true,
		gatewayEnabled: true,
		gatewayURL:     "ws" + strings.TrimPrefix(server.URL, "http"),
		allowedHost:    map[string]bool{"127.0.0.1": true},
		timeout:        time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "unavailable" || !strings.Contains(health.Reason, "unexpected health response") {
		t.Fatalf("unexpected companion gateway health = %#v", health)
	}
}

func TestOpenClawGatewayTaskLedgerDiscoveryRequiresAuthenticatedDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	}))
	defer server.Close()

	adapter := &openClawAdapter{
		enabled:                           true,
		gatewayEnabled:                    true,
		gatewayTaskLedgerDiscoveryEnabled: true,
		gatewayURL:                        "ws" + strings.TrimPrefix(server.URL, "http"),
		allowedHost:                       map[string]bool{"127.0.0.1": true},
		timeout:                           time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "blocked" || !strings.Contains(health.Reason, "OPENCLAW_GATEWAY_AUTH_DISCOVERY_ENABLED") {
		t.Fatalf("task-ledger discovery without authenticated discovery = %#v", health)
	}
}

func TestOpenClawGatewayTaskLedgerDiscoveryRequiresToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	}))
	defer server.Close()

	adapter := &openClawAdapter{
		enabled:                              true,
		gatewayEnabled:                       true,
		gatewayAuthenticatedDiscoveryEnabled: true,
		gatewayTaskLedgerDiscoveryEnabled:    true,
		gatewayURL:                           "ws" + strings.TrimPrefix(server.URL, "http"),
		allowedHost:                          map[string]bool{"127.0.0.1": true},
		timeout:                              time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "blocked" || !strings.Contains(health.Reason, "OPENCLAW_GATEWAY_TOKEN") {
		t.Fatalf("task-ledger discovery without token = %#v", health)
	}
}

func TestOpenClawCompanionGatewayHealthRejectsCredentialBearingURL(t *testing.T) {
	adapter := &openClawAdapter{
		enabled:        true,
		gatewayEnabled: true,
		gatewayURL:     "ws://gateway-token@127.0.0.1:18789",
		allowedHost:    map[string]bool{"127.0.0.1": true},
		timeout:        time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "blocked" || !strings.Contains(health.Reason, "must not include credentials") {
		t.Fatalf("credential-bearing companion gateway URL health = %#v", health)
	}
}

func TestOpenClawCompanionGatewayProtocolChallengeIsReadOnly(t *testing.T) {
	var receivedFrame string
	var authorizationHeader string
	gateway := websocket.Server{
		Handshake: func(_ *websocket.Config, request *http.Request) error {
			authorizationHeader = request.Header.Get("Authorization")
			return nil
		},
		Handler: func(connection *websocket.Conn) {
			if err := websocket.JSON.Send(connection, map[string]any{
				"type":    "event",
				"event":   "connect.challenge",
				"payload": map[string]any{"nonce": "challenge-nonce", "ts": float64(1_737_264_000_000)},
			}); err != nil {
				t.Errorf("send protocol challenge: %v", err)
				return
			}
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			if err := websocket.Message.Receive(connection, &receivedFrame); err != nil && !errors.Is(err, io.EOF) {
				t.Errorf("receive unexpected client frame: %v", err)
			}
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	})
	mux.Handle("/", gateway)
	server := httptest.NewServer(mux)
	defer server.Close()

	adapter := &openClawAdapter{
		enabled:                         true,
		gatewayEnabled:                  true,
		gatewayProtocolDiscoveryEnabled: true,
		gatewayURL:                      "ws" + strings.TrimPrefix(server.URL, "http"),
		gatewayToken:                    "must-not-be-sent",
		allowedHost:                     map[string]bool{"127.0.0.1": true},
		timeout:                         time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "available" || !strings.Contains(health.Reason, "protocol challenge was verified") {
		t.Fatalf("companion gateway protocol health = %#v", health)
	}
	if receivedFrame != "" {
		t.Fatalf("challenge probe sent a client frame: %q", receivedFrame)
	}
	if authorizationHeader != "" {
		t.Fatalf("challenge probe sent an authorization header: %q", authorizationHeader)
	}
}

func TestOpenClawCompanionGatewayProtocolChallengeRejectsMalformedFrame(t *testing.T) {
	gateway := websocket.Server{
		Handshake: func(_ *websocket.Config, _ *http.Request) error { return nil },
		Handler: func(connection *websocket.Conn) {
			_ = websocket.JSON.Send(connection, map[string]any{
				"type":    "event",
				"event":   "unexpected.event",
				"payload": map[string]any{"nonce": "challenge-nonce", "ts": float64(1)},
			})
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	})
	mux.Handle("/", gateway)
	server := httptest.NewServer(mux)
	defer server.Close()

	adapter := &openClawAdapter{
		enabled:                         true,
		gatewayEnabled:                  true,
		gatewayProtocolDiscoveryEnabled: true,
		gatewayURL:                      "ws" + strings.TrimPrefix(server.URL, "http"),
		allowedHost:                     map[string]bool{"127.0.0.1": true},
		timeout:                         time.Second,
	}

	health := adapter.HealthCheck(context.Background())
	if health.Status != "unavailable" || !strings.Contains(health.Reason, "protocol challenge") {
		t.Fatalf("malformed companion gateway protocol health = %#v", health)
	}
}

func TestOpenClawCompanionGatewayAuthenticatedReadDiscoveryUsesBoundedOperatorHandshake(t *testing.T) {
	var receivedFrame map[string]any
	var authorizationHeader string
	done := make(chan struct{})
	gateway := websocket.Server{
		Handshake: func(_ *websocket.Config, request *http.Request) error {
			authorizationHeader = request.Header.Get("Authorization")
			return nil
		},
		Handler: func(connection *websocket.Conn) {
			defer close(done)
			if err := websocket.JSON.Send(connection, map[string]any{
				"type":    "event",
				"event":   "connect.challenge",
				"payload": map[string]any{"nonce": "challenge-nonce", "ts": float64(1_737_264_000_000)},
			}); err != nil {
				t.Errorf("send protocol challenge: %v", err)
				return
			}
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			if err := websocket.JSON.Receive(connection, &receivedFrame); err != nil {
				return
			}
			requestID, _ := receivedFrame["id"].(string)
			if err := websocket.JSON.Send(connection, map[string]any{
				"type": "res",
				"id":   requestID,
				"ok":   true,
				"payload": map[string]any{
					"type":     "hello-ok",
					"protocol": float64(4),
					"server":   map[string]any{"version": "2026.8.1", "connId": "connection-1"},
					"features": map[string]any{"methods": []string{"status"}, "events": []string{}},
					"snapshot": map[string]any{},
					"auth":     map[string]any{"role": "operator", "scopes": []string{"operator.read"}},
					"policy":   map[string]any{"maxPayload": float64(65536), "maxBufferedBytes": float64(65536)},
				},
			}); err != nil {
				t.Errorf("send hello response: %v", err)
			}
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	})
	mux.Handle("/", gateway)
	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("OPENCLAW_AGENT_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_PROTOCOL_DISCOVERY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_AUTH_DISCOVERY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_TOKEN", "gateway-read-token")
	t.Setenv("OPENCLAW_GATEWAY_URL", "ws"+strings.TrimPrefix(server.URL, "http"))
	t.Setenv("AGENT_RUNTIME_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("OPENCLAW_TIMEOUT_SECONDS", "1")

	health := newOpenClawAdapterFromEnv().HealthCheck(context.Background())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gateway did not finish the authenticated handshake")
	}
	if health.Status != "available" || !strings.Contains(health.Reason, "authenticated operator.read") {
		t.Fatalf("authenticated discovery health = %#v", health)
	}
	if health.Version != "2026.8.1" {
		t.Fatalf("authenticated discovery version = %q, want server version", health.Version)
	}
	if authorizationHeader != "" {
		t.Fatalf("authenticated discovery sent an authorization header: %q", authorizationHeader)
	}
	if receivedFrame["type"] != "req" || receivedFrame["method"] != "connect" {
		t.Fatalf("unexpected gateway frame: %#v", receivedFrame)
	}
	params, ok := receivedFrame["params"].(map[string]any)
	if !ok {
		t.Fatalf("connect params missing: %#v", receivedFrame)
	}
	scopes, ok := params["scopes"].([]any)
	if !ok {
		t.Fatalf("connect scopes missing: %#v", params)
	}
	hasRead := false
	hasWrite := false
	for _, scope := range scopes {
		value, _ := scope.(string)
		hasRead = hasRead || value == "operator.read"
		hasWrite = hasWrite || value == "operator.write"
	}
	if params["role"] != "operator" || len(scopes) != 1 || !hasRead || hasWrite {
		t.Fatalf("connect scopes were not bounded to operator.read: %#v", params)
	}
	auth, ok := params["auth"].(map[string]any)
	if !ok || auth["token"] != "gateway-read-token" {
		t.Fatalf("connect auth token was not sent through the protocol body: %#v", params)
	}
}

func TestOpenClawCompanionGatewayReadOnlyTaskLedgerDiscoveryReturnsOnlyAggregateCounts(t *testing.T) {
	var receivedFrames []map[string]any
	done := make(chan struct{})
	gateway := websocket.Server{
		Handler: func(connection *websocket.Conn) {
			defer close(done)
			if err := websocket.JSON.Send(connection, map[string]any{
				"type":    "event",
				"event":   "connect.challenge",
				"payload": map[string]any{"nonce": "challenge-nonce", "ts": float64(1_737_264_000_000)},
			}); err != nil {
				t.Errorf("send protocol challenge: %v", err)
				return
			}
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			for len(receivedFrames) < 2 {
				var frame map[string]any
				if err := websocket.JSON.Receive(connection, &frame); err != nil {
					return
				}
				receivedFrames = append(receivedFrames, frame)
				requestID, _ := frame["id"].(string)
				if frame["method"] == "connect" {
					if err := websocket.JSON.Send(connection, map[string]any{
						"type": "res",
						"id":   requestID,
						"ok":   true,
						"payload": map[string]any{
							"type":     "hello-ok",
							"protocol": float64(4),
							"server":   map[string]any{"version": "2026.8.1", "connId": "connection-1"},
							"features": map[string]any{"methods": []string{"tasks.list"}, "events": []string{}},
							"snapshot": map[string]any{},
							"auth":     map[string]any{"role": "operator", "scopes": []string{"operator.read"}},
							"policy":   map[string]any{"maxPayload": float64(65536), "maxBufferedBytes": float64(65536)},
						},
					}); err != nil {
						t.Errorf("send hello response: %v", err)
						return
					}
					continue
				}
				if err := websocket.JSON.Send(connection, map[string]any{
					"type": "res",
					"id":   requestID,
					"ok":   true,
					"payload": map[string]any{
						"tasks": []map[string]any{
							{"id": "sensitive-task-one", "title": "Never expose this", "status": "running"},
							{"id": "sensitive-task-two", "title": "Never expose this either", "status": "completed"},
							{"id": "sensitive-task-three", "title": "Still private", "status": "running"},
						},
						"nextCursor": "more-private-tasks",
					},
				}); err != nil {
					t.Errorf("send task ledger response: %v", err)
				}
			}
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"status":"live"}`))
	})
	mux.Handle("/", gateway)
	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("OPENCLAW_AGENT_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_AUTH_DISCOVERY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_TASK_LEDGER_DISCOVERY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_TOKEN", "gateway-read-token")
	t.Setenv("OPENCLAW_GATEWAY_URL", "ws"+strings.TrimPrefix(server.URL, "http"))
	t.Setenv("AGENT_RUNTIME_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("OPENCLAW_TIMEOUT_SECONDS", "1")

	health := newOpenClawAdapterFromEnv().HealthCheck(context.Background())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gateway did not finish the bounded task ledger discovery")
	}
	if health.Status != "available" || health.GatewayTaskLedger == nil {
		t.Fatalf("task-ledger discovery health = %#v", health)
	}
	if health.GatewayTaskLedger.SampledTasks != 3 || !health.GatewayTaskLedger.Truncated || health.GatewayTaskLedger.StatusCounts["running"] != 2 || health.GatewayTaskLedger.StatusCounts["completed"] != 1 {
		t.Fatalf("unexpected aggregate task ledger = %#v", health.GatewayTaskLedger)
	}
	if len(receivedFrames) != 2 || receivedFrames[0]["method"] != "connect" || receivedFrames[1]["method"] != "tasks.list" {
		t.Fatalf("unexpected gateway frames = %#v", receivedFrames)
	}
	params, ok := receivedFrames[1]["params"].(map[string]any)
	if !ok || params["limit"] != float64(openClawGatewayTaskLedgerLimit) || len(params) != 1 {
		t.Fatalf("task-ledger request should contain only its bounded limit: %#v", receivedFrames[1])
	}
}

func TestOpenClawGatewayTaskLedgerRejectsUnknownTaskStatus(t *testing.T) {
	_, err := openClawGatewayTaskLedgerFromResponse(json.RawMessage(`{"tasks":[{"status":"unexpected"}]}`))
	if err == nil {
		t.Fatal("unknown task status should be rejected")
	}
}

func TestPresentGatewayJSONRejectsMissingOrNullValues(t *testing.T) {
	tests := []struct {
		name  string
		value json.RawMessage
		want  bool
	}{
		{name: "missing", value: nil, want: false},
		{name: "empty", value: json.RawMessage("  "), want: false},
		{name: "null", value: json.RawMessage("null"), want: false},
		{name: "object", value: json.RawMessage(`{}`), want: true},
		{name: "array", value: json.RawMessage(`[]`), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := presentGatewayJSON(test.value); got != test.want {
				t.Fatalf("presentGatewayJSON(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestValidGatewayEvidenceValueRejectsUnboundedOrControlValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "version", value: "2026.7.1-2", want: true},
		{name: "control character", value: "2026.7\n1", want: false},
		{name: "oversized", value: strings.Repeat("a", 129), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validGatewayEvidenceValue(test.value); got != test.want {
				t.Fatalf("validGatewayEvidenceValue(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestOpenClawHighRiskSurfacesBlockExecutionUntilAcknowledged(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "openclaw")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	adapter := &openClawAdapter{
		enabled:          true,
		executable:       "openclaw",
		workspace:        workspace,
		workspaceRoot:    root,
		allowedHost:      map[string]bool{"127.0.0.1": true},
		agentCLIEnabled:  true,
		messagesEnabled:  true,
		browserEnabled:   true,
		hostToolsEnabled: true,
		sandboxRequired:  true,
		sandboxMode:      "all",
	}

	info := adapter.Info()
	if info.Configured || info.ExecutionEnabled {
		t.Fatalf("high-risk OpenClaw surfaces should block generic execution: %#v", info)
	}
	joinedMissing := strings.Join(info.MissingConfiguration, " ")
	for _, expected := range []string{"high-risk surfaces", "messaging/channel", "browser control", "host tools"} {
		if !strings.Contains(joinedMissing, expected) {
			t.Fatalf("missing configuration %q did not explain %q", joinedMissing, expected)
		}
	}
	if health := adapter.HealthCheck(context.Background()); health.Status != "blocked" || !strings.Contains(health.Reason, "high-risk surfaces") {
		t.Fatalf("health should be blocked by high-risk surfaces: %#v", health)
	}
	result := adapter.ExecuteTask(context.Background(), Task{Prompt: "do work", HumanApproved: true})
	if result.Status != "blocked" || !strings.Contains(result.Message, "high-risk surfaces") {
		t.Fatalf("execution should be blocked by high-risk surfaces: %#v", result)
	}
	registry := NewRegistry(adapter)
	registryResult := registry.Execute(context.Background(), "openclaw", approvedRuntimeTask("task-1", "do work"))
	if registryResult.Status != "blocked" || !strings.Contains(registryResult.Message, "browser control") || !strings.Contains(registryResult.Message, "host tools") {
		t.Fatalf("registry execution should preserve OpenClaw policy reason: %#v", registryResult)
	}

	adapter.highRiskExecution = true
	info = adapter.Info()
	if !info.Configured || !info.ExecutionEnabled {
		t.Fatalf("explicit acknowledgement should allow configured runtime state: %#v", info)
	}
	if !strings.Contains(strings.Join(info.Controls, " "), "OPENCLAW_ALLOW_HIGH_RISK_EXECUTION=true") {
		t.Fatalf("controls should disclose high-risk acknowledgement: %#v", info.Controls)
	}
}

func TestOpenClawSetEcosystemPathReleasesPreviousUploadArtifact(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "openclaw")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	firstUpload := filepath.Join(root, "openclaw-ecosystem-first.zip")
	secondUpload := filepath.Join(root, "openclaw-ecosystem-second.zip")
	manualFirst := filepath.Join(root, "openclaw-main.zip")
	manualSecond := filepath.Join(root, "openclaw-checkpoint.zip")
	if err := writeMinimalOpenClawZip(firstUpload); err != nil {
		t.Fatalf("write first upload file: %v", err)
	}
	if err := writeMinimalOpenClawZip(secondUpload); err != nil {
		t.Fatalf("write second upload file: %v", err)
	}
	if err := writeMinimalOpenClawZip(manualFirst); err != nil {
		t.Fatalf("write manual file: %v", err)
	}
	if err := writeMinimalOpenClawZip(manualSecond); err != nil {
		t.Fatalf("write manual file: %v", err)
	}

	adapter := &openClawAdapter{
		enabled:         true,
		executable:      "openclaw",
		workspace:       workspace,
		workspaceRoot:   root,
		ecosystemPath:   firstUpload,
		allowedHost:     map[string]bool{"127.0.0.1": true},
		agentCLIEnabled: true,
	}
	registry := NewRegistry(adapter)

	if _, err := registry.SetOpenClawEcosystemPath(secondUpload); err != nil {
		t.Fatalf("set second upload path: %v", err)
	}
	if _, err := os.Stat(firstUpload); err == nil {
		t.Fatalf("expected previous upload path to be removed: %s", firstUpload)
	}

	if _, err := registry.SetOpenClawEcosystemPath(manualFirst); err != nil {
		t.Fatalf("set manual path: %v", err)
	}
	if _, err := os.Stat(secondUpload); err == nil {
		t.Fatalf("expected upload path to be removed when leaving it: %s", secondUpload)
	}

	if _, err := registry.SetOpenClawEcosystemPath(manualSecond); err != nil {
		t.Fatalf("set second manual path: %v", err)
	}
	if _, err := os.Stat(manualFirst); err != nil {
		t.Fatalf("manual path should not be removed by cleanup: %v", err)
	}
}

func TestOpenClawAdapterInvokesControlledCli(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve native test executable: %v", err)
	}
	adapter := &openClawAdapter{
		enabled:         true,
		executable:      executable,
		workspace:       workspace,
		workspaceRoot:   root,
		stateDir:        stateDir,
		thinking:        "high",
		timeout:         30 * time.Second,
		outputLimit:     defaultOutputLimit,
		allowedHost:     map[string]bool{"127.0.0.1": true},
		agentCLIEnabled: true,
		sandboxRequired: true,
		sandboxMode:     "all",
	}

	result := adapter.ExecuteTask(context.Background(), Task{ID: "task-1", Prompt: "move safe work forward", ProjectKey: "project-1"})
	if result.Status != "completed" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{
		"agent", "--message", "move safe work forward", "--thinking", "high",
		"HAI approved OpenClaw task envelope", "Execution mode:", "Blocked surfaces:", "Validation checklist:",
		"OPENCLAW_STATE_DIR=" + stateDir, "OPENCLAW_HOME=" + stateDir, "HAI_RUNTIME_TASK_ID=task-1",
	} {
		if !strings.Contains(result.Output, expected) {
			t.Fatalf("output %q missing %q", result.Output, expected)
		}
	}
	for _, forbidden := range []string{"openclaw message send", "pairing approve --yes", "--yolo"} {
		if strings.Contains(result.Output, forbidden) {
			t.Fatalf("OpenClaw execution exposed forbidden operation %q in %q", forbidden, result.Output)
		}
	}
}

func TestOpenClawTaskEnvelopeRoutesThroughIndexedSkillsAndControls(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "openclaw-main.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{
		"openclaw-main/package.json",
		"openclaw-main/.agents/skills/autoreview/SKILL.md",
		"openclaw-main/.agents/skills/gitcrawl/SKILL.md",
		"openclaw-main/.agents/skills/channel-message-flows/SKILL.md",
		"openclaw-main/.agents/skills/technical-documentation/SKILL.md",
		"openclaw-main/extensions/whatsapp/package.json",
		"openclaw-main/extensions/ollama/package.json",
		"openclaw-main/extensions/browser/package.json",
		"openclaw-main/extensions/openshell/package.json",
		"openclaw-main/.github/workflows/ci.yml",
		"openclaw-main/.github/actions/setup-node-env/action.yml",
		"openclaw-main/.github/codeql/codeql-core-auth-secrets-critical-security.yml",
		"openclaw-main/.github/ISSUE_TEMPLATE/bug_report.yml",
		"openclaw-main/.github/instructions/copilot.instructions.md",
		"openclaw-main/qa/live/whatsapp-smoke.md",
		"openclaw-main/test/e2e/openclaw-smoke.test.ts",
		"openclaw-main/security/secret-scanning.yml",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		payload := []byte("{}")
		if name == "openclaw-main/package.json" {
			payload = []byte(`{"name":"openclaw","version":"2026.6.10"}`)
		}
		if _, err := entry.Write(payload); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	adapter := &openClawAdapter{
		enabled:          true,
		executable:       "openclaw",
		workspace:        root,
		workspaceRoot:    root,
		ecosystemPath:    zipPath,
		agentCLIEnabled:  true,
		sandboxRequired:  true,
		sandboxMode:      "all",
		messagesEnabled:  false,
		browserEnabled:   false,
		hostToolsEnabled: false,
	}

	envelope := adapter.openClawTaskEnvelope(Task{
		ID:         "task-1",
		ProjectKey: "share-t",
		Prompt:     "Review the GitHub pull request, security CI, issue triage instructions, and draft a WhatsApp follow-up, but do not send it.",
	})
	for _, expected := range []string{
		"HAI approved OpenClaw task envelope",
		"HAI task id: task-1",
		"HAI project key: share-t",
		"software engineering and repository workflow",
		"autoreview",
		"gitcrawl",
		"channel-message-flows",
		"ollama",
		"browser",
		"Relevant OpenClaw maps:",
		"github-action:setup-node-env",
		"security:codeql-core-auth-secrets-critical-security",
		"security-asset:secret-scanning",
		"qa:live/whatsapp-smoke",
		"test:e2e/openclaw-smoke.test",
		"issue-template:bug_report",
		"instruction:copilot.instructions",
		"whatsapp outbound send without separate HAI approval",
		"outbound message sending",
		"do not send messages",
		"Return format: concise completion summary",
	} {
		if !strings.Contains(envelope, expected) {
			t.Fatalf("task envelope missing %q:\n%s", expected, envelope)
		}
	}
	if strings.Contains(envelope, "public posting is allowed") {
		t.Fatalf("task envelope should not permit public effects:\n%s", envelope)
	}
	trace := openClawRouteTrace(adapter.openClawTaskProfile(Task{
		ID:         "task-1",
		ProjectKey: "share-t",
		Prompt:     "Review the GitHub pull request, security CI, issue triage instructions, and draft a WhatsApp follow-up, but do not send it.",
	}))
	if trace == nil || trace.RuntimeID != "openclaw" || trace.Intent != "software engineering and repository workflow" {
		t.Fatalf("route trace missing OpenClaw intent: %#v", trace)
	}
	for _, expected := range []string{"autoreview", "gitcrawl", "channel-message-flows"} {
		if !containsString(trace.RecommendedSkills, expected) {
			t.Fatalf("route trace missing skill %q from %#v", expected, trace.RecommendedSkills)
		}
	}
	if !containsString(trace.VisibleProviders, "ollama") || !containsString(trace.VisibleTools, "browser") {
		t.Fatalf("route trace did not expose provider/tool context: %#v", trace)
	}
	if !containsString(trace.BlockedSurfaces, "whatsapp outbound send without separate HAI approval") {
		t.Fatalf("route trace missing blocked channel send: %#v", trace.BlockedSurfaces)
	}
	for _, expected := range []string{"github-action:setup-node-env", "security:codeql-core-auth-secrets-critical-security", "issue-template:bug_report", "instruction:copilot.instructions"} {
		if !containsString(trace.RelevantMaps, expected) {
			t.Fatalf("route trace missing OpenClaw map %q from %#v", expected, trace.RelevantMaps)
		}
	}
}

func TestOpenClawTaskEnvelopeRoutesPersonalOperatingWork(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "openclaw-main.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{
		"openclaw-main/package.json",
		"openclaw-main/AGENTS.md",
		"openclaw-main/README.md",
		"openclaw-main/pnpm-workspace.yaml",
		"openclaw-main/.github/codex/prompts/maturity-scorecard-agent.md",
		"openclaw-main/.agents/maintainer-notes/telegram.md",
		"openclaw-main/.agents/skills/taskflow/SKILL.md",
		"openclaw-main/.agents/skills/agent-transcript/SKILL.md",
		"openclaw-main/.agents/skills/claw-score/SKILL.md",
		"openclaw-main/.agents/skills/claw-score/references/completeness/whatsapp.md",
		"openclaw-main/.agents/skills/technical-documentation/SKILL.md",
		"openclaw-main/extensions/document-extract/SKILL.md",
		"openclaw-main/extensions/ollama/package.json",
		"openclaw-main/extensions/whatsapp/package.json",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		payload := []byte("{}")
		if name == "openclaw-main/package.json" {
			payload = []byte(`{"name":"openclaw","version":"2026.6.10"}`)
		}
		if _, err := entry.Write(payload); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	adapter := &openClawAdapter{
		enabled:         true,
		executable:      "openclaw",
		workspace:       root,
		workspaceRoot:   root,
		ecosystemPath:   zipPath,
		agentCLIEnabled: true,
		sandboxRequired: true,
		sandboxMode:     "all",
	}
	task := Task{
		ID:         "pursuit-1",
		ProjectKey: "robert-os",
		Prompt:     "Create a pursuit next action from WhatsApp evidence, timeline, deadline follow-up, Odoo HERP operation, and Ollama local model routing. Do not send, publish, or delete anything.",
	}
	envelope := adapter.openClawTaskEnvelope(task)
	for _, expected := range []string{
		"HAI pursuit and open-loop operations",
		"taskflow",
		"agent-transcript",
		"technical-documentation",
		"claw-score",
		"document-extract/default",
		"completeness:claw-score/completeness/whatsapp",
		"maintainer-note:telegram",
		"root-doc:AGENTS",
		"root-doc:README",
		"root-config:pnpm-workspace.yaml",
		"codex-prompt:maturity-scorecard-agent",
		"outbound communication without separate HAI approval",
		"public posting without source-grounded review and separate HAI approval",
		"destructive or irreversible file action without rollback plan and explicit approval",
		"pursuit/open-loop state and next safe action are explicit when applicable",
		"source, evidence, or missing-evidence status is reported when factual claims are made",
	} {
		if !strings.Contains(envelope, expected) {
			t.Fatalf("personal operating envelope missing %q:\n%s", expected, envelope)
		}
	}
	trace := openClawRouteTrace(adapter.openClawTaskProfile(task))
	if trace == nil || trace.Intent != "HAI pursuit and open-loop operations" {
		t.Fatalf("route trace missing personal-ops intent: %#v", trace)
	}
	for _, expected := range []string{"taskflow", "agent-transcript", "technical-documentation", "claw-score", "document-extract/default"} {
		if !containsString(trace.RecommendedSkills, expected) {
			t.Fatalf("route trace missing personal-ops skill %q from %#v", expected, trace.RecommendedSkills)
		}
	}
	if !containsString(trace.VisibleProviders, "ollama") {
		t.Fatalf("route trace should expose local model provider context: %#v", trace.VisibleProviders)
	}
}

func TestOpenClawEcosystemInventoryFromZip(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	zipPath := filepath.Join(root, "openclaw-main.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{
		"openclaw-main/package.json",
		"openclaw-main/AGENTS.md",
		"openclaw-main/CLAUDE.md",
		"openclaw-main/README.md",
		"openclaw-main/.crabbox.yaml",
		"openclaw-main/Dockerfile",
		"openclaw-main/pnpm-workspace.yaml",
		"openclaw-main/.github/codex/prompts/docs-agent.md",
		"openclaw-main/.github/codex/prompts/maturity-scorecard-agent.md",
		"openclaw-main/.github/workflows/ci.yml",
		"openclaw-main/.github/workflows/openclaw-release-publish.yml",
		"openclaw-main/.github/actions/docker-e2e-plan/action.yml",
		"openclaw-main/.github/actions/setup-node-env/action.yml",
		"openclaw-main/.github/ISSUE_TEMPLATE/bug_report.yml",
		"openclaw-main/.github/ISSUE_TEMPLATE/feature_request.yml",
		"openclaw-main/.github/codeql/codeql-core-auth-secrets-critical-security.yml",
		"openclaw-main/.github/codeql/openclaw-boundary/queries/managed-proxy-runtime-mutation.ql",
		"openclaw-main/.github/package-trusted-sources.json",
		"openclaw-main/.github/zizmor.yml",
		"openclaw-main/.github/instructions/copilot.instructions.md",
		"openclaw-main/docs/gateway/architecture.md",
		"openclaw-main/scripts/install-openclaw.mjs",
		"openclaw-main/qa/live/whatsapp-smoke.md",
		"openclaw-main/test/e2e/openclaw-smoke.test.ts",
		"openclaw-main/config/gateway.policy.json",
		"openclaw-main/security/secret-scanning.yml",
		"openclaw-main/deploy/docker-compose.preview.yml",
		"openclaw-main/fly.toml",
		"openclaw-main/.agents/maintainer-notes/telegram.md",
		"openclaw-main/.agents/skills/autoreview/SKILL.md",
		"openclaw-main/.agents/skills/autoreview/scripts/autoreview",
		"openclaw-main/.agents/skills/autoreview/scripts/test-review-harness.ps1",
		"openclaw-main/.agents/skills/gitcrawl/SKILL.md",
		"openclaw-main/.agents/skills/gitcrawl/agents/openai.yaml",
		"openclaw-main/.agents/skills/gitcrawl/references/source-map.md",
		"openclaw-main/.agents/skills/claw-score/SKILL.md",
		"openclaw-main/.agents/skills/claw-score/references/completeness/whatsapp.md",
		"openclaw-main/.agents/skills/technical-documentation/SKILL.md",
		"openclaw-main/.agents/skills/technical-documentation/agents/docs-framework-agent.md",
		"openclaw-main/.agents/skills/technical-documentation/scripts/docs-summary.mjs",
		"openclaw-main/extensions/acpx/skills/acp-router/SKILL.md",
		"openclaw-main/extensions/acpx/skills/acp-router/agents/openai.yaml",
		"openclaw-main/extensions/acpx/skills/acp-router/references/router.md",
		"openclaw-main/extensions/azure-speech/package.json",
		"openclaw-main/extensions/browser/skills/browser-automation/SKILL.md",
		"openclaw-main/extensions/github-copilot/package.json",
		"openclaw-main/extensions/lobster/SKILL.md",
		"openclaw-main/extensions/lobster/package.json",
		"openclaw-main/extensions/openai/package.json",
		"openclaw-main/extensions/ollama/package.json",
		"openclaw-main/extensions/whatsapp/package.json",
		"openclaw-main/extensions/browser/package.json",
		"openclaw-main/apps/android/README.md",
		"openclaw-main/packages/agent-core/package.json",
		"openclaw-main/skills/taskflow/SKILL.md",
		"openclaw-main/src/agents/index.ts",
		"openclaw-main/src/gateway/index.ts",
		"openclaw-main/ui/src/ui/views/overview.ts",
		"openclaw-main/ui/src/ui/views/overview.test.ts",
		"openclaw-main/ui/src/ui/controllers/exec-approval.ts",
		"openclaw-main/ui/src/ui/controllers/exec-approval.test.ts",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		payload := []byte("{}")
		if name == "openclaw-main/package.json" {
			payload = []byte(`{"name":"openclaw","version":"2026.6.10","license":"MIT","packageManager":"pnpm@11.2.2+sha512.test","engines":{"node":">=22.19.0"}}`)
		}
		if _, err := entry.Write(payload); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	adapter := &openClawAdapter{
		enabled:         true,
		executable:      "openclaw",
		workspace:       workspace,
		workspaceRoot:   root,
		ecosystemPath:   zipPath,
		agentCLIEnabled: true,
		sandboxRequired: true,
		sandboxMode:     "all",
		companionApps:   []string{"windows"},
	}
	info := adapter.Info()
	joined := strings.Join(ecosystemSurfaceStrings(info.Ecosystem), " ")
	for _, expected := range []string{
		"Skills:8",
		"Skill scripts:3",
		"Package metadata:5",
		"Configured HAI surfaces:3",
		"HAI-blocked high-risk surfaces:10",
		"Operator setup checklist:5",
		"Agent profiles:3",
		"Skill reference maps:3",
		"Completeness maps:1",
		"Maintainer notes:1",
		"Documentation corpus:1",
		"Root scripts:1",
		"QA assets:1",
		"Test suites:1",
		"Configuration profiles:1",
		"Security assets:1",
		"Deployment targets:2",
		"Codex prompt maps:2",
		"GitHub workflows:2",
		"GitHub Actions:2",
		"GitHub issue templates:2",
		"Security and CodeQL maps:4",
		"Repository instructions:1",
		"Repository docs:3",
		"Repository config:4",
		"Provider extensions:3",
		"Channel extensions:1",
		"Tool/runtime extensions:4",
		"Companion apps:1",
		"Core packages:1",
		"Source modules:2",
		"Control UI views:1",
		"Control UI controllers:1",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("ecosystem inventory missing %q from %#v", expected, info.Ecosystem)
		}
	}
	metadata := runtimeSurfaceItems(info.Ecosystem, "Package metadata")
	for _, expected := range []string{"package=openclaw", "version=2026.6.10", "license=MIT", "node=>=22.19.0", "package-manager=pnpm@11.2.2"} {
		if !containsString(metadata, expected) {
			t.Fatalf("OpenClaw metadata missing %q from %#v", expected, metadata)
		}
	}
	inventory := runtimeSurface(info.Ecosystem, "Package inventory")
	if inventory.Count != summedInventoryItems(inventory.Items) {
		t.Fatalf("package inventory count %d does not match displayed inventory items %#v", inventory.Count, inventory.Items)
	}
	if !containsString(inventory.Items, "4 tool/runtime extensions") {
		t.Fatalf("package inventory should include tool/runtime extension count: %#v", inventory.Items)
	}
	if views := runtimeSurfaceItems(info.Ecosystem, "Control UI views"); !containsString(views, "overview") || containsString(views, "overview.test") {
		t.Fatalf("OpenClaw UI view inventory should include production views only: %#v", views)
	}
	if controllers := runtimeSurfaceItems(info.Ecosystem, "Control UI controllers"); !containsString(controllers, "exec-approval") || containsString(controllers, "exec-approval.test") {
		t.Fatalf("OpenClaw controller inventory should include production controllers only: %#v", controllers)
	}
	if surface := runtimeSurface(info.Ecosystem, "Channel extensions"); surface.RiskLevel != "high" || !surface.ApprovalRequired {
		t.Fatalf("channel extensions should be high-risk and approval-gated: %#v", surface)
	}
	if surface := runtimeSurface(info.Ecosystem, "Provider extensions"); surface.RiskLevel != "medium" || !surface.ApprovalRequired {
		t.Fatalf("provider extensions should be medium-risk and approval-gated: %#v", surface)
	}
	if surface := runtimeSurface(info.Ecosystem, "Skill scripts"); surface.RiskLevel != "high" || !surface.ApprovalRequired {
		t.Fatalf("skill scripts should be high-risk and approval-gated: %#v", surface)
	}
	scripts := runtimeSurfaceItems(info.Ecosystem, "Skill scripts")
	for _, expected := range []string{"autoreview/autoreview", "autoreview/test-review-harness.ps1", "technical-documentation/docs-summary.mjs"} {
		if !containsString(scripts, expected) {
			t.Fatalf("OpenClaw script inventory missing %q from %#v", expected, scripts)
		}
	}
	skills := adapter.ListSkills(context.Background())
	foundScriptSkill := false
	for _, skill := range skills {
		if skill.Name == "autoreview/autoreview" && skill.Category == "skill_script" {
			foundScriptSkill = true
			if skill.RiskLevel != "high" || !skill.ApprovalRequired || skill.ExecutionMode != "catalog_only_not_directly_invoked" {
				t.Fatalf("OpenClaw script skill should be high-risk catalog-only: %#v", skill)
			}
		}
	}
	if !foundScriptSkill {
		t.Fatalf("OpenClaw ListSkills did not surface executable skill scripts: %#v", skills)
	}
}

func ecosystemSurfaceStrings(surfaces []RuntimeEcosystemSurface) []string {
	result := []string{}
	for _, surface := range surfaces {
		result = append(result, surface.Category+":"+strconv.Itoa(surface.Count))
	}
	return result
}

func runtimeSurfaceItems(surfaces []RuntimeEcosystemSurface, category string) []string {
	return runtimeSurface(surfaces, category).Items
}

func runtimeSurface(surfaces []RuntimeEcosystemSurface, category string) RuntimeEcosystemSurface {
	for _, surface := range surfaces {
		if surface.Category == category {
			return surface
		}
	}
	return RuntimeEcosystemSurface{}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func summedInventoryItems(values []string) int {
	total := 0
	for _, value := range values {
		var count int
		if _, err := fmt.Sscanf(value, "%d", &count); err == nil {
			total += count
		}
	}
	return total
}

func TestOpenClawSetEcosystemPathRejectsCallerSelectedPathOutsideAllowedRoots(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "allowed")
	outside := filepath.Join(parent, "outside")
	for _, directory := range []string{allowed, outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	initial := filepath.Join(allowed, "openclaw-main.zip")
	callerSelected := filepath.Join(outside, "openclaw-main.zip")
	for _, archive := range []string{initial, callerSelected} {
		if err := writeMinimalOpenClawZip(archive); err != nil {
			t.Fatalf("write OpenClaw archive %s: %v", archive, err)
		}
	}
	adapter := &openClawAdapter{
		workspace:     allowed,
		workspaceRoot: allowed,
		ecosystemPath: initial,
	}

	if err := adapter.setEcosystemPath(callerSelected); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside ecosystem path error = %v, want allowlist rejection", err)
	}
}

func TestOpenClawSetEcosystemPathRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "allowed")
	outside := filepath.Join(parent, "outside")
	for _, directory := range []string{allowed, outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	initial := filepath.Join(allowed, "openclaw-main.zip")
	outsideArchive := filepath.Join(outside, "openclaw-main.zip")
	for _, archive := range []string{initial, outsideArchive} {
		if err := writeMinimalOpenClawZip(archive); err != nil {
			t.Fatalf("write OpenClaw archive %s: %v", archive, err)
		}
	}
	link := filepath.Join(allowed, "linked-openclaw.zip")
	if err := os.Symlink(outsideArchive, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	adapter := &openClawAdapter{
		workspace:     allowed,
		workspaceRoot: allowed,
		ecosystemPath: initial,
	}

	if err := adapter.setEcosystemPath(link); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("symlink ecosystem path error = %v, want resolved allowlist rejection", err)
	}
}

func TestOdysseusAdapterEmergencyStopPreventsNetworkIO(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter := &odysseusAdapter{
		enabled:     true,
		baseURL:     server.URL,
		timeout:     time.Second,
		outputLimit: defaultOutputLimit,
		allowedHost: map[string]bool{"127.0.0.1": true},
	}

	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("task-1", "do not send"))
	if result.Status != "blocked" || calls != 0 {
		t.Fatalf("emergency-stop result=%#v calls=%d, want block before network I/O", result, calls)
	}
}

func approvedRuntimeTask(id, prompt string) Task {
	task := Task{
		ID:               id,
		Prompt:           prompt,
		OwnerIdentity:    "alice",
		HumanApproved:    true,
		ApprovalSourceID: "task-review:11111111-1111-4111-8111-111111111111",
	}
	return withValidFinalEffectProof("test", task, Info{RequiresApproval: true})
}

func withValidFinalEffectProof(runtimeID string, task Task, info Info) Task {
	request := runtimeFinalEffectRequest(runtimeID, task, info)
	task.FinalEffectProof = FinalEffectAuthorizationProof{
		ReceiptID:                  "11111111-1111-4111-8111-111111111111",
		AuthorizationRequestDigest: strings.Repeat("c", 64),
		DecisionDigest:             strings.Repeat("a", 64),
		RuntimeRequestDigest:       finalEffectRequestDigest(request),
	}
	return task
}

func verifyTestFinalEffectProof(
	request FinalEffectAuthorizationRequest,
	proof FinalEffectAuthorizationProof,
) error {
	if proof.ReceiptID != "11111111-1111-4111-8111-111111111111" {
		return errors.New("receipt not found")
	}
	if proof.AuthorizationRequestDigest != strings.Repeat("c", 64) {
		return errors.New("authorization request digest mismatch")
	}
	if proof.DecisionDigest != strings.Repeat("a", 64) {
		return errors.New("decision digest mismatch")
	}
	if proof.RuntimeRequestDigest != finalEffectRequestDigest(request) {
		return errors.New("runtime request digest mismatch")
	}
	return nil
}

func newVerifiedTestRegistry(adapters ...Adapter) *Registry {
	return NewRegistryWithFinalEffectVerifier(
		FinalEffectProofVerifierFunc(func(_ context.Context, request FinalEffectAuthorizationRequest, proof FinalEffectAuthorizationProof) error {
			return verifyTestFinalEffectProof(request, proof)
		}),
		adapters...,
	)
}

func executableFakeAdapter() *fakeAdapter {
	return &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
}

func TestDeepSeekHarnessAdapterIsRegisteredAndDisabledByDefault(t *testing.T) {
	t.Setenv("DEEPSEEK_HARNESS_ENABLED", "false")
	t.Setenv("DEEPSEEK_HARNESS_WORKSPACE", "")
	t.Setenv("DEEPSEEK_HARNESS_STATE_DIR", "")
	t.Setenv("AGENT_RUNTIME_WORKSPACE_ROOT", "")
	registry := NewRegistry(newDeepSeekHarnessAdapterFromEnv())
	infos := registry.List()
	if len(infos) != 1 {
		t.Fatalf("runtime count = %d, want 1", len(infos))
	}
	info := infos[0]
	if info.ID != "deepseek-harness" || info.Enabled || info.ExecutionEnabled || info.Type != "deepseek_harness" {
		t.Fatalf("unexpected DeepSeek Harness info: %#v", info)
	}
	if !containsString(info.MissingConfiguration, "DEEPSEEK_HARNESS_WORKSPACE") {
		t.Fatalf("missing configuration = %#v, want workspace", info.MissingConfiguration)
	}
	if !containsString(info.MissingConfiguration, "DEEPSEEK_HARNESS_VERSION") {
		t.Fatalf("missing configuration = %#v, want pinned version", info.MissingConfiguration)
	}
	if skills, err := registry.Skills(context.Background(), "deepseek-harness"); err != nil || len(skills) != 1 || skills[0].ExecutionMode != "approved_headless_task" {
		t.Fatalf("skills = %#v, err = %v", skills, err)
	}
}

func TestDeepSeekHarnessAdapterRequiresExplicitHeadlessExecutionOptIn(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:         true,
		executable:      "dsh",
		expectedVersion: "test-preview-1.0",
		workspace:       workspace,
		workspaceRoot:   root,
		stateDir:        filepath.Join(workspace, ".dsh-state"),
	}
	info := adapter.Info()
	if !info.Configured || info.ExecutionEnabled {
		t.Fatalf("adapter configuration = %#v, want configured but execution opt-in disabled", info)
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "EXECUTION_ENABLED") {
		t.Fatalf("result = %#v, want execution opt-in block", result)
	}
}

func TestDeepSeekHarnessAdapterQueuesApprovedTaskToHostBridge(t *testing.T) {
	dispatcher := &capturingHostRuntimeDispatcher{}
	adapter := &deepSeekHarnessAdapter{
		enabled:             true,
		executionEnabled:    true,
		expectedVersion:     "0.1.1-rc.2",
		workspaceKey:        "hai",
		dispatcher:          dispatcher,
		hostDispatchEnabled: true,
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "queued" || !strings.Contains(result.Message, "Windows host bridge") {
		t.Fatalf("result = %#v, want queued host bridge task", result)
	}
	if dispatcher.task.RuntimeID != "deepseek-harness" || dispatcher.task.TaskID != "harness-task" || !dispatcher.task.Approved {
		t.Fatalf("host runtime task = %#v", dispatcher.task)
	}
}

func TestDeepSeekHarnessAdapterDoesNotQueueWhenHostBridgeIsDisabled(t *testing.T) {
	dispatcher := &capturingHostRuntimeDispatcher{}
	adapter := &deepSeekHarnessAdapter{
		enabled:          true,
		executionEnabled: true,
		expectedVersion:  "0.1.1-rc.2",
		workspaceKey:     "hai",
		dispatcher:       dispatcher,
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "host bridge is disabled") {
		t.Fatalf("result = %#v, want disabled host bridge block", result)
	}
	if dispatcher.task.TaskID != "" {
		t.Fatalf("disabled host bridge unexpectedly received task: %#v", dispatcher.task)
	}
}

func TestDeepSeekHarnessAdapterRunsDocumentedHeadlessProfile(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		versionProbe:                func(context.Context) (string, error) { return "dsh test-preview-1.0", nil },
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		timeout:                     5 * time.Second,
		outputLimit:                 defaultOutputLimit,
		allowDirectExecutionForTest: true,
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "completed" || !strings.Contains(result.Output, "--profile") || !strings.Contains(result.Output, "headless") {
		t.Fatalf("result = %#v, want documented headless invocation", result)
	}
	if !strings.Contains(result.Output, "DSH_HOME=") || !strings.Contains(result.Output, "HAI_RUNTIME_TASK_ID=harness-task") {
		t.Fatalf("result output = %q, want isolated state and task metadata", result.Output)
	}
}

func TestDeepSeekHarnessAdapterRejectsOptionLikePrompt(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		versionProbe:                func(context.Context) (string, error) { return "dsh test-preview-1.0", nil },
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		timeout:                     time.Second,
		outputLimit:                 defaultOutputLimit,
		allowDirectExecutionForTest: true,
	}

	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "--install-plugin=untrusted"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "must not start with a command option") {
		t.Fatalf("result = %#v, want option-like prompt rejection", result)
	}
}

func TestDeepSeekHarnessAdapterRejectsLauncherSubcommandPrompt(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		timeout:                     time.Second,
		outputLimit:                 defaultOutputLimit,
		allowDirectExecutionForTest: true,
	}

	for _, prompt := range []string{"web", "plugin"} {
		result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", prompt))
		if result.Status != "blocked" || !strings.Contains(result.Message, "launcher subcommand") {
			t.Fatalf("prompt %q result = %#v, want launcher subcommand rejection", prompt, result)
		}
	}
}

func TestDeepSeekHarnessAdapterBoundsWaitingForSharedState(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		timeout:                     10 * time.Millisecond,
		outputLimit:                 defaultOutputLimit,
		allowDirectExecutionForTest: true,
	}
	if !adapter.acquireExecutionGate(context.Background()) {
		t.Fatal("could not acquire test execution gate")
	}
	defer adapter.releaseExecutionGate()

	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "already running") {
		t.Fatalf("result = %#v, want bounded shared-state wait", result)
	}
}

func TestDeepSeekHarnessAdapterRejectsWorkspaceOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	adapter := &deepSeekHarnessAdapter{
		enabled:       true,
		executable:    "dsh",
		workspace:     outside,
		workspaceRoot: root,
		stateDir:      filepath.Join(outside, ".dsh-state"),
	}
	if reason := adapter.workspaceBlockedReason(); !strings.Contains(reason, "must stay inside") {
		t.Fatalf("workspace block reason = %q", reason)
	}
}

func TestDeepSeekHarnessAdapterRejectsMissingWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		workspace:                   workspace,
		workspaceRoot:               "",
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		allowDirectExecutionForTest: true,
	}

	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "workspace root") {
		t.Fatalf("result = %#v, want missing workspace root block", result)
	}
}

func TestDeepSeekHarnessAdapterRejectsStateDirectoryOutsideRoot(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "test-preview-1.0",
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(t.TempDir(), ".dsh-state"),
		allowDirectExecutionForTest: true,
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "state directory must stay inside") {
		t.Fatalf("result = %#v, want state directory block", result)
	}
}

func TestDeepSeekHarnessAdapterRejectsStateDirectorySymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	stateDir := filepath.Join(workspace, ".dsh-state")
	if err := os.Symlink(outside, stateDir); err != nil {
		t.Skipf("symlink creation is unavailable in this test environment: %v", err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:          true,
		executionEnabled: true,
		executable:       os.Args[0],
		expectedVersion:  "test-preview-1.0",
		workspace:        workspace,
		workspaceRoot:    root,
		stateDir:         stateDir,
	}
	if reason := adapter.stateDirBlockedReason(); !strings.Contains(reason, "must not be a symbolic link") {
		t.Fatalf("state directory block reason = %q", reason)
	}
}

func TestDeepSeekHarnessHealthReportsPreviewReadiness(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:          true,
		executionEnabled: true,
		executable:       os.Args[0],
		expectedVersion:  "test-preview-1.0",
		versionProbe:     func(context.Context) (string, error) { return "dsh test-preview-1.0", nil },
		workspace:        workspace,
		workspaceRoot:    root,
		stateDir:         filepath.Join(workspace, ".dsh-state"),
	}
	health := adapter.HealthCheck(context.Background())
	if health.Status != "ready" || !strings.Contains(health.Reason, "headless") {
		t.Fatalf("health = %#v, want headless readiness", health)
	}
}

func TestDeepSeekHarnessAdapterBlocksVersionMismatchBeforeTaskExecution(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "deepseek-harness")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &deepSeekHarnessAdapter{
		enabled:                     true,
		executionEnabled:            true,
		executable:                  os.Args[0],
		expectedVersion:             "different-preview",
		versionProbe:                func(context.Context) (string, error) { return "dsh test-preview-1.0", nil },
		workspace:                   workspace,
		workspaceRoot:               root,
		stateDir:                    filepath.Join(workspace, ".dsh-state"),
		timeout:                     time.Second,
		outputLimit:                 defaultOutputLimit,
		allowDirectExecutionForTest: true,
	}
	result := adapter.ExecuteTask(context.Background(), approvedRuntimeTask("harness-task", "inspect workspace"))
	if result.Status != "blocked" || !strings.Contains(result.Message, "version mismatch") {
		t.Fatalf("result = %#v, want version mismatch block", result)
	}
}

type capturingHostRuntimeDispatcher struct {
	task hostruntime.ApprovedTask
	err  error
}

func (d *capturingHostRuntimeDispatcher) Enqueue(task hostruntime.ApprovedTask) (*hostruntime.Job, error) {
	d.task = task
	if d.err != nil {
		return nil, d.err
	}
	return &hostruntime.Job{ID: uuid.New(), Status: hostruntime.StatusPending}, nil
}

type fakeAdapter struct {
	info        Info
	called      bool
	healthCalls int
}

func (a *fakeAdapter) Info() Info {
	return a.info
}

func (a *fakeAdapter) HealthCheck(context.Context) Health {
	a.healthCalls++
	return Health{RuntimeID: a.info.ID, Status: "ready"}
}

func (a *fakeAdapter) ListSkills(context.Context) []Skill {
	return []Skill{{
		ID:               a.info.ID + ":skill:test",
		RuntimeID:        a.info.ID,
		Name:             "test",
		Category:         "skill",
		RiskLevel:        "low",
		ApprovalRequired: false,
		ExecutionMode:    "test",
	}}
}

func (a *fakeAdapter) ExecuteTask(context.Context, Task) Result {
	a.called = true
	return Result{RuntimeID: a.info.ID, Status: "completed"}
}

func (a *fakeAdapter) StopTask(_ context.Context, taskID string) StopResult {
	return StopResult{RuntimeID: a.info.ID, TaskID: taskID, Status: "stopped"}
}

type blockingAdapter struct {
	info    Info
	started chan struct{}
}

func (a *blockingAdapter) Info() Info {
	return a.info
}

func (a *blockingAdapter) HealthCheck(context.Context) Health {
	return Health{RuntimeID: a.info.ID, Status: "ready"}
}

func (a *blockingAdapter) ListSkills(context.Context) []Skill {
	return nil
}

func (a *blockingAdapter) ExecuteTask(ctx context.Context, _ Task) Result {
	if a.started == nil {
		a.started = make(chan struct{})
	}
	close(a.started)
	<-ctx.Done()
	return Result{
		RuntimeID:   a.info.ID,
		Status:      "failed",
		Message:     ctx.Err().Error(),
		ExitCode:    -1,
		AuditEvents: []string{"blocking adapter saw context cancellation"},
	}
}

func (a *blockingAdapter) StopTask(_ context.Context, taskID string) StopResult {
	return StopResult{RuntimeID: a.info.ID, TaskID: taskID, Status: "unsupported"}
}
