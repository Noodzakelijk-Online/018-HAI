package runtimelab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/frameworkregistry"
	"automation-hub-backend/internal/operations"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	broker := newAuthorizedRuntimeLabTestBroker(
		t,
		t.TempDir(),
		"local-operator",
		"local",
	)
	ops := operations.NewService(operations.NewMemoryRepository())
	return NewService(broker, ops, "local-operator", "local")
}

func newAuthorizedRuntimeLabTestBroker(
	t *testing.T,
	workspace string,
	owner string,
	workspaceID string,
) *executionbroker.Broker {
	t.Helper()
	frameworks, err := frameworkregistry.NewService(
		frameworkregistry.NewMemoryRepository(),
	)
	if err != nil {
		t.Fatalf("new framework registry: %v", err)
	}
	draft, err := frameworks.CreateConstitutionDraft(
		owner,
		frameworkregistry.ConstitutionDraftRequest{
			BaseVersion:   1,
			ChangeSummary: "Activate production-like local execution test policy.",
		},
	)
	if err != nil {
		t.Fatalf("create Constitution draft: %v", err)
	}
	active, err := frameworks.ActivateConstitution(
		owner,
		draft.ID,
		owner,
		frameworkregistry.ActivateConstitutionRequest{
			Confirmation: "ACTIVATE CONSTITUTION",
			ApprovalNote: "Owner reviewed and approved this test policy.",
		},
	)
	if err != nil {
		t.Fatalf("activate Constitution: %v", err)
	}
	if active.Status != frameworkregistry.ConstitutionActive {
		t.Fatalf("Constitution status = %q, want active", active.Status)
	}
	constitution, err := executionauth.NewConstitutionPolicyAdapter(frameworks)
	if err != nil {
		t.Fatalf("adapt Constitution policy: %v", err)
	}
	authorization, err := executionauth.NewService(
		executionauth.NewMemoryRepository(),
		constitution,
		nil,
		nil,
		nil,
		func() time.Time {
			return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
		},
	)
	if err != nil {
		t.Fatalf("new execution authorization service: %v", err)
	}
	authorization.WithEmergencyStopEvaluator(func() executionauth.EmergencyStopEvidence {
		return executionauth.EmergencyStopEvidence{
			Source: "runtimelab-test",
		}
	})
	broker, err := executionbroker.NewAuthorizedBroker(
		workspace,
		owner,
		workspaceID,
		authorization,
	)
	if err != nil {
		t.Fatalf("new authorized broker: %v", err)
	}
	return broker
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

func TestFeatureParityAccountsForEveryRequiredAreaPerRuntime(t *testing.T) {
	s := newTestService(t)
	s.now = func() time.Time {
		return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	}
	overview, err := s.FeatureParity()
	if err != nil {
		t.Fatalf("feature parity: %v", err)
	}
	if len(overview.Inventories) != 3 {
		t.Fatalf("inventories = %d, want 3", len(overview.Inventories))
	}
	if len(overview.RequiredCoverageAreas) != len(requiredRuntimeCoverageAreas) {
		t.Fatalf("required coverage = %d, want %d", len(overview.RequiredCoverageAreas), len(requiredRuntimeCoverageAreas))
	}
	if overview.GeneratedAt != s.now().UTC() {
		t.Fatalf("generatedAt = %s, want %s", overview.GeneratedAt, s.now().UTC())
	}
	for _, inventory := range overview.Inventories {
		if inventory.ReadinessCeiling != "declared" {
			t.Fatalf("%s readiness ceiling = %q, want declared", inventory.RuntimeID, inventory.ReadinessCeiling)
		}
		if len(inventory.Features) < 13 {
			t.Fatalf("%s feature groups = %d, want at least 13", inventory.RuntimeID, len(inventory.Features))
		}
		if err := validateRuntimeInventory(inventory); err != nil {
			t.Fatalf("validate %s: %v", inventory.RuntimeID, err)
		}
		for _, item := range inventory.Features {
			if item.Disposition == DispositionDeferred || item.Disposition == DispositionBlockedExternal {
				if item.BacklogPriority == "" || item.RecommendedPath == "" || len(item.Requirements) == 0 {
					t.Fatalf("%s/%s lacks actionable backlog: %#v", inventory.RuntimeID, item.ID, item)
				}
			}
		}
	}
	if overview.DispositionCounts[string(DispositionIntegratedDirectly)] != 0 {
		t.Fatal("source review must not claim a direct integration")
	}
}

func TestOdysseusParityFailsClosedOnLicenseAndUnsafeAdminTools(t *testing.T) {
	s := newTestService(t)
	inventory, ok, err := s.RuntimeFeatureParity(" ODYSSEUS ")
	if err != nil || !ok {
		t.Fatalf("odysseus inventory = (%t, %v)", ok, err)
	}
	if inventory.License != "AGPL-3.0-or-later" {
		t.Fatalf("license = %q", inventory.License)
	}
	byID := map[string]RuntimeFeature{}
	for _, item := range inventory.Features {
		byID[item.ID] = item
	}
	if byID["odysseus-capabilities"].Disposition != DispositionExcludedIncompatibleLicense {
		t.Fatalf("capability import disposition = %q", byID["odysseus-capabilities"].Disposition)
	}
	if byID["odysseus-host-tools"].Disposition != DispositionConstrainedUnsafe {
		t.Fatalf("host tool disposition = %q", byID["odysseus-host-tools"].Disposition)
	}
	if byID["odysseus-security"].ExclusionReason == "" {
		t.Fatal("unsafe admin boundary requires an explicit reason")
	}
}

func TestRuntimeFeatureParityUnknownRuntime(t *testing.T) {
	_, ok, err := newTestService(t).RuntimeFeatureParity("missing")
	if err != nil || ok {
		t.Fatalf("unknown inventory = (%t, %v), want false nil", ok, err)
	}
}

func TestCapabilityCardsAreCompleteAndNeverGrantAuthority(t *testing.T) {
	for _, key := range []string{"OPENCLAW_BASE_URL", "HERMES_BASE_URL", "ODYSSEUS_BASE_URL"} {
		t.Setenv(key, "")
	}
	overview, err := newTestService(t).CapabilityCards(context.Background())
	if err != nil {
		t.Fatalf("capability cards: %v", err)
	}
	if overview.Authority != "contract_only" || len(overview.Cards) != 7 {
		t.Fatalf("capability overview = %#v", overview)
	}
	seen := map[string]bool{}
	for _, card := range overview.Cards {
		if seen[card.ID] {
			t.Fatalf("duplicate card %q", card.ID)
		}
		seen[card.ID] = true
		if card.ReadinessLevel != ReadinessDeclared || card.CanInvoke || card.CanExecuteExternalEffect {
			t.Fatalf("card widened runtime authority: %#v", card)
		}
		if card.ExpectedCostEURMax != 0 || card.TimeoutSeconds < 1 || len(card.RequiredAuthority) == 0 ||
			len(card.ApprovalRequirements) == 0 || len(card.EvidenceReturned) == 0 ||
			card.InputSchema["type"] != "object" || card.OutputSchema["type"] != "object" {
			t.Fatalf("incomplete capability card: %#v", card)
		}
	}
}

func TestConfiguredCapabilityRemainsNonInvocable(t *testing.T) {
	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("HERMES_BASE_URL", "http://127.0.0.1:8999")
	overview, err := newTestService(t).CapabilityCards(context.Background())
	if err != nil {
		t.Fatalf("capability cards: %v", err)
	}
	for _, card := range overview.Cards {
		if card.RuntimeID != "hermes" {
			continue
		}
		if card.ReadinessLevel != ReadinessConfigured || card.AuthenticationState != "operator_configured_unverified" {
			t.Fatalf("configured Hermes card = %#v", card)
		}
		if card.CanInvoke || card.CanExecuteExternalEffect {
			t.Fatalf("configured Hermes card granted authority: %#v", card)
		}
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

func TestUnauthorizedSafeWorkerIsBlockedAndSelfTestFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	broker := executionbroker.NewBroker(workspace)
	ops := operations.NewService(operations.NewMemoryRepository())
	s := NewService(broker, ops, "local-operator", "local")

	byID := map[string]RuntimeSummary{}
	for _, runtime := range s.Overview(context.Background()) {
		byID[runtime.Info.ID] = runtime
	}
	safeWorker := byID[executionbroker.LocalSafeWorkerID]
	if safeWorker.CanExecute ||
		safeWorker.Status != executionbroker.RuntimeBlocked {
		t.Fatalf("unauthorized safe worker summary = %#v", safeWorker)
	}

	attempt, ok := s.SelfTest(
		context.Background(),
		executionbroker.LocalSafeWorkerID,
	)
	if !ok {
		t.Fatal("safe worker self-test must remain discoverable")
	}
	if attempt.Status != AttemptFailed || attempt.VerificationPassed {
		t.Fatalf("unauthorized self-test = %#v, want failed and unverified", attempt)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("unauthorized self-test created %d artifacts", len(entries))
	}
	completed, err := ops.List(operations.Filter{
		OwnerUserID: "local-operator",
		WorkspaceID: "local",
		Status:      operations.StatusCompleted,
	})
	if err != nil {
		t.Fatalf("list completed operations: %v", err)
	}
	if len(completed) != 0 {
		t.Fatalf("unauthorized self-test completed %d operations", len(completed))
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

func TestSafeWorkerAttemptsRecoverFromOperationLedgerAfterRestart(t *testing.T) {
	repository := operations.NewMemoryRepository()
	firstOperations := operations.NewService(repository)
	first := NewService(
		newAuthorizedRuntimeLabTestBroker(t, t.TempDir(), "local-operator", "local"),
		firstOperations,
		"local-operator",
		"local",
	)
	attempt, ok := first.SelfTest(context.Background(), executionbroker.LocalSafeWorkerID)
	if !ok || attempt.Status != AttemptSucceeded {
		t.Fatalf("initial self-test = (%#v, %t), want succeeded", attempt, ok)
	}

	restarted := NewService(
		newAuthorizedRuntimeLabTestBroker(t, t.TempDir(), "local-operator", "local"),
		operations.NewService(repository),
		"local-operator",
		"local",
	)
	recovered := restarted.Attempts(executionbroker.LocalSafeWorkerID)
	if len(recovered) != 1 {
		t.Fatalf("recovered attempts = %d, want 1", len(recovered))
	}
	if recovered[0].OperationID != attempt.OperationID ||
		recovered[0].Status != AttemptSucceeded ||
		!recovered[0].VerificationPassed {
		t.Fatalf("recovered attempt = %#v, want verified original operation", recovered[0])
	}

	var safeWorker RuntimeSummary
	for _, runtime := range restarted.Overview(context.Background()) {
		if runtime.Info.ID == executionbroker.LocalSafeWorkerID {
			safeWorker = runtime
			break
		}
	}
	if safeWorker.LastAttempt == nil || safeWorker.LastAttempt.OperationID != attempt.OperationID {
		t.Fatalf("overview last attempt = %#v, want recovered operation %s", safeWorker.LastAttempt, attempt.OperationID)
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("OPENHANDS_BASE_URL", server.URL)
	t.Setenv("OPENHANDS_HEALTH_PATH", "/health")
	runtime := newRemoteRuntime("openhands", "OpenHands", "OPENHANDS_BASE_URL")

	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeBlocked || probe.DiscoveryState != "reachable_unverified" || probe.ProtocolValid {
		t.Fatalf("an unregistered OpenHands response must remain reachable but unverified: %#v", probe)
	}
	if _, err := runtime.Execute(context.Background(), map[string]any{"task": "do not run"}); err == nil {
		t.Fatal("a healthy OpenHands endpoint must not enable task execution")
	}
}

func TestOpenClawDiscoveryValidatesExactHealthContractWithoutGrantingExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":"live"}`))
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("OPENCLAW_BASE_URL", server.URL)
	runtime := newRemoteRuntime("openclaw", "OpenClaw", "OPENCLAW_BASE_URL")
	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeBlocked || probe.ReadinessLevel != ReadinessAvailable ||
		!probe.ProtocolValid || probe.IdentityVerified || probe.EvidenceSHA256 == "" {
		t.Fatalf("OpenClaw discovery must validate only the reviewed read-only contract: %#v", probe)
	}
	if runtime.HealthCheck(context.Background()).Claim != executionbroker.ClaimProbed {
		t.Fatal("schema-valid discovery should raise only the probe claim")
	}
	if _, err := runtime.Execute(context.Background(), map[string]any{"task": "do not run"}); err == nil {
		t.Fatal("OpenClaw discovery must not enable execution")
	}
}

func TestOpenClawRuntimeLabUsesCanonicalAgentRuntimeRegistry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":"live"}`))
	}))
	defer server.Close()

	t.Setenv("OPENCLAW_BASE_URL", "") // The legacy Runtime Lab endpoint must not be consulted.
	t.Setenv("OPENCLAW_AGENT_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_ENABLED", "true")
	t.Setenv("OPENCLAW_GATEWAY_URL", server.URL)
	t.Setenv("OPENCLAW_GATEWAY_PROTOCOL_DISCOVERY_ENABLED", "false")
	t.Setenv("OPENCLAW_GATEWAY_AUTH_DISCOVERY_ENABLED", "false")
	t.Setenv("OPENCLAW_GATEWAY_TASK_LEDGER_DISCOVERY_ENABLED", "false")
	t.Setenv("AGENT_RUNTIME_ALLOWED_HOSTS", "127.0.0.1")

	broker := newAuthorizedRuntimeLabTestBroker(t, t.TempDir(), "local-operator", "local")
	service := NewServiceWithAgentRuntimeRegistry(
		broker,
		operations.NewService(operations.NewMemoryRepository()),
		"local-operator",
		"local",
		agentruntime.DefaultRegistry(),
	)
	probe, ok := service.Probe(context.Background(), "openclaw")
	if !ok || probe.Status != executionbroker.RuntimeBlocked ||
		probe.ReadinessLevel != ReadinessAvailable || !probe.ProtocolValid ||
		probe.RuntimeVersion != "" || probe.Authenticated || probe.IdentityVerified {
		t.Fatalf("canonical OpenClaw probe = (%#v, %t)", probe, ok)
	}

	openclaw, ok := service.reg.Adapter("openclaw")
	if !ok {
		t.Fatal("OpenClaw adapter must remain available in Runtime Lab")
	}
	if _, err := openclaw.Execute(context.Background(), map[string]any{"task": "do not run"}); err == nil {
		t.Fatal("Runtime Lab must not gain OpenClaw execution authority from the canonical registry")
	}
}

func TestHermesDiscoveryUsesIdentityAndOptionalAuthenticatedCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok","platform":"hermes-agent","version":"2026.8.3"}`))
		case "/v1/capabilities":
			if got := r.Header.Get("Authorization"); got != "Bearer local-test-key" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"object":"hermes.api_server.capabilities","platform":"hermes-agent","features":{"skills_api":true,"audio_api":false,"run_submission":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("HERMES_BASE_URL", server.URL)
	t.Setenv("HERMES_API_SERVER_KEY", "local-test-key")
	runtime := newRemoteRuntime("hermes", "Hermes", "HERMES_BASE_URL")
	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeBlocked || probe.ReadinessLevel != ReadinessHealthChecked ||
		!probe.ProtocolValid || !probe.IdentityVerified || !probe.Authenticated || probe.RuntimeVersion != "2026.8.3" {
		t.Fatalf("Hermes discovery result = %#v", probe)
	}
	if len(probe.Capabilities) != 2 || probe.Capabilities[0] != "run_submission" || probe.Capabilities[1] != "skills_api" {
		t.Fatalf("bounded enabled capabilities = %#v", probe.Capabilities)
	}
}

func TestOdysseusDiscoveryUsesReviewedHealthAndVersionPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			_, _ = w.Write([]byte(`{"status":"healthy","timestamp":"2026-08-08T12:00:00Z"}`))
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.9.0-dev"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("ODYSSEUS_BASE_URL", server.URL)
	t.Setenv("ODYSSEUS_HEALTH_PATH", "")
	runtime := newRemoteRuntime("odysseus", "Odysseus", "ODYSSEUS_BASE_URL")
	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeBlocked || probe.ReadinessLevel != ReadinessAvailable ||
		!probe.ProtocolValid || probe.IdentityVerified || probe.RuntimeVersion != "0.9.0-dev" {
		t.Fatalf("Odysseus discovery result = %#v", probe)
	}
}

func TestProtocolDiscoveryAdvancesOnlyTheReadOnlyCapabilityCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","platform":"hermes-agent","version":"2026.8.3"}`))
	}))
	defer server.Close()

	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("HERMES_BASE_URL", server.URL)
	t.Setenv("HERMES_API_SERVER_KEY", "")
	svc := newTestService(t)
	probe, ok := svc.Probe(context.Background(), "hermes")
	if !ok || !probe.ProtocolValid {
		t.Fatalf("Hermes probe = (%#v, %t)", probe, ok)
	}
	overview, err := svc.CapabilityCards(context.Background())
	if err != nil {
		t.Fatalf("capability cards: %v", err)
	}
	for _, card := range overview.Cards {
		if card.RuntimeID != "hermes" {
			continue
		}
		if card.ID == "hermes.gateway.discovery" {
			if !card.CanInvoke || card.ReadinessLevel != ReadinessHealthChecked || card.LatestDiscovery == nil {
				t.Fatalf("Hermes discovery card did not advance: %#v", card)
			}
			continue
		}
		if card.CanInvoke || card.ReadinessLevel != ReadinessConfigured || card.CanExecuteExternalEffect {
			t.Fatalf("non-discovery Hermes card widened: %#v", card)
		}
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

func TestRemoteHealthProbeDoesNotExposeEndpointOnTransportFailure(t *testing.T) {
	t.Setenv(runtimeLabAllowedHostsEnv, "127.0.0.1")
	t.Setenv("OPENHANDS_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("OPENHANDS_HEALTH_PATH", "/health")
	runtime := newRemoteRuntime("openhands", "OpenHands", "OPENHANDS_BASE_URL")

	probe := runtime.Probe(context.Background(), time.Now().UTC())
	if probe.Status != executionbroker.RuntimeUnavailable {
		t.Fatalf("probe status = %q, want unavailable: %#v", probe.Status, probe)
	}
	if probe.Detail != "runtime discovery could not reach or validate the configured endpoint; review the local runtime configuration" {
		t.Fatalf("probe detail = %q", probe.Detail)
	}
	for _, forbidden := range []string{"127.0.0.1", ":1", "connect"} {
		if strings.Contains(strings.ToLower(probe.Detail), strings.ToLower(forbidden)) {
			t.Fatalf("probe detail leaked %q: %q", forbidden, probe.Detail)
		}
	}
}
