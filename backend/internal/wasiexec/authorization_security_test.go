package wasiexec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"github.com/google/uuid"
)

type memoryRunRepository struct {
	mu   sync.Mutex
	runs map[uuid.UUID]models.WASIRun
}

func newMemoryRunRepository() *memoryRunRepository {
	return &memoryRunRepository{runs: map[uuid.UUID]models.WASIRun{}}
}

func (r *memoryRunRepository) Create(run *models.WASIRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = *run
	return nil
}

func (r *memoryRunRepository) Save(run *models.WASIRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = *run
	return nil
}

func (r *memoryRunRepository) ListForOwner(
	owner string,
	limit int,
) ([]models.WASIRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]models.WASIRun, 0, len(r.runs))
	for _, run := range r.runs {
		if run.OwnerIdentity == owner {
			out = append(out, run)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type finalEffectAuthorizerFunc func(
	context.Context,
	executionauth.Request,
	string,
	string,
) (executionauth.Receipt, error)

func (f finalEffectAuthorizerFunc) AuthorizeAndConsume(
	ctx context.Context,
	request executionauth.Request,
	consumer string,
	target string,
) (executionauth.Receipt, error) {
	return f(ctx, request, consumer, target)
}

func TestRunFailsClosedWithoutFinalEffectAuthorizer(t *testing.T) {
	clearEmergencyStop(t)
	var runnerCalls atomic.Int32
	runner := newWASIRunner(t, &runnerCalls, nil)
	service := NewService(
		newMemoryRunRepository(),
		true,
		runner.URL,
		"1234567890abcdef",
		testModules(),
	)

	run, err := service.Run(context.Background(), "owner-1", validRunRequest())

	if !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("expected fail-closed authorization error, got %v", err)
	}
	if run == nil || run.Status != "blocked" {
		t.Fatalf("expected a persisted blocked run, got %#v", run)
	}
	if got := runnerCalls.Load(); got != 0 {
		t.Fatalf("runner was invoked %d times without an authorizer", got)
	}
}

func TestRunConsumesExactOwnerBoundEffectImmediatelyBeforeRunner(t *testing.T) {
	clearEmergencyStop(t)
	var runnerCalls atomic.Int32
	var events []string
	runner := newWASIRunner(t, &runnerCalls, func() {
		events = append(events, "runner")
	})
	request := validRunRequest()
	module := testModules()[0]
	expectedBinding := buildFinalEffectBinding("owner-1", request, module, runner.URL)
	expectedDigest, err := finalEffectDigest(expectedBinding)
	if err != nil {
		t.Fatalf("finalEffectDigest: %v", err)
	}

	authorizer := finalEffectAuthorizerFunc(func(
		_ context.Context,
		authRequest executionauth.Request,
		consumer string,
		target string,
	) (executionauth.Receipt, error) {
		events = append(events, "authorize")
		if authRequest.OwnerIdentity != "owner-1" ||
			authRequest.ActorIdentity != "owner-1" ||
			authRequest.ActorKind != executionauth.ActorHuman {
			t.Fatalf("owner/actor binding is incomplete: %#v", authRequest)
		}
		if authRequest.Action != wasiRunAction ||
			authRequest.Stage != executionauth.StageExecution ||
			authRequest.ResourceType != wasiResourceType ||
			authRequest.ResourceID != module.ID+"@sha256:"+module.SHA256 {
			t.Fatalf("action/resource binding is incomplete: %#v", authRequest)
		}
		if authRequest.RuntimeID != wasiRuntimeID ||
			authRequest.TaskID != request.TaskID ||
			authRequest.ProjectKey != request.ProjectKey {
			t.Fatalf("runtime/task/project binding is incomplete: %#v", authRequest)
		}
		if authRequest.ApprovalSourceID != request.ApprovalSourceID ||
			authRequest.ApprovalBindingDigest != request.ApprovalBindingDigest {
			t.Fatalf("approval binding is incomplete: %#v", authRequest)
		}
		if authRequest.Facts["module_sha256"] != module.SHA256 ||
			authRequest.EffectDigest != expectedDigest {
			t.Fatalf("module/effect binding is incomplete: %#v", authRequest)
		}
		if consumer != wasiConsumer || target != wasiConsumer+":"+expectedDigest {
			t.Fatalf("unexpected consumption boundary: consumer=%q target=%q", consumer, target)
		}
		return executionauth.Receipt{
			Outcome:       executionauth.OutcomeAuthorized,
			OwnerIdentity: authRequest.OwnerIdentity,
			EffectDigest:  authRequest.EffectDigest,
		}, nil
	})
	service := NewServiceWithAuthorizer(
		newMemoryRunRepository(),
		true,
		runner.URL,
		"1234567890abcdef",
		testModules(),
		authorizer,
	)

	run, err := service.Run(context.Background(), "owner-1", request)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run == nil || run.Status != "completed" {
		t.Fatalf("expected completed run, got %#v", run)
	}
	if strings.Join(events, ",") != "authorize,runner" {
		t.Fatalf("authorization was not immediately before runner: %v", events)
	}
}

func TestRunRejectsMismatchedAuthorizationReceipt(t *testing.T) {
	clearEmergencyStop(t)
	var runnerCalls atomic.Int32
	runner := newWASIRunner(t, &runnerCalls, nil)
	authorizer := finalEffectAuthorizerFunc(func(
		_ context.Context,
		request executionauth.Request,
		_ string,
		_ string,
	) (executionauth.Receipt, error) {
		return executionauth.Receipt{
			Outcome:       executionauth.OutcomeAuthorized,
			OwnerIdentity: "different-owner",
			EffectDigest:  request.EffectDigest,
		}, nil
	})
	service := NewServiceWithAuthorizer(
		newMemoryRunRepository(),
		true,
		runner.URL,
		"1234567890abcdef",
		testModules(),
		authorizer,
	)

	run, err := service.Run(context.Background(), "owner-1", validRunRequest())

	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected mismatched receipt to be rejected, got %v", err)
	}
	if run == nil || run.Status != "blocked" {
		t.Fatalf("expected blocked run, got %#v", run)
	}
	if got := runnerCalls.Load(); got != 0 {
		t.Fatalf("runner was invoked %d times with mismatched receipt", got)
	}
}

func TestRunRechecksEmergencyStopAfterAuthorizationConsumption(t *testing.T) {
	var stopped atomic.Bool
	restore := safety.SetEmergencyStopProvider(safety.EmergencyStopProviderFunc(
		func() (bool, string, error) {
			return stopped.Load(), "operator stop engaged during authorization", nil
		},
	))
	t.Cleanup(restore)
	clearEmergencyStopEnvironment(t)

	var runnerCalls atomic.Int32
	runner := newWASIRunner(t, &runnerCalls, nil)
	authorizer := finalEffectAuthorizerFunc(func(
		_ context.Context,
		request executionauth.Request,
		_ string,
		_ string,
	) (executionauth.Receipt, error) {
		stopped.Store(true)
		return executionauth.Receipt{
			Outcome:       executionauth.OutcomeAuthorized,
			OwnerIdentity: request.OwnerIdentity,
			EffectDigest:  request.EffectDigest,
		}, nil
	})
	service := NewServiceWithAuthorizer(
		newMemoryRunRepository(),
		true,
		runner.URL,
		"1234567890abcdef",
		testModules(),
		authorizer,
	)

	run, err := service.Run(context.Background(), "owner-1", validRunRequest())

	if !errors.Is(err, ErrEmergencyStopActive) {
		t.Fatalf("expected final emergency-stop check to block, got %v", err)
	}
	if run == nil || run.Status != "blocked" {
		t.Fatalf("expected blocked run, got %#v", run)
	}
	if got := runnerCalls.Load(); got != 0 {
		t.Fatalf("runner was invoked %d times after emergency stop", got)
	}
}

func TestRunRequiresTaskProjectAndApprovalBinding(t *testing.T) {
	clearEmergencyStop(t)
	var authorizationCalls atomic.Int32
	authorizer := finalEffectAuthorizerFunc(func(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error) {
		authorizationCalls.Add(1)
		return executionauth.Receipt{}, nil
	})
	service := NewServiceWithAuthorizer(
		newMemoryRunRepository(),
		true,
		"http://127.0.0.1:8090",
		"1234567890abcdef",
		testModules(),
		authorizer,
	)

	for name, mutate := range map[string]func(*RunRequest){
		"task":             func(request *RunRequest) { request.TaskID = "" },
		"project":          func(request *RunRequest) { request.ProjectKey = "" },
		"approval source":  func(request *RunRequest) { request.ApprovalSourceID = "" },
		"approval binding": func(request *RunRequest) { request.ApprovalBindingDigest = "" },
	} {
		t.Run(name, func(t *testing.T) {
			request := validRunRequest()
			mutate(&request)
			if _, err := service.Run(context.Background(), "owner-1", request); !errors.Is(err, ErrInvalidRunRequest) {
				t.Fatalf("expected invalid request, got %v", err)
			}
		})
	}
	if got := authorizationCalls.Load(); got != 0 {
		t.Fatalf("authorizer was called %d times for incomplete requests", got)
	}
}

func newWASIRunner(
	t *testing.T,
	calls *atomic.Int32,
	onCall func(),
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if onCall != nil {
			onCall()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","summary":"bounded module completed","exitCode":0}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func testModules() []Module {
	return []Module{{
		ID:     "health",
		Name:   "Health probe",
		File:   "health.wasm",
		SHA256: testModuleHash,
	}}
}

func validRunRequest() RunRequest {
	return RunRequest{
		ModuleID:              "health",
		TaskID:                "task-123",
		ProjectKey:            "project-hai",
		ApprovalSourceID:      "approval-456",
		ApprovalBindingDigest: strings.Repeat("a", 64),
	}
}

func clearEmergencyStop(t *testing.T) {
	t.Helper()
	restore := safety.SetEmergencyStopProvider(safety.EmergencyStopProviderFunc(
		func() (bool, string, error) { return false, "", nil },
	))
	t.Cleanup(restore)
	clearEmergencyStopEnvironment(t)
}

func clearEmergencyStopEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("HAI_EMERGENCY_STOP", "")
	t.Setenv("AUTONOMY_EMERGENCY_STOP", "")
	t.Setenv("EMERGENCY_STOP", "")
}
