package workflowgraph

import (
	"fmt"
	"sort"
	"time"
)

type CompensationStep struct {
	Order              int    `json:"order"`
	CompletedNodeID    string `json:"completedNodeId"`
	CompensationNodeID string `json:"compensationNodeId"`
}

type CancellationPlan struct {
	RunID                string             `json:"runId"`
	CancelActiveNodeIDs  []string           `json:"cancelActiveNodeIds"`
	CompensationSteps    []CompensationStep `json:"compensationSteps"`
	RequiresCompensation bool               `json:"requiresCompensation"`
	TargetStatus         RunStatus          `json:"targetStatus"`
	Reason               string             `json:"reason"`
}

// PlanCancellation produces a saga-style reverse completion plan. It does not
// cancel work, execute compensators, or mutate the run.
func PlanCancellation(definition Definition, run Run, reason string) (CancellationPlan, error) {
	return PlanCompensation(definition, run, RunCancelled, reason)
}

// PlanCompensation supports cancellation and failure recovery. Successfully
// completed action nodes are compensated newest-first; a compensator already
// recorded as succeeded or compensated is not scheduled again.
func PlanCompensation(
	definition Definition,
	run Run,
	targetStatus RunStatus,
	reason string,
) (CancellationPlan, error) {
	if err := ValidateDefinition(definition); err != nil {
		return CancellationPlan{}, err
	}
	if run.SchemaVersion != CurrentRunSchemaVersion {
		return CancellationPlan{}, validationProblem("run schema version %d is unsupported", run.SchemaVersion)
	}
	if run.DefinitionID != definition.ID || run.DefinitionVersion != definition.Version {
		return CancellationPlan{}, validationProblem(
			"run definition %s@%d does not match %s@%d",
			run.DefinitionID,
			run.DefinitionVersion,
			definition.ID,
			definition.Version,
		)
	}
	if targetStatus != RunCancelled && targetStatus != RunFailed {
		return CancellationPlan{}, validationProblem("compensation target status %q is unsupported", targetStatus)
	}

	nodes, _, _ := indexDefinition(definition)
	type candidate struct {
		node        Node
		completedAt time.Time
	}
	var candidates []candidate
	for nodeID, state := range run.NodeStates {
		node, exists := nodes[nodeID]
		if !exists || node.Type != NodeAction || node.CompensationNodeID == "" {
			continue
		}
		if state.Status != NodeSucceeded || state.CompletedAt == nil {
			continue
		}
		compensationState := run.NodeStates[node.CompensationNodeID].Status
		if compensationState == NodeSucceeded || compensationState == NodeCompensated {
			continue
		}
		candidates = append(candidates, candidate{node: node, completedAt: *state.CompletedAt})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].completedAt.Equal(candidates[j].completedAt) {
			return candidates[i].completedAt.After(candidates[j].completedAt)
		}
		return candidates[i].node.ID < candidates[j].node.ID
	})

	steps := make([]CompensationStep, 0, len(candidates))
	for index, candidate := range candidates {
		steps = append(steps, CompensationStep{
			Order:              index + 1,
			CompletedNodeID:    candidate.node.ID,
			CompensationNodeID: candidate.node.CompensationNodeID,
		})
	}

	active := append([]string(nil), run.ActiveNodeIDs...)
	sort.Strings(active)
	if reason == "" {
		reason = fmt.Sprintf("run requested transition to %s", targetStatus)
	}
	return CancellationPlan{
		RunID:                run.ID,
		CancelActiveNodeIDs:  active,
		CompensationSteps:    steps,
		RequiresCompensation: len(steps) > 0,
		TargetStatus:         targetStatus,
		Reason:               reason,
	}, nil
}
