package standingmandate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStandingMandateLifecycleAndOwnerIsolation(t *testing.T) {
	service, now := newTestService(t)
	mandate := createTestMandate(t, service, *now)
	if mandate.Status != StatusDraft || mandate.Revision != 1 {
		t.Fatalf("created mandate = %#v", mandate)
	}

	active, err := service.Activate(context.Background(), "robert", mandate.ID, mandate.Revision)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if active.Status != StatusActive || active.ActivatedAt == nil || active.Revision != 2 {
		t.Fatalf("active mandate = %#v", active)
	}
	if _, err := service.Get(context.Background(), "other-owner", mandate.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner isolation error = %v, want ErrNotFound", err)
	}
	if _, err := service.Revoke(context.Background(), "robert", mandate.ID, 1, "robert", "obsolete"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revoke error = %v, want ErrRevisionConflict", err)
	}

	revoked, err := service.Revoke(context.Background(), "robert", mandate.ID, active.Revision, "robert", "scope retired")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revoked.Status != StatusRevoked || revoked.RevokedAt == nil ||
		revoked.RevokedBy != "robert" || revoked.RevocationReason != "scope retired" {
		t.Fatalf("revoked mandate = %#v", revoked)
	}
}

func TestAuthorizeExactBoundedAction(t *testing.T) {
	service, now := newTestService(t)
	mandate := activateTestMandate(t, service, *now)
	request := validAction(*now)

	decision, err := service.Authorize(context.Background(), mandate.ID, request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if decision.Outcome != DecisionAuthorized || decision.EffectiveAutonomy != 4 {
		t.Fatalf("decision = %#v", decision)
	}
	if len(decision.Evidence.MatchedScopeIDs) != 1 ||
		len(decision.Evidence.RequestDigest) != 64 ||
		len(decision.Evidence.MandateDigest) != 64 ||
		len(decision.Evidence.DecisionDigest) != 64 {
		t.Fatalf("audit evidence = %#v", decision.Evidence)
	}
}

func TestAuthorizeDeniesOutOfScopeAndAutonomyAboveCeiling(t *testing.T) {
	service, now := newTestService(t)
	mandate := activateTestMandate(t, service, *now)

	outOfScope := validAction(*now)
	outOfScope.ProjectKey = "different-project"
	decision, err := service.Authorize(context.Background(), mandate.ID, outOfScope)
	if err != nil {
		t.Fatalf("Authorize out of scope: %v", err)
	}
	if decision.Outcome != DecisionDenied || decision.Reason != "action is outside every mandate scope" {
		t.Fatalf("out-of-scope decision = %#v", decision)
	}

	aboveCeiling := validAction(*now)
	aboveCeiling.RequestedAutonomy = 7
	decision, err = service.Authorize(context.Background(), mandate.ID, aboveCeiling)
	if err != nil {
		t.Fatalf("Authorize above ceiling: %v", err)
	}
	if decision.Outcome != DecisionDenied || decision.Reason != "requested autonomy exceeds mandate ceiling" {
		t.Fatalf("ceiling decision = %#v", decision)
	}
}

func TestApprovalPolicyRequiresExactFreshEvidence(t *testing.T) {
	service, now := newTestService(t)
	mandate := activateTestMandate(t, service, *now)
	request := validAction(*now)
	request.Risk = RiskHigh

	pending, err := service.Authorize(context.Background(), mandate.ID, request)
	if err != nil {
		t.Fatalf("Authorize pending: %v", err)
	}
	if pending.Outcome != DecisionRequiresApproval || !pending.ApprovalRequired || pending.ApprovalSatisfied {
		t.Fatalf("pending approval decision = %#v", pending)
	}

	request.Approval = &ApprovalEvidence{
		ID:            "approval-1",
		ApprovedBy:    "robert",
		ApproverRoles: []string{"owner"},
		ActionDigest:  pending.Evidence.RequestDigest,
		ApprovedAt:    now.Add(-time.Minute),
		ExpiresAt:     now.Add(time.Minute),
		Source:        "approval-queue:decision-1",
	}
	authorized, err := service.Authorize(context.Background(), mandate.ID, request)
	if err != nil {
		t.Fatalf("Authorize approved: %v", err)
	}
	if authorized.Outcome != DecisionAuthorized || !authorized.ApprovalSatisfied {
		t.Fatalf("approved decision = %#v", authorized)
	}

	request.ResourceID = "task-2"
	tampered, err := service.Authorize(context.Background(), mandate.ID, request)
	if err != nil {
		t.Fatalf("Authorize tampered: %v", err)
	}
	if tampered.Outcome != DecisionRequiresApproval ||
		tampered.Reason != "approval evidence is bound to a different action" {
		t.Fatalf("tampered decision = %#v", tampered)
	}
}

func TestStopConditionsDenyOrEscalateAndMissingRequiredFactsFailClosed(t *testing.T) {
	service, now := newTestService(t)
	mandate := activateTestMandate(t, service, *now)

	missing := validAction(*now)
	missing.Facts = nil
	decision, err := service.Authorize(context.Background(), mandate.ID, missing)
	if err != nil {
		t.Fatalf("Authorize missing fact: %v", err)
	}
	if decision.Outcome != DecisionDenied || len(decision.Evidence.TriggeredStops) == 0 {
		t.Fatalf("missing fact decision = %#v", decision)
	}

	budget := validAction(*now)
	budget.Facts = map[string]string{"emergency_stop": "false", "cost_eur": "20"}
	decision, err = service.Authorize(context.Background(), mandate.ID, budget)
	if err != nil {
		t.Fatalf("Authorize budget stop: %v", err)
	}
	if decision.Outcome != DecisionRequiresApproval {
		t.Fatalf("budget decision = %#v", decision)
	}

	stopped := validAction(*now)
	stopped.Facts = map[string]string{"emergency_stop": "true", "cost_eur": "0"}
	decision, err = service.Authorize(context.Background(), mandate.ID, stopped)
	if err != nil {
		t.Fatalf("Authorize emergency stop: %v", err)
	}
	if decision.Outcome != DecisionDenied {
		t.Fatalf("emergency-stop decision = %#v", decision)
	}
}

func TestExpiredAndRevokedMandatesFailClosed(t *testing.T) {
	service, now := newTestService(t)
	mandate := activateTestMandate(t, service, *now)
	*now = now.Add(3 * time.Hour)

	expired, err := service.Authorize(context.Background(), mandate.ID, validAction(*now))
	if err != nil {
		t.Fatalf("Authorize expired: %v", err)
	}
	if expired.Outcome != DecisionDenied || expired.Reason != "mandate has expired" {
		t.Fatalf("expired decision = %#v", expired)
	}

	*now = now.Add(-4 * time.Hour)
	second := activateTestMandate(t, service, *now)
	revoked, err := service.Revoke(context.Background(), "robert", second.ID, second.Revision, "robert", "manual stop")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	decision, err := service.Authorize(context.Background(), revoked.ID, validAction(*now))
	if err != nil {
		t.Fatalf("Authorize revoked: %v", err)
	}
	if decision.Outcome != DecisionDenied || decision.Reason != "mandate is not active" {
		t.Fatalf("revoked decision = %#v", decision)
	}
}

func TestCreateRejectsUnboundedAndInvalidMandates(t *testing.T) {
	service, now := newTestService(t)
	request := testCreateRequest(*now)
	request.Scopes[0].Projects = nil
	request.Scopes[0].Resources = nil
	request.Scopes[0].Tools = nil
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("expected unbounded scope rejection")
	}

	request = testCreateRequest(*now)
	request.Scopes[0].Actions = []string{"*"}
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("expected wildcard action rejection")
	}

	request = testCreateRequest(*now)
	request.AutonomyCeiling = 11
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("expected autonomy ceiling rejection")
	}
}

func newTestService(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	now := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	service, err := NewService(NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, &now
}

func createTestMandate(t *testing.T, service *Service, now time.Time) *StandingMandate {
	t.Helper()
	mandate, err := service.Create(context.Background(), testCreateRequest(now))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return mandate
}

func activateTestMandate(t *testing.T, service *Service, now time.Time) *StandingMandate {
	t.Helper()
	mandate := createTestMandate(t, service, now)
	active, err := service.Activate(context.Background(), "robert", mandate.ID, mandate.Revision)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	return active
}

func testCreateRequest(now time.Time) CreateRequest {
	expires := now.Add(2 * time.Hour)
	return CreateRequest{
		OwnerIdentity:   "robert",
		Name:            "Safe task administration",
		Purpose:         "Allow a bounded worker to update one project's tasks.",
		Version:         "1.0.0",
		AutonomyCeiling: 6,
		Scopes: []Scope{{
			ID:      "project-task-updates",
			Actions: []string{"task.update"},
			Resources: []ResourceScope{{
				Type: "task",
				IDs:  []string{"task-1", "task-2"},
			}},
			Projects:    []string{"hai"},
			Tools:       []string{"local-task-worker"},
			MaximumRisk: RiskHigh,
		}},
		ApprovalPolicy: ApprovalPolicy{
			Mode:                      ApprovalForRiskOrAction,
			RiskLevels:                []RiskLevel{RiskHigh, RiskCritical},
			ApproverRoles:             []string{"owner"},
			MaximumEvidenceAgeSeconds: 300,
		},
		StopConditions: []StopCondition{
			{
				ID:            "emergency-stop",
				Description:   "Emergency stop is active.",
				FactKey:       "emergency_stop",
				Operator:      StopEquals,
				ExpectedValue: "true",
				Required:      true,
				Effect:        StopDeny,
			},
			{
				ID:            "cost-limit",
				Description:   "Estimated cost meets or exceeds the approval threshold.",
				FactKey:       "cost_eur",
				Operator:      StopGreaterOrEqual,
				ExpectedValue: "10",
				Effect:        StopRequireApproval,
			},
		},
		SourceReferences: []string{"policy:local-safe-work-v1"},
		CreatedBy:        "robert",
		ExpiresAt:        &expires,
	}
}

func validAction(now time.Time) ActionRequest {
	return ActionRequest{
		OwnerIdentity:     "robert",
		ActorIdentity:     "worker:local-task",
		Action:            "task.update",
		ResourceType:      "task",
		ResourceID:        "task-1",
		ProjectKey:        "hai",
		ToolID:            "local-task-worker",
		Risk:              RiskLow,
		RequestedAutonomy: 4,
		Facts: map[string]string{
			"emergency_stop": "false",
			"cost_eur":       "0",
		},
		SourceReferences: []string{"task:task-1"},
		RequestedAt:      now,
	}
}
