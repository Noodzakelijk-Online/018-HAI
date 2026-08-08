package dataexport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

type authorizerFunc func(
	context.Context,
	executionauth.Request,
	string,
	string,
) (executionauth.Receipt, error)

func (f authorizerFunc) AuthorizeAndConsume(
	ctx context.Context,
	request executionauth.Request,
	consumer string,
	target string,
) (executionauth.Receipt, error) {
	return f(ctx, request, consumer, target)
}

func testMemories(owner string) []models.ContextMemory {
	return []models.ContextMemory{
		{ID: uuid.New(), OwnerIdentity: owner, Content: "first"},
		{ID: uuid.New(), OwnerIdentity: owner, Content: "second"},
	}
}

func testAuthorization(
	t *testing.T,
	owner string,
	memories []models.ContextMemory,
) AuthorizationRequest {
	t.Helper()
	digest, err := MemoryExportEffectDigest(owner, memories)
	if err != nil {
		t.Fatalf("effect digest: %v", err)
	}
	return AuthorizationRequest{
		OwnerIdentity:         owner,
		ActorIdentity:         "operator@example.test",
		IdempotencyKey:        "export-once",
		TaskID:                "task-export",
		ApprovalSourceID:      "approval-1",
		ApprovalBindingDigest: digest,
		ProjectKey:            "private-project",
	}
}

func authorizedReceipt(request executionauth.Request, now time.Time) executionauth.Receipt {
	return executionauth.Receipt{
		ID:               uuid.New(),
		OwnerIdentity:    request.OwnerIdentity,
		ActorIdentity:    request.ActorIdentity,
		ActorKind:        request.ActorKind,
		TaskID:           request.TaskID,
		Action:           request.Action,
		Stage:            request.Stage,
		ResourceType:     request.ResourceType,
		ResourceID:       request.ResourceID,
		ApprovalSourceID: request.ApprovalSourceID,
		EffectDigest:     request.EffectDigest,
		Outcome:          executionauth.OutcomeAuthorized,
		DecisionDigest:   strings.Repeat("d", 64),
		EvaluatedAt:      now,
		Evidence: executionauth.DecisionEvidence{
			Approval: executionauth.ApprovalEvidence{
				SourceID:       request.ApprovalSourceID,
				DecisionID:     "decision-1",
				DecisionDigest: strings.Repeat("a", 64),
				ApprovedBy:     "robert@example.test",
				ApprovedAt:     now.Add(-time.Minute),
				ExpiresAt:      now.Add(time.Minute),
			},
		},
	}
}

func clearEmergencyStop(t *testing.T) {
	t.Helper()
	restore := safety.SetEmergencyStopProvider(
		safety.EmergencyStopProviderFunc(func() (bool, string, error) {
			return false, "", nil
		}),
	)
	t.Cleanup(restore)
}

func TestBuildMemoryExportConsumesExactOwnerBoundAuthorization(t *testing.T) {
	clearEmergencyStop(t)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	owner := "owner@example.test"
	memories := testMemories(owner)
	auth := testAuthorization(t, owner, memories)
	calls := 0
	service := NewService(authorizerFunc(func(
		_ context.Context,
		request executionauth.Request,
		consumer string,
		target string,
	) (executionauth.Receipt, error) {
		calls++
		if request.OwnerIdentity != owner ||
			request.EffectDigest != auth.ApprovalBindingDigest ||
			request.Risk != executionauth.RiskHigh ||
			request.Stage != executionauth.StageDataAccess ||
			request.ActorKind != executionauth.ActorHuman ||
			request.ApprovalSourceID != auth.ApprovalSourceID {
			t.Fatalf("unexpected authorization request: %+v", request)
		}
		if consumer != memoryExportConsumer ||
			target != memoryExportAction+":"+request.EffectDigest {
			t.Fatalf("unexpected consumption boundary: %q %q", consumer, target)
		}
		return authorizedReceipt(request, now), nil
	}), func() time.Time { return now })

	result, err := service.BuildMemoryExport(context.Background(), auth, memories)
	if err != nil {
		t.Fatalf("build export: %v", err)
	}
	if calls != 1 || result.Data.Count != len(memories) {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
	if result.Evidence.EffectDigest != auth.ApprovalBindingDigest ||
		!result.Evidence.OwnerBound || !result.Evidence.EmergencyStopOK {
		t.Fatalf("evidence does not prove exact boundary: %+v", result.Evidence)
	}
}

func TestBuildMemoryExportFailsClosedWithoutAuthorizer(t *testing.T) {
	owner := "owner@example.test"
	memories := testMemories(owner)
	result, err := NewService(nil, time.Now).BuildMemoryExport(
		context.Background(),
		testAuthorization(t, owner, memories),
		memories,
	)
	if !errors.Is(err, ErrAuthorizationUnavailable) || result.Data.Count != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestBuildMemoryExportRejectsCrossOwnerRecordsBeforeAuthorization(t *testing.T) {
	called := false
	service := NewService(authorizerFunc(func(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error) {
		called = true
		return executionauth.Receipt{}, nil
	}), time.Now)
	memories := testMemories("other@example.test")
	_, err := service.BuildMemoryExport(context.Background(), AuthorizationRequest{
		OwnerIdentity:         "owner@example.test",
		ActorIdentity:         "operator@example.test",
		IdempotencyKey:        "one",
		TaskID:                "task",
		ApprovalSourceID:      "approval",
		ApprovalBindingDigest: strings.Repeat("a", 64),
	}, memories)
	if !errors.Is(err, ErrInvalidRequest) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestBuildMemoryExportRejectsChangedSnapshotBeforeAuthorization(t *testing.T) {
	owner := "owner@example.test"
	memories := testMemories(owner)
	auth := testAuthorization(t, owner, memories)
	memories[0].Content = "changed after approval"
	called := false
	service := NewService(authorizerFunc(func(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error) {
		called = true
		return executionauth.Receipt{}, nil
	}), time.Now)
	_, err := service.BuildMemoryExport(context.Background(), auth, memories)
	if !errors.Is(err, ErrAuthorizationMismatch) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestBuildMemoryExportRechecksEmergencyStopAfterConsumption(t *testing.T) {
	active := false
	restore := safety.SetEmergencyStopProvider(
		safety.EmergencyStopProviderFunc(func() (bool, string, error) {
			return active, "operator stop", nil
		}),
	)
	t.Cleanup(restore)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	owner := "owner@example.test"
	memories := testMemories(owner)
	auth := testAuthorization(t, owner, memories)
	service := NewService(authorizerFunc(func(
		_ context.Context,
		request executionauth.Request,
		_ string,
		_ string,
	) (executionauth.Receipt, error) {
		active = true
		return authorizedReceipt(request, now), nil
	}), func() time.Time { return now })

	result, err := service.BuildMemoryExport(context.Background(), auth, memories)
	if !errors.Is(err, ErrEmergencyStopActive) || result.Data.Count != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestBuildMemoryExportDoesNotLeakAuthorizationError(t *testing.T) {
	owner := "owner@example.test"
	memories := testMemories(owner)
	service := NewService(authorizerFunc(func(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error) {
		return executionauth.Receipt{}, errors.New("Bearer secret-access-token")
	}), time.Now)
	_, err := service.BuildMemoryExport(
		context.Background(),
		testAuthorization(t, owner, memories),
		memories,
	)
	if !errors.Is(err, ErrAuthorizationDenied) ||
		strings.Contains(err.Error(), "secret-access-token") {
		t.Fatalf("unsafe error: %v", err)
	}
}
