package executionauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

type authorizationProjectionFunc func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)

func (function authorizationProjectionFunc) ProjectOperationalRecord(ctx context.Context, request lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
	return function(ctx, request)
}

func TestAuthorizationProjectsApprovalCommitmentAndCostIntoOwnerGraph(t *testing.T) {
	repository := NewMemoryRepository()
	binding := strings.Repeat("b", 64)
	approval := ResolvedApproval{
		SourceID: "task-review:review-1", DecisionID: "decision-1",
		DecisionDigest: strings.Repeat("d", 64), BindingDigest: binding,
		ApprovedBy: "alice", ApprovedAt: fixedNow().Add(-time.Minute),
		ExpiresAt: fixedNow().Add(time.Hour),
	}
	service := newTestService(t, repository, permissiveConstitution(), fakeApprovalResolver{
		values: map[string]ResolvedApproval{"alice\x00" + approval.SourceID: approval},
	}, nil)
	graph := lifeontology.NewService(nil, fixedNow)
	if _, err := service.WithLifeOntologyProjection(graph); err != nil {
		t.Fatal(err)
	}

	request := baseRequest("project-authorized-commitment")
	request.Stage = StageCommitment
	request.Action = "legal.commitment.accept"
	request.ResourceType = "legal-agreement"
	request.ProjectKey = "vivare-case"
	request.Domain = string(lifeontology.DomainLegalGovernment)
	request.RequestedAutonomy = 6
	request.ApprovalSourceID = approval.SourceID
	request.ApprovalBindingDigest = binding
	request.EstimatedCostEUR = 12.50

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil || receipt.Outcome != OutcomeAuthorized {
		t.Fatalf("Authorize() = outcome %q err %v", receipt.Outcome, err)
	}
	if receipt.LifeGraphProjection == nil || receipt.LifeGraphProjectionWarning != "" {
		t.Fatalf("projection = %#v warning=%q", receipt.LifeGraphProjection, receipt.LifeGraphProjectionWarning)
	}
	if !receipt.LifeGraphProjection.AdvisoryOnly || receipt.LifeGraphProjection.CanExecute || receipt.LifeGraphProjection.GrantsAuthority {
		t.Fatalf("projection crossed authority boundary: %#v", receipt.LifeGraphProjection)
	}
	if receipt.LifeGraphProjection.Primary.Domain != lifeontology.DomainLegalGovernment ||
		receipt.LifeGraphProjection.Primary.VerificationStatus != lifeontology.VerificationHumanApproved {
		t.Fatalf("primary projection = %#v", receipt.LifeGraphProjection.Primary)
	}
	expectedDigest, err := finishDigest(receipt)
	if err != nil || expectedDigest != receipt.DecisionDigest {
		t.Fatalf("response metadata changed receipt digest: expected=%q stored=%q err=%v", expectedDigest, receipt.DecisionDigest, err)
	}

	entities, err := graph.QueryEntities(context.Background(), "alice", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[lifeontology.EntityType]int{}
	for _, entity := range entities {
		counts[entity.Type]++
	}
	if len(entities) != 6 || counts[lifeontology.EntityOutcome] != 2 ||
		counts[lifeontology.EntityCommitment] != 1 || counts[lifeontology.EntityCost] != 1 ||
		counts[lifeontology.EntityTask] != 1 || counts[lifeontology.EntityProject] != 1 {
		t.Fatalf("projected entities = %d counts=%#v", len(entities), counts)
	}
	relations, err := graph.QueryRelations(context.Background(), "alice", lifeontology.RelationQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil || len(relations) != 5 {
		t.Fatalf("projected relations = %d err=%v", len(relations), err)
	}

	listed, err := service.List(context.Background(), "alice", 10)
	if err != nil || len(listed) != 1 || listed[0].LifeGraphProjection == nil || !listed[0].LifeGraphProjection.AlreadyExisted {
		t.Fatalf("idempotent list projection = %#v err=%v", listed, err)
	}
	entitiesAfter, _ := graph.QueryEntities(context.Background(), "alice", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if len(entitiesAfter) != len(entities) {
		t.Fatalf("idempotent projection duplicated entities: before=%d after=%d", len(entities), len(entitiesAfter))
	}
	other, err := graph.QueryEntities(context.Background(), "bob", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-owner graph leak = %#v err=%v", other, err)
	}
}

func TestAuthorizationProjectionFailureCannotRollbackReceiptOrLeakSecrets(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	if _, err := service.WithLifeOntologyProjection(authorizationProjectionFunc(func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
		return lifeontology.OperationalProjectionResult{}, errors.New("projection unavailable api_key=do-not-expose")
	})); err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Authorize(context.Background(), baseRequest("projection-warning"))
	if err != nil || receipt.DecisionDigest == "" || receipt.LifeGraphProjection != nil {
		t.Fatalf("durable receipt = %#v err=%v", receipt, err)
	}
	if receipt.LifeGraphProjectionWarning == "" || strings.Contains(receipt.LifeGraphProjectionWarning, "do-not-expose") {
		t.Fatalf("unsafe projection warning = %q", receipt.LifeGraphProjectionWarning)
	}
	stored, err := service.repository.Get(context.Background(), "alice", receipt.ID)
	if err != nil || stored.DecisionDigest != receipt.DecisionDigest || stored.LifeGraphProjectionWarning != "" {
		t.Fatalf("authoritative receipt was changed by projection failure: %#v err=%v", stored, err)
	}
}

func TestAuthorizationRejectsAuthorityBearingGraphResponse(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	if _, err := service.WithLifeOntologyProjection(authorizationProjectionFunc(func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
		return lifeontology.OperationalProjectionResult{AdvisoryOnly: false, CanExecute: true, GrantsAuthority: true}, nil
	})); err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Authorize(context.Background(), baseRequest("authority-boundary"))
	if err != nil || receipt.DecisionDigest == "" {
		t.Fatalf("Authorize() receipt=%#v err=%v", receipt, err)
	}
	if receipt.LifeGraphProjection != nil || !strings.Contains(receipt.LifeGraphProjectionWarning, "authority boundary") {
		t.Fatalf("authority-bearing projection accepted: %#v warning=%q", receipt.LifeGraphProjection, receipt.LifeGraphProjectionWarning)
	}
}
