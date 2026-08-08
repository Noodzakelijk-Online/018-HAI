package workflow

import (
	"context"
	"fmt"
	"strings"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"

	"github.com/google/uuid"
)

const coordinationProjectionFailurePrefix = "coordination draft unavailable: "

// CoordinationPlanProjector creates advisory plan revisions. It cannot accept
// a revision or grant workflow, tool, runtime, or provider authority.
type CoordinationPlanProjector interface {
	Preview(context.Context, string, plangraph.PreviewRequest) (*plangraph.Plan, error)
}

func WithCoordinationPlanProjector(value Service, projector CoordinationPlanProjector) (Service, error) {
	service, ok := value.(*service)
	if !ok || service == nil {
		return nil, fmt.Errorf("workflow service does not support coordination plan projection")
	}
	if projector == nil {
		return nil, fmt.Errorf("coordination plan projector is required")
	}
	service.coordinationProjector = projector
	return service, nil
}

// ensureWorkflowCoordinationDraft is deliberately recovery-safe. The workflow
// receipt is authoritative; a projection outage cannot erase it, but a ready
// item is blocked until the immutable advisory draft can be stored and bound.
func (s *service) ensureWorkflowCoordinationDraft(item *models.WorkflowItem, actor string) {
	if s.coordinationProjector == nil || item == nil || item.ID == uuid.Nil ||
		item.CoordinationPlanID != nil || item.CoordinationDraftPlanID != nil {
		return
	}
	checklist, err := s.repo.FindChecklist(item.ID)
	if err == nil {
		var draft *plangraph.Plan
		draft, err = s.projectWorkflowCoordinationDraft(item, checklist)
		if err == nil {
			applyWorkflowCoordinationDraftBinding(item, draft)
			if strings.HasPrefix(strings.TrimSpace(item.BlockedReason), coordinationProjectionFailurePrefix) {
				item.BlockedReason = ""
				if item.RequiresApproval && item.ApprovalStatus != "approved" {
					item.CurrentState = StateNeedsApproval
				} else {
					item.CurrentState = StateReady
				}
			}
			if _, err = s.repo.UpdateItem(item); err == nil {
				s.audit(item.ID, "workflow.coordination_draft_projected", "", item.CurrentState,
					"immutable advisory workflow draft projected; explicit acceptance and separate effect authority remain required",
					"workflow_intake", "plan graph projection", "", firstNonEmpty(actor, "engine"))
				return
			}
		}
	}
	s.recordCoordinationProjectionFailure(item, err, actor)
}

func (s *service) projectWorkflowCoordinationDraft(item *models.WorkflowItem, checklist []models.WorkflowChecklistItem) (*plangraph.Plan, error) {
	owner := strings.TrimSpace(item.OwnerIdentity)
	if owner == "" {
		return nil, fmt.Errorf("workflow owner identity is required")
	}
	nodes, edges := workflowCoordinationGraph(*item, checklist)
	draft, err := s.coordinationProjector.Preview(context.Background(), owner, plangraph.PreviewRequest{
		IdempotencyKey: "workflow-plan-graph-" + item.ID.String(),
		Title:          compact(item.Title, 300),
		Nodes:          nodes,
		Edges:          edges,
		CreatedBy:      owner,
	})
	if err != nil {
		return nil, fmt.Errorf("project workflow coordination draft: %w", err)
	}
	if draft == nil || draft.Status != plangraph.StatusDraft || draft.Revision != 1 || draft.CanExecute {
		return nil, fmt.Errorf("project workflow coordination draft: projector violated the immutable advisory invariant")
	}
	if workflowCoordinationRoot(draft, item.ID.String()) == nil {
		return nil, fmt.Errorf("project workflow coordination draft: projector lost the workflow binding")
	}
	return draft, nil
}

func (s *service) recordCoordinationProjectionFailure(item *models.WorkflowItem, projectionErr error, actor string) {
	if item == nil {
		return
	}
	reason := "coordination draft projection failed"
	if projectionErr != nil {
		reason = compact(projectionErr.Error(), 420)
	}
	if item.CurrentState == StateReady || strings.HasPrefix(strings.TrimSpace(item.BlockedReason), coordinationProjectionFailurePrefix) {
		item.CurrentState = StateBlocked
		item.BlockedReason = coordinationProjectionFailurePrefix + reason
		_, _ = s.repo.UpdateItem(item)
	}
	s.audit(item.ID, "workflow.coordination_draft_failed", "", item.CurrentState,
		"workflow retained but cannot enter execution without its advisory coordination draft",
		"workflow_intake", reason, "", firstNonEmpty(actor, "engine"))
}

func applyWorkflowCoordinationDraftBinding(item *models.WorkflowItem, draft *plangraph.Plan) {
	if item == nil || draft == nil {
		return
	}
	root := workflowCoordinationRoot(draft, item.ID.String())
	if root == nil {
		return
	}
	planID := draft.ID
	item.CoordinationDraftPlanID = &planID
	item.CoordinationDraftRevision = draft.Revision
	item.CoordinationDraftDigest = draft.Digest
	item.CoordinationDraftNodeID = root.ID
}

func workflowCoordinationRoot(draft *plangraph.Plan, workflowID string) *plangraph.Node {
	if draft == nil {
		return nil
	}
	for index := range draft.Nodes {
		node := &draft.Nodes[index]
		if node.ID == "workflow" && node.Type == "workflow" && node.Bindings.WorkflowID == workflowID {
			return node
		}
	}
	return nil
}

func workflowCoordinationGraph(item models.WorkflowItem, checklist []models.WorkflowChecklistItem) ([]plangraph.Node, []plangraph.Edge) {
	bindings := plangraph.Bindings{WorkflowID: item.ID.String()}
	risk := workflowPlanRisk(item.RiskLevel)
	nodes := make([]plangraph.Node, 0, len(checklist)+1)
	nodes = append(nodes, plangraph.Node{
		ID:            "workflow",
		Type:          "workflow",
		Title:         compact(item.Title, 300),
		Owner:         strings.TrimSpace(item.OwnerIdentity),
		Status:        workflowPlanStatus(item.CurrentState),
		Deadline:      item.DueAt,
		Risk:          risk,
		ApprovalState: workflowPlanApproval(item.RequiresApproval, item.ApprovalStatus),
		Bindings:      bindings,
	})
	edges := make([]plangraph.Edge, 0, len(checklist))
	previous := "workflow"
	for index, step := range checklist {
		nodeID := fmt.Sprintf("checklist-%03d", index+1)
		nodes = append(nodes, plangraph.Node{
			ID:            nodeID,
			Type:          "workflow_step",
			Title:         compact(step.Label, 300),
			Owner:         strings.TrimSpace(item.OwnerIdentity),
			Status:        workflowChecklistPlanStatus(step),
			Deadline:      step.DueAt,
			Risk:          risk,
			ApprovalState: workflowPlanApproval(step.RequiresApproval, ""),
			Bindings:      bindings,
		})
		edges = append(edges, plangraph.Edge{
			ID:   fmt.Sprintf("dependency-%03d", index+1),
			From: previous,
			To:   nodeID,
			Type: "finish_to_start",
		})
		previous = nodeID
	}
	return nodes, edges
}

func workflowPlanRisk(value string) plangraph.Risk {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return plangraph.RiskLow
	case "high", "critical":
		return plangraph.RiskHigh
	default:
		return plangraph.RiskMedium
	}
}

func workflowPlanApproval(required bool, status string) plangraph.ApprovalState {
	if !required {
		return plangraph.ApprovalNotRequired
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return plangraph.ApprovalGranted
	case "rejected":
		return plangraph.ApprovalRejected
	default:
		return plangraph.ApprovalRequired
	}
}

func workflowPlanStatus(value string) plangraph.NodeStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StateReady:
		return plangraph.NodeReady
	case StateBlocked:
		return plangraph.NodeBlocked
	case StateWaitingInput:
		return plangraph.NodeWaiting
	case StateNeedsApproval:
		return plangraph.NodeNeedsApproval
	case StateCompleted, StateArchived:
		return plangraph.NodeCompleted
	case "failed":
		return plangraph.NodeFailed
	default:
		return plangraph.NodePlanned
	}
}

func workflowChecklistPlanStatus(step models.WorkflowChecklistItem) plangraph.NodeStatus {
	if step.RequiresApproval {
		return plangraph.NodeNeedsApproval
	}
	switch strings.ToLower(strings.TrimSpace(step.Status)) {
	case "completed", "done":
		return plangraph.NodeCompleted
	case "blocked":
		return plangraph.NodeBlocked
	case "waiting":
		return plangraph.NodeWaiting
	case "failed":
		return plangraph.NodeFailed
	case "ready":
		return plangraph.NodeReady
	default:
		return plangraph.NodePlanned
	}
}
