package agentregistry

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

var testNow = time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

func TestRegistryIsOwnerScopedAndUsesOptimisticRevisions(t *testing.T) {
	service, repository := newTestService(t)
	alice := registerAgent(t, service, testAgent("alice@example.com", "shared"))
	bob := registerAgent(t, service, testAgent("bob@example.com", "shared"))

	if alice.OwnerIdentity == bob.OwnerIdentity {
		t.Fatal("test setup did not create separate owners")
	}
	if _, err := service.Get(context.Background(), "alice@example.com", bob.ID); err != nil {
		t.Fatalf("same id in alice scope should still resolve alice's record: %v", err)
	}
	if _, err := service.Get(context.Background(), "carol@example.com", alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get error = %v, want ErrNotFound", err)
	}

	replacement := cloneAgent(alice)
	replacement.Name = "Updated"
	updated, err := service.Update(context.Background(), alice.OwnerIdentity, replacement, alice.Revision)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Revision != alice.Revision+1 {
		t.Fatalf("revision = %d, want %d", updated.Revision, alice.Revision+1)
	}
	if _, err := service.Update(context.Background(), alice.OwnerIdentity, replacement, alice.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v, want ErrConflict", err)
	}

	list, err := repository.List(context.Background(), "alice@example.com")
	if err != nil || len(list) != 1 {
		t.Fatalf("owner list = %#v, %v", list, err)
	}
}

func TestLifecycleTransitionsFailClosed(t *testing.T) {
	service, _ := newTestService(t)
	agent := registerAgent(t, service, testAgent("alice@example.com", "worker"))

	if _, err := service.Transition(context.Background(), agent.OwnerIdentity, agent.ID, agent.Revision, StateDraining, "start drain"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("registered to draining error = %v, want ErrInvalidTransition", err)
	}
	enabled, err := service.Transition(context.Background(), agent.OwnerIdentity, agent.ID, agent.Revision, StateEnabled, "operator enabled")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	quarantined, err := service.Transition(context.Background(), enabled.OwnerIdentity, enabled.ID, enabled.Revision, StateQuarantined, "health policy violation")
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if _, err := service.Transition(context.Background(), quarantined.OwnerIdentity, quarantined.ID, quarantined.Revision, StateEnabled, "skip remediation"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("quarantined to enabled error = %v, want ErrInvalidTransition", err)
	}
	disabled, err := service.Transition(context.Background(), quarantined.OwnerIdentity, quarantined.ID, quarantined.Revision, StateDisabled, "remediation starts disabled")
	if err != nil {
		t.Fatalf("quarantine to disabled: %v", err)
	}
	if disabled.State != StateDisabled {
		t.Fatalf("state = %s, want disabled", disabled.State)
	}
	transitions, err := service.ListTransitions(context.Background(), agent.OwnerIdentity, agent.ID)
	if err != nil {
		t.Fatalf("transitions: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("transition count = %d, want 3", len(transitions))
	}
}

func TestEnableRequiresFreshHealthyReadiness(t *testing.T) {
	service, _ := newTestService(t)
	for index, test := range []struct {
		name   string
		mutate func(*Agent)
	}{
		{name: "unhealthy", mutate: func(agent *Agent) { agent.Health.Status = HealthUnhealthy }},
		{name: "not ready", mutate: func(agent *Agent) { agent.Health.Ready = false }},
		{name: "stale", mutate: func(agent *Agent) { agent.Health.CheckedAt = testNow.Add(-2 * time.Hour) }},
		{name: "unavailable", mutate: func(agent *Agent) { agent.Availability.Available = false }},
		{name: "at capacity", mutate: func(agent *Agent) { agent.Availability.ActiveAssignments = agent.Availability.MaxConcurrent }},
	} {
		t.Run(test.name, func(t *testing.T) {
			agent := testAgent("alice@example.com", "agent-"+string(rune('a'+index)))
			test.mutate(&agent)
			registered := registerAgent(t, service, agent)
			if _, err := service.Transition(context.Background(), registered.OwnerIdentity, registered.ID, registered.Revision, StateEnabled, "enable"); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("enable error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestAssignmentRejectsCapabilityHealthAuthorityAndCompatibilityMismatches(t *testing.T) {
	tests := []struct {
		name          string
		mutateAgent   func(*Agent)
		mutateRequest func(*AssignmentRequest)
	}{
		{name: "capability", mutateRequest: func(request *AssignmentRequest) {
			request.Capabilities[0].ID = "legal-drafting"
		}},
		{name: "operation", mutateRequest: func(request *AssignmentRequest) {
			request.Capabilities[0].Operations = []string{"delete"}
		}},
		{name: "version", mutateRequest: func(request *AssignmentRequest) {
			request.Capabilities[0].MinVersion = "3.0.0"
			request.Capabilities[0].MaxVersion = "3.9.0"
		}},
		{name: "authority", mutateRequest: func(request *AssignmentRequest) {
			request.RequiredAuthority = 7
			request.PolicyMaxAuthority = 7
		}},
		{name: "autonomy", mutateRequest: func(request *AssignmentRequest) {
			request.RequiredAutonomy = 7
			request.PolicyMaxAutonomy = 7
		}},
		{name: "runtime adapter", mutateRequest: func(request *AssignmentRequest) {
			request.Compatibility.RuntimeAdapterID = "openclaw"
		}},
		{name: "protocol version", mutateRequest: func(request *AssignmentRequest) {
			request.Compatibility.MinProtocolVersion = "2.0.0"
			request.Compatibility.MaxProtocolVersion = "2.9.0"
		}},
		{name: "tool allowlist", mutateRequest: func(request *AssignmentRequest) {
			request.RequiredTools = []string{"shell"}
		}},
		{name: "folder allowlist", mutateRequest: func(request *AssignmentRequest) {
			request.RequiredFolders = []string{"C:/Windows/System32"}
		}},
		{name: "unhealthy", mutateAgent: func(agent *Agent) { agent.Health.Status = HealthUnhealthy }},
		{name: "quarantined", mutateAgent: func(agent *Agent) { agent.State = StateQuarantined }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, repository := newTestService(t)
			agent := testAgent("alice@example.com", "worker")
			if test.mutateAgent != nil {
				test.mutateAgent(&agent)
			}
			registered := registerAgent(t, service, agent)
			if agent.State == StateQuarantined {
				registered.State = StateQuarantined
				registered.Revision++
				if _, err := repository.CompareAndSwap(context.Background(), registered, registered.Revision-1); err != nil {
					t.Fatalf("seed quarantine: %v", err)
				}
			} else if registered.Health.Status != HealthUnhealthy {
				registered = enableAgent(t, service, registered)
			} else {
				// Unhealthy agents cannot be enabled via the lifecycle service.
				registered.State = StateEnabled
				registered.Revision++
				if _, err := repository.CompareAndSwap(context.Background(), registered, registered.Revision-1); err != nil {
					t.Fatalf("seed unhealthy enabled agent: %v", err)
				}
			}
			request := testRequest()
			if test.mutateRequest != nil {
				test.mutateRequest(&request)
			}
			if _, err := service.Assign(context.Background(), request); !errors.Is(err, ErrNoEligibleAgent) {
				t.Fatalf("assignment error = %v, want ErrNoEligibleAgent", err)
			}
		})
	}
}

func TestAssignmentIsDeterministicExplainedAndImmutable(t *testing.T) {
	service, _ := newTestService(t)
	first := testAgent("alice@example.com", "a-local")
	first.Reliability = ReliabilityEvidence{Successes: 9, Failures: 1, MeanLatencyMs: 100}
	second := testAgent("alice@example.com", "b-cloud")
	second.Performance.Locality = LocalityCloud
	second.Performance.EstimatedCostEUR = 0.05
	second.Availability.ActiveAssignments = 1
	second.Reliability = ReliabilityEvidence{Successes: 9, Failures: 1, MeanLatencyMs: 100}
	enableAgent(t, service, registerAgent(t, service, first))
	enableAgent(t, service, registerAgent(t, service, second))

	request := testRequest()
	maxCost := 1.0
	request.MaxEstimatedCostEUR = &maxCost
	assignment, err := service.Assign(context.Background(), request)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment.AgentID != "a-local" {
		t.Fatalf("selected agent = %s, want a-local", assignment.AgentID)
	}
	if !assignment.Explanation.Eligible || len(assignment.Explanation.Components) != 7 ||
		len(assignment.Explanation.Constraints) == 0 {
		t.Fatalf("incomplete explanation: %#v", assignment.Explanation)
	}
	if assignment.GrantedAuthority != request.RequiredAuthority ||
		assignment.GrantedAutonomy != request.RequiredAutonomy {
		t.Fatalf("assignment granted broader authority: %#v", assignment)
	}
	if assignment.Score < 0 || assignment.Score > 1 {
		t.Fatalf("assignment score = %f, want normalized score", assignment.Score)
	}
	reservedAgent, err := service.Get(context.Background(), request.OwnerIdentity, assignment.AgentID)
	if err != nil {
		t.Fatalf("get reserved agent: %v", err)
	}
	if reservedAgent.Availability.ActiveAssignments != 1 ||
		reservedAgent.Revision != assignment.AgentRevision+1 {
		t.Fatalf("assignment did not atomically reserve capacity: %#v", reservedAgent)
	}

	repeated, err := service.Assign(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat assign: %v", err)
	}
	if !reflect.DeepEqual(assignment, repeated) {
		t.Fatalf("repeat assignment changed:\nfirst %#v\nsecond %#v", assignment, repeated)
	}
	assignment.Explanation.Components[0].Reason = "mutated"
	stored, err := service.GetAssignment(context.Background(), request.OwnerIdentity, assignment.ID)
	if err != nil {
		t.Fatalf("get assignment: %v", err)
	}
	if stored.Explanation.Components[0].Reason == "mutated" {
		t.Fatal("stored assignment explanation was mutable through returned value")
	}
	if stored.RequestDigest == "" {
		t.Fatal("assignment did not retain immutable request digest")
	}
}

func TestAssignmentTieBreaksByAgentID(t *testing.T) {
	service, _ := newTestService(t)
	for _, id := range []string{"z-agent", "a-agent"} {
		enableAgent(t, service, registerAgent(t, service, testAgent("alice@example.com", id)))
	}
	assignment, err := service.Assign(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment.AgentID != "a-agent" {
		t.Fatalf("tie selected %s, want lexical a-agent", assignment.AgentID)
	}
}

func TestAssignmentCapacityIsReservedUntilOutcome(t *testing.T) {
	service, _ := newTestService(t)
	agent := testAgent("alice@example.com", "single-slot")
	agent.Availability.MaxConcurrent = 1
	enableAgent(t, service, registerAgent(t, service, agent))

	firstRequest := testRequest()
	first, err := service.Assign(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first assignment: %v", err)
	}
	secondRequest := testRequest()
	secondRequest.TaskID = "task-2"
	if _, err := service.Assign(context.Background(), secondRequest); !errors.Is(err, ErrNoEligibleAgent) {
		t.Fatalf("second assignment error = %v, want ErrNoEligibleAgent while at capacity", err)
	}
	reserved, err := service.Get(context.Background(), first.OwnerIdentity, first.AgentID)
	if err != nil {
		t.Fatalf("get reserved agent: %v", err)
	}
	if _, err := service.RecordAssignmentOutcome(
		context.Background(),
		first.OwnerIdentity,
		first.ID,
		reserved.Revision,
		Outcome{Success: true, RecordedAt: testNow},
	); err != nil {
		t.Fatalf("release first assignment: %v", err)
	}
	if _, err := service.Assign(context.Background(), secondRequest); err != nil {
		t.Fatalf("second assignment after release: %v", err)
	}
}

func TestAssignmentOutcomeReleasesCapacityAndOnlyChangesBoundedEvidence(t *testing.T) {
	service, _ := newTestService(t)
	registered := registerAgent(t, service, testAgent("alice@example.com", "worker"))
	enableAgent(t, service, registered)
	assignment, err := service.Assign(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	before, err := service.Get(context.Background(), assignment.OwnerIdentity, assignment.AgentID)
	if err != nil {
		t.Fatalf("get reserved agent: %v", err)
	}
	if before.Availability.ActiveAssignments != 1 {
		t.Fatalf("active assignments before outcome = %d, want 1", before.Availability.ActiveAssignments)
	}

	updated, err := service.RecordAssignmentOutcome(
		context.Background(),
		assignment.OwnerIdentity,
		assignment.ID,
		before.Revision,
		Outcome{
			Success: true, Latency: 250 * time.Millisecond, RecordedAt: testNow,
		},
	)
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	if updated.Reliability.Successes != before.Reliability.Successes+1 {
		t.Fatalf("success evidence = %d", updated.Reliability.Successes)
	}
	if updated.AuthorityCeiling != before.AuthorityCeiling ||
		updated.AutonomyCeiling != before.AutonomyCeiling ||
		!reflect.DeepEqual(updated.ToolAllowlist, before.ToolAllowlist) ||
		!reflect.DeepEqual(updated.DataAllowlist, before.DataAllowlist) ||
		!reflect.DeepEqual(updated.FolderAllowlist, before.FolderAllowlist) ||
		!reflect.DeepEqual(updated.Capabilities, before.Capabilities) {
		t.Fatal("outcome learning expanded or changed agent authority")
	}
	if updated.Reliability.Score() < 0 || updated.Reliability.Score() > 1 {
		t.Fatalf("reliability score out of bounds: %f", updated.Reliability.Score())
	}
	if updated.Availability.ActiveAssignments != 0 {
		t.Fatalf("active assignments after outcome = %d, want 0", updated.Availability.ActiveAssignments)
	}
	if _, err := service.RecordAssignmentOutcome(
		context.Background(),
		assignment.OwnerIdentity,
		assignment.ID,
		updated.Revision,
		Outcome{Success: true},
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate outcome error = %v, want ErrConflict", err)
	}
}

func TestConfigurationUpdateCannotForgeActiveAssignmentCount(t *testing.T) {
	service, _ := newTestService(t)
	registered := registerAgent(t, service, testAgent("alice@example.com", "worker"))
	enableAgent(t, service, registered)
	assignment, err := service.Assign(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	current, err := service.Get(context.Background(), assignment.OwnerIdentity, assignment.AgentID)
	if err != nil {
		t.Fatalf("get assigned agent: %v", err)
	}
	replacement := cloneAgent(current)
	replacement.Availability.ActiveAssignments = 0
	replacement.Availability.MaxConcurrent = 3
	updated, err := service.Update(
		context.Background(),
		current.OwnerIdentity,
		replacement,
		current.Revision,
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Availability.ActiveAssignments != 1 {
		t.Fatalf("configuration update forged active assignment count: %#v", updated.Availability)
	}
	if updated.Availability.MaxConcurrent != 3 {
		t.Fatalf("configured maximum capacity = %d, want 3", updated.Availability.MaxConcurrent)
	}
}

func TestValidationRejectsSecretsAndInvalidVersionRanges(t *testing.T) {
	service, _ := newTestService(t)
	secret := testAgent("alice@example.com", "worker")
	secret.Health.Reason = "authorization=Bearer-secret"
	if _, err := service.Register(context.Background(), secret); err == nil {
		t.Fatal("agent secret was accepted")
	}

	valid := registerAgent(t, service, testAgent("alice@example.com", "valid"))
	enableAgent(t, service, valid)
	request := testRequest()
	request.Capabilities[0].MinVersion = "2.0.0"
	request.Capabilities[0].MaxVersion = "1.0.0"
	if _, err := service.Assign(context.Background(), request); err == nil {
		t.Fatal("inverted version range was accepted")
	}
}

func newTestService(t *testing.T) (*Service, *MemoryRepository) {
	t.Helper()
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return testNow })
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service, repository
}

func testAgent(owner, id string) Agent {
	return Agent{
		ID: id, OwnerIdentity: owner, Name: id, Type: AgentTypeExecutor,
		Runtime: RuntimeAdapter{ID: "hermes", Type: "claw", ProtocolVersion: "1.2.0"},
		Capabilities: []CapabilityDeclaration{{
			ID: "code", Version: "2.1.0", Operations: []string{"read", "write"},
		}},
		AuthorityCeiling: 5, AutonomyCeiling: 5,
		ToolAllowlist:   []string{"git", "editor"},
		DataAllowlist:   []string{"project:hai"},
		FolderAllowlist: []string{"C:/workspace/hai"},
		Health: HealthEvidence{
			Status: HealthHealthy, Ready: true, CheckedAt: testNow, FreshFor: time.Hour,
		},
		Availability: Availability{Available: true, MaxConcurrent: 2},
		Performance: PerformanceProfile{
			EstimatedCostEUR: 0, P95LatencyMs: 1000, Locality: LocalityLocal,
		},
	}
}

func testRequest() AssignmentRequest {
	return AssignmentRequest{
		OwnerIdentity: "alice@example.com",
		TaskID:        "task-1",
		Capabilities: []CapabilityRequirement{{
			ID: "code", MinVersion: "2.0.0", MaxVersion: "2.9.0", Operations: []string{"read"},
		}},
		Compatibility: CompatibilityRequirement{
			RuntimeType: "claw", MinProtocolVersion: "1.0.0", MaxProtocolVersion: "1.9.0",
		},
		RequiredAuthority: 2, RequiredAutonomy: 2,
		PolicyMaxAuthority: 4, PolicyMaxAutonomy: 4,
		RequiredTools:   []string{"git"},
		RequiredData:    []string{"project:hai"},
		RequiredFolders: []string{"C:/workspace/hai/src"},
	}
}

func registerAgent(t *testing.T, service *Service, agent Agent) Agent {
	t.Helper()
	registered, err := service.Register(context.Background(), agent)
	if err != nil {
		t.Fatalf("register %s: %v", agent.ID, err)
	}
	return registered
}

func enableAgent(t *testing.T, service *Service, agent Agent) Agent {
	t.Helper()
	enabled, err := service.Transition(context.Background(), agent.OwnerIdentity, agent.ID, agent.Revision, StateEnabled, "test enable")
	if err != nil {
		t.Fatalf("enable %s: %v", agent.ID, err)
	}
	return enabled
}
