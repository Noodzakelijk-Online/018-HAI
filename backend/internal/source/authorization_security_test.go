package source

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
	"gorm.io/gorm"
)

func TestRevokeRejectsCrossOwnerBeforeAuthorizationConsumption(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "alice"))
	authorizer := &recordingSourceAuthorizer{}
	service := configuredSourceEffectService(NewService(repo, nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	_, err := service.RevokeAuthorized(
		context.Background(),
		sourceID,
		testSourceAuthorization("mallory"),
	)
	if !errors.Is(err, ErrDestructiveOwnerMismatch) {
		t.Fatalf("Revoke error = %v, want ErrDestructiveOwnerMismatch", err)
	}
	if authorizer.calls != 0 {
		t.Fatalf("authorization calls = %d, want 0", authorizer.calls)
	}
	if source, _ := repo.FindSource(sourceID); source.Status != "active" {
		t.Fatalf("cross-owner revoke changed source: %#v", source)
	}
}

func TestDeleteExtractionRejectsCrossOwnerBeforeAuthorizationConsumption(
	t *testing.T,
) {
	sourceID := uuid.New()
	extractionID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "alice"))
	if _, err := repo.SaveExtraction(&models.SourceExtraction{
		ID: extractionID, SourceID: sourceID, RawItemID: uuid.New(),
		ProjectKey: "project-1", ContentHash: strings.Repeat("b", 64),
	}); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	authorizer := &recordingSourceAuthorizer{}
	service := configuredSourceEffectService(
		NewService(repo, nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	err := service.DeleteExtractionAuthorized(
		context.Background(),
		extractionID,
		testSourceAuthorization("mallory"),
	)
	if !errors.Is(err, ErrDestructiveOwnerMismatch) {
		t.Fatalf(
			"DeleteExtraction error = %v, want owner mismatch",
			err,
		)
	}
	if authorizer.calls != 0 {
		t.Fatalf("authorization calls = %d, want 0", authorizer.calls)
	}
	if _, err := repo.FindExtraction(extractionID); err != nil {
		t.Fatalf("cross-owner deletion changed extraction: %v", err)
	}
}

func TestDestructiveEffectsFailClosedWithoutAuthorizer(t *testing.T) {
	sourceID := uuid.New()
	extractionID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "robert"))
	if _, err := repo.SaveExtraction(&models.SourceExtraction{
		ID: extractionID, SourceID: sourceID, RawItemID: uuid.New(),
		ProjectKey: "project-1", ContentHash: strings.Repeat("b", 64),
	}); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	service := configuredSourceEffectService(
		NewService(repo, nil),
		nil,
		clearSourceEmergencyStop,
	)

	if _, err := service.RevokeAuthorized(
		context.Background(),
		sourceID,
		testSourceAuthorization("robert"),
	); !errors.Is(err, ErrDestructiveAuthorizationRequired) {
		t.Fatalf("Revoke error = %v, want authorization required", err)
	}
	if err := service.DeleteExtractionAuthorized(
		context.Background(),
		extractionID,
		testSourceAuthorization("robert"),
	); !errors.Is(err, ErrDestructiveAuthorizationRequired) {
		t.Fatalf("DeleteExtraction error = %v, want authorization required", err)
	}
}

func TestInvalidDestructiveTargetDoesNotConsumeAuthorization(t *testing.T) {
	authorizer := &recordingSourceAuthorizer{}
	service := configuredSourceEffectService(
		NewService(newFakeSourceRepo(), nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	if _, err := service.RevokeAuthorized(
		context.Background(),
		uuid.New(),
		testSourceAuthorization("robert"),
	); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Revoke error = %v, want not found", err)
	}
	if err := service.DeleteExtractionAuthorized(
		context.Background(),
		uuid.New(),
		testSourceAuthorization("robert"),
	); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteExtraction error = %v, want not found", err)
	}
	if authorizer.calls != 0 {
		t.Fatalf("authorization calls = %d, want 0", authorizer.calls)
	}
}

func TestRevokeConsumesExactAuthorizationOnceAtMutationBoundary(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "robert"))
	if err := repo.SaveOAuthToken(&models.SourceOAuthToken{
		ID: uuid.New(), SourceID: sourceID, Provider: "google",
		AccessToken: []byte("encrypted"),
	}); err != nil {
		t.Fatalf("SaveOAuthToken: %v", err)
	}
	authorizer := &recordingSourceAuthorizer{}
	service := configuredSourceEffectService(NewService(repo, nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	updated, err := service.RevokeAuthorized(
		context.Background(),
		sourceID,
		testSourceAuthorization("robert"),
	)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorization calls = %d, want 1", authorizer.calls)
	}
	request := authorizer.request
	if request.OwnerIdentity != "robert" ||
		request.ActorIdentity != "robert" ||
		request.Action != revokeSourceAction ||
		request.Stage != executionauth.StageDeletion ||
		request.ResourceType != connectedSourceResourceType ||
		request.ResourceID != sourceID.String() ||
		request.ProjectKey != "project-1" ||
		request.ApprovalSourceID != "approval-1" ||
		request.ApprovalBindingDigest != strings.Repeat("a", 64) ||
		len(request.EffectDigest) != 64 {
		t.Fatalf("authorization request is not exact: %#v", request)
	}
	if authorizer.consumer != sourceAuthorizationConsumer ||
		authorizer.target != "source-effect:"+request.EffectDigest {
		t.Fatalf(
			"consumer/target = %q / %q",
			authorizer.consumer,
			authorizer.target,
		)
	}
	if updated.Enabled || updated.Status != "revoked" || updated.RevokedAt == nil {
		t.Fatalf("source was not revoked: %#v", updated)
	}
	if _, err := repo.FindOAuthToken(sourceID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("revoked source retained OAuth credentials: %v", err)
	}
}

func TestMismatchedAuthorizationReceiptBlocksRevoke(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "robert"))
	authorizer := &recordingSourceAuthorizer{
		mutateReceipt: func(receipt *executionauth.Receipt) {
			receipt.ResourceID = uuid.NewString()
		},
	}
	service := configuredSourceEffectService(NewService(repo, nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	_, err := service.RevokeAuthorized(
		context.Background(),
		sourceID,
		testSourceAuthorization("robert"),
	)
	if !errors.Is(err, ErrDestructiveAuthorizationMismatch) {
		t.Fatalf("Revoke error = %v, want receipt mismatch", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorization calls = %d, want 1", authorizer.calls)
	}
	if source, _ := repo.FindSource(sourceID); source.Status != "active" {
		t.Fatalf("mismatched receipt changed source: %#v", source)
	}
}

func TestPostAuthorizationEmergencyStopBlocksDeletion(t *testing.T) {
	sourceID := uuid.New()
	extractionID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "robert"))
	if _, err := repo.SaveExtraction(&models.SourceExtraction{
		ID: extractionID, SourceID: sourceID, RawItemID: uuid.New(),
		ProjectKey: "project-1", ContentHash: strings.Repeat("c", 64),
	}); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	authorizer := &recordingSourceAuthorizer{}
	stopChecks := 0
	service := configuredSourceEffectService(NewService(repo, nil),
		authorizer,
		func() safety.EmergencyStopDecision {
			stopChecks++
			return safety.EmergencyStopDecision{
				Active: stopChecks >= 2,
				Reason: "operator stop",
				Source: "test",
			}
		},
	)

	err := service.DeleteExtractionAuthorized(
		context.Background(),
		extractionID,
		testSourceAuthorization("robert"),
	)
	if !errors.Is(err, ErrSourceEmergencyStopActive) {
		t.Fatalf("DeleteExtraction error = %v, want emergency stop", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorization calls = %d, want 1", authorizer.calls)
	}
	if _, err := repo.FindExtraction(extractionID); err != nil {
		t.Fatalf("post-authorization stop deleted extraction: %v", err)
	}
}

func TestPostAuthorizationSnapshotChangeRollsBackDeletion(t *testing.T) {
	sourceID := uuid.New()
	extractionID := uuid.New()
	repo := newFakeSourceRepo(testOwnedSource(sourceID, "robert"))
	if _, err := repo.SaveExtraction(&models.SourceExtraction{
		ID: extractionID, SourceID: sourceID, RawItemID: uuid.New(),
		ProjectKey: "project-1", ContentHash: strings.Repeat("c", 64),
	}); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	authorizer := &recordingSourceAuthorizer{
		afterAuthorize: func(executionauth.Request) {
			repo.extractions[extractionID].ProjectKey = "changed-project"
		},
	}
	service := configuredSourceEffectService(
		NewService(repo, nil),
		authorizer,
		clearSourceEmergencyStop,
	)

	err := service.DeleteExtractionAuthorized(
		context.Background(),
		extractionID,
		testSourceAuthorization("robert"),
	)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteExtraction error = %v, want snapshot mismatch", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorization calls = %d, want 1", authorizer.calls)
	}
	if _, err := repo.FindExtraction(extractionID); err != nil {
		t.Fatalf("snapshot mismatch deleted extraction: %v", err)
	}
}

func testOwnedSource(id uuid.UUID, owner string) *models.ConnectedSource {
	return &models.ConnectedSource{
		ID: id, OwnerIdentity: owner, ConnectorKey: "email",
		Name: "Project mailbox", Category: "email", Enabled: true,
		LocalOnly: true, Status: "active", DefaultProjectKey: "project-1",
	}
}

func testSourceAuthorization(owner string) DestructiveEffectAuthorization {
	return DestructiveEffectAuthorization{
		OwnerIdentity: owner, ActorIdentity: owner,
		IdempotencyKey: "source-effect-1", TaskID: "task-1",
		ApprovalSourceID:      "approval-1",
		ApprovalBindingDigest: strings.Repeat("a", 64),
	}
}

func configuredSourceEffectService(
	base Service,
	authorizer FinalEffectAuthorizer,
	stop func() safety.EmergencyStopDecision,
) *service {
	configured, ok := base.(*service)
	if !ok {
		panic("source service does not expose controlled final effects")
	}
	configured.WithDestructiveEffectAuthorization(authorizer, stop)
	return configured
}

func authorizedSourceEffectService(base Service) *service {
	return configuredSourceEffectService(
		base,
		&recordingSourceAuthorizer{},
		clearSourceEmergencyStop,
	)
}

func clearSourceEmergencyStop() safety.EmergencyStopDecision {
	return safety.EmergencyStopDecision{Source: "test"}
}

type recordingSourceAuthorizer struct {
	calls          int
	request        executionauth.Request
	consumer       string
	target         string
	err            error
	mutateReceipt  func(*executionauth.Receipt)
	afterAuthorize func(executionauth.Request)
}

func (a *recordingSourceAuthorizer) AuthorizeAndConsume(
	_ context.Context,
	request executionauth.Request,
	consumer string,
	target string,
) (executionauth.Receipt, error) {
	a.calls++
	a.request = request
	a.consumer = consumer
	a.target = target
	if a.err != nil {
		return executionauth.Receipt{}, a.err
	}
	if a.afterAuthorize != nil {
		a.afterAuthorize(request)
	}
	receipt := executionauth.Receipt{
		ID:                uuid.New(),
		ContractVersion:   executionauth.ContractVersion,
		OwnerIdentity:     request.OwnerIdentity,
		IdempotencyKey:    request.IdempotencyKey,
		ActorIdentity:     request.ActorIdentity,
		ActorKind:         request.ActorKind,
		TaskID:            request.TaskID,
		Action:            request.Action,
		Stage:             request.Stage,
		ResourceType:      request.ResourceType,
		ResourceID:        request.ResourceID,
		ProjectKey:        request.ProjectKey,
		ApprovalSourceID:  request.ApprovalSourceID,
		EffectDigest:      request.EffectDigest,
		Outcome:           executionauth.OutcomeAuthorized,
		RequestDigest:     strings.Repeat("d", 64),
		DecisionDigest:    strings.Repeat("e", 64),
		RequiredAuthority: request.RequiredAuthority,
		RequestedAutonomy: request.RequestedAutonomy,
		Risk:              request.Risk,
		Reversible:        request.Reversible,
		Evidence: executionauth.DecisionEvidence{
			Approval: executionauth.ApprovalEvidence{
				SourceID:       request.ApprovalSourceID,
				DecisionID:     "approval-decision-1",
				DecisionDigest: strings.Repeat("f", 64),
				ApprovedBy:     request.OwnerIdentity,
				ApprovedAt:     time.Now().UTC().Add(-time.Minute),
				ExpiresAt:      time.Now().UTC().Add(time.Minute),
			},
		},
	}
	if a.mutateReceipt != nil {
		a.mutateReceipt(&receipt)
	}
	return receipt, nil
}
