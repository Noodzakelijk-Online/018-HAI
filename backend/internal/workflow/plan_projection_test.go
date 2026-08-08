package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/plangraph"
)

type recordingWorkflowCoordinationProjector struct {
	service *plangraph.Service
	calls   int
	owner   string
	request plangraph.PreviewRequest
	err     error
}

func (projector *recordingWorkflowCoordinationProjector) Preview(ctx context.Context, owner string, request plangraph.PreviewRequest) (*plangraph.Plan, error) {
	projector.calls++
	projector.owner = owner
	projector.request = request
	if projector.err != nil {
		return nil, projector.err
	}
	return projector.service.Preview(ctx, owner, request)
}

func TestWorkflowIntakeProjectsImmutableAdvisoryCoordinationDraft(t *testing.T) {
	projector := &recordingWorkflowCoordinationProjector{
		service: plangraph.NewService(plangraph.NewMemoryRepository(), nil),
	}
	configured, err := WithCoordinationPlanProjector(NewService(newFakeWorkflowRepo()), projector)
	if err != nil {
		t.Fatalf("configure projector: %v", err)
	}
	record, err := configured.Intake(IntakeRequest{
		OwnerIdentity: "owner-a",
		Input:         "Create a low-risk administrative checklist and verify completion.",
		ProjectKey:    "project-a",
		SourceType:    "manual",
		SourceID:      "workflow-projection-1",
	})
	if err != nil {
		t.Fatalf("workflow intake: %v", err)
	}
	if projector.calls != 1 || projector.owner != "owner-a" {
		t.Fatalf("unexpected projection calls=%d owner=%q", projector.calls, projector.owner)
	}
	item := record.Item
	if item.CoordinationPlanID != nil || item.CoordinationDraftPlanID == nil {
		t.Fatalf("expected only a draft coordination binding: %+v", item)
	}
	if item.CoordinationDraftRevision != 1 || item.CoordinationDraftDigest == "" || item.CoordinationDraftNodeID != "workflow" {
		t.Fatalf("draft binding is incomplete: %+v", item)
	}
	plans, err := projector.service.List(context.Background(), "owner-a")
	if err != nil || len(plans) != 1 {
		t.Fatalf("list projected plans: count=%d err=%v", len(plans), err)
	}
	draft := plans[0]
	if draft.Status != plangraph.StatusDraft || draft.Revision != 1 || draft.CanExecute {
		t.Fatalf("draft violated advisory contract: %+v", draft)
	}
	if len(draft.Nodes) != len(record.Checklist)+1 || len(draft.Edges) != len(record.Checklist) {
		t.Fatalf("workflow graph did not preserve checklist: nodes=%d edges=%d checklist=%d", len(draft.Nodes), len(draft.Edges), len(record.Checklist))
	}
	for _, node := range draft.Nodes {
		if node.Owner != "owner-a" || node.Bindings.WorkflowID != item.ID.String() {
			t.Fatalf("node is not exactly owner/workflow bound: %+v", node)
		}
		if node.EstimatedMinutes != 0 || node.EstimatedCostEUR != 0 {
			t.Fatalf("projection invented unsupported estimates: %+v", node)
		}
	}
	previous := "workflow"
	for index, edge := range draft.Edges {
		if edge.From != previous || edge.Type != "finish_to_start" {
			t.Fatalf("dependency %d is not a deterministic chain: %+v", index, edge)
		}
		previous = edge.To
	}
	if projector.request.IdempotencyKey != "workflow-plan-graph-"+item.ID.String() {
		t.Fatalf("draft idempotency is not workflow-bound: %+v", projector.request)
	}
	if strings.Contains(strings.ToLower(projector.request.Title), "execute") && draft.CanExecute {
		t.Fatal("workflow text broadened advisory authority")
	}
}

func TestAcceptedWorkflowCoordinationPlanSuppressesCompetingDraft(t *testing.T) {
	reference := workflowPlanReference(strings.Repeat("b", 64))
	resolver := &workflowAcceptedPlanResolverStub{binding: &plangraph.AcceptedRevisionBinding{
		PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
		NodeID: reference.NodeID, Node: plangraph.Node{ID: reference.NodeID}, CanExecute: false,
	}}
	projector := &recordingWorkflowCoordinationProjector{
		service: plangraph.NewService(plangraph.NewMemoryRepository(), nil),
	}
	configured, err := WithAcceptedPlanResolver(NewService(newFakeWorkflowRepo()), resolver)
	if err != nil {
		t.Fatal(err)
	}
	configured, err = WithCoordinationPlanProjector(configured, projector)
	if err != nil {
		t.Fatal(err)
	}
	record, err := configured.Intake(IntakeRequest{
		OwnerIdentity:    "owner-a",
		Input:            "Prepare an administrative checklist.",
		CoordinationPlan: reference,
	})
	if err != nil {
		t.Fatalf("workflow intake: %v", err)
	}
	if projector.calls != 0 || record.Item.CoordinationDraftPlanID != nil || record.Item.CoordinationPlanID == nil {
		t.Fatalf("accepted provenance should suppress a competing draft: calls=%d item=%+v", projector.calls, record.Item)
	}
}

func TestWorkflowCoordinationProjectionFailureBlocksAndReplayRecovers(t *testing.T) {
	projector := &recordingWorkflowCoordinationProjector{
		service: plangraph.NewService(plangraph.NewMemoryRepository(), nil),
		err:     errors.New("plan storage unavailable"),
	}
	configured, err := WithCoordinationPlanProjector(NewService(newFakeWorkflowRepo()), projector)
	if err != nil {
		t.Fatal(err)
	}
	request := IntakeRequest{
		OwnerIdentity: "owner-a",
		Input:         "Create a low-risk administrative checklist.",
		SourceType:    "manual",
		SourceID:      "workflow-projection-recovery",
	}
	first, err := configured.Intake(request)
	if err != nil {
		t.Fatalf("retain intake during projection outage: %v", err)
	}
	if first.Item.CurrentState != StateBlocked || !strings.HasPrefix(first.Item.BlockedReason, coordinationProjectionFailurePrefix) {
		t.Fatalf("projection outage did not block execution: %+v", first.Item)
	}
	if first.Item.CoordinationDraftPlanID != nil {
		t.Fatal("failed projection recorded a draft binding")
	}

	projector.err = nil
	second, err := configured.Intake(request)
	if err != nil {
		t.Fatalf("recover idempotent workflow projection: %v", err)
	}
	if second.Item.ID != first.Item.ID || second.Item.CoordinationDraftPlanID == nil {
		t.Fatalf("replay did not recover the same workflow: first=%+v second=%+v", first.Item, second.Item)
	}
	if second.Item.CurrentState != StateReady || second.Item.BlockedReason != "" {
		t.Fatalf("successful recovery did not restore safe runnable state: %+v", second.Item)
	}
	if projector.calls != 2 {
		t.Fatalf("unexpected projection attempts: %d", projector.calls)
	}
}
