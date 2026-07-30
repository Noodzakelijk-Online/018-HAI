package workflowgraph

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// EvaluateNext returns a deterministic transition decision without mutating
// the run or executing side effects. The caller applies the decision and saves
// the run through RunRepository using optimistic concurrency.
func EvaluateNext(definition Definition, run Run, evaluation Evaluation) (Decision, error) {
	if err := ValidateDefinition(definition); err != nil {
		return Decision{}, err
	}
	if run.SchemaVersion != CurrentRunSchemaVersion {
		return Decision{}, validationProblem("run schema version %d is unsupported", run.SchemaVersion)
	}
	if run.DefinitionID != definition.ID || run.DefinitionVersion != definition.Version {
		return Decision{}, validationProblem(
			"run definition %s@%d does not match %s@%d",
			run.DefinitionID,
			run.DefinitionVersion,
			definition.ID,
			definition.Version,
		)
	}
	if run.Status.Terminal() {
		return Decision{}, validationProblem("run %q is already terminal with status %q", run.ID, run.Status)
	}
	if run.Steps >= definition.MaxRunSteps {
		return Decision{}, fmt.Errorf("%w: run has used %d of %d steps", ErrRunStepLimit, run.Steps, definition.MaxRunSteps)
	}
	if !contains(run.ActiveNodeIDs, evaluation.NodeID) {
		return Decision{}, fmt.Errorf("%w: %s", ErrNodeNotActive, evaluation.NodeID)
	}

	nodes, outgoing, incoming := indexDefinition(definition)
	node, exists := nodes[evaluation.NodeID]
	if !exists {
		return Decision{}, validationProblem("evaluation node %q does not exist", evaluation.NodeID)
	}

	switch node.Type {
	case NodeCondition:
		result := evaluateCondition(*node.Condition, evaluation.Variables)
		outcome := OutcomeFalse
		if result {
			outcome = OutcomeTrue
		}
		return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], outcome))
	case NodeHumanApproval:
		switch evaluation.Approval {
		case ApprovalPending:
			return waiting(node.ID, "human approval is still pending"), nil
		case ApprovalApproved:
			return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], OutcomeApproved))
		case ApprovalRejected:
			return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], OutcomeRejected))
		default:
			return Decision{}, validationProblem("approval decision %q is unsupported", evaluation.Approval)
		}
	case NodeWait:
		signal := strings.TrimSpace(evaluation.Signal)
		if signal == "" {
			return waiting(node.ID, "wait signal has not arrived"), nil
		}
		return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], signal))
	case NodeTimer:
		if evaluation.Now.IsZero() {
			return Decision{}, validationProblem("timer node %q requires an explicit evaluation time", node.ID)
		}
		state, exists := run.NodeStates[node.ID]
		if !exists || state.EnteredAt.IsZero() {
			return Decision{}, validationProblem("timer node %q has no recorded entry time", node.ID)
		}
		dueAt := state.EnteredAt.Add(node.Timer.After)
		if evaluation.Now.Before(dueAt) {
			return waiting(node.ID, "timer has not elapsed"), nil
		}
		return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], OutcomeElapsed))
	case NodeParallelSplit:
		edges := append([]Edge(nil), outgoing[node.ID]...)
		sortEdges(edges)
		return advanceAlong(run, node.ID, edges)
	case NodeParallelJoin:
		return evaluateJoin(run, node, outgoing[node.ID], incoming[node.ID])
	case NodeTerminal:
		result := node.Terminal.Result
		decision := Decision{
			FromNodeID:    node.ID,
			TerminalState: &result,
			Reason:        "terminal state reached",
		}
		switch result {
		case TerminalCompleted:
			decision.Disposition = DispositionComplete
			decision.ResultingRun = RunCompleted
		case TerminalFailed:
			decision.Disposition = DispositionFail
			decision.ResultingRun = RunFailed
		case TerminalCancelled:
			decision.Disposition = DispositionCancel
			decision.ResultingRun = RunCancelled
		}
		return decision, nil
	case NodeCompensation:
		edges := outgoing[node.ID]
		if len(edges) == 0 {
			return Decision{
				Disposition:  DispositionAdvance,
				FromNodeID:   node.ID,
				ResultingRun: RunCompensating,
				Reason:       "compensation step completed",
			}, nil
		}
		return advanceAlong(run, node.ID, selectOutcome(edges, evaluation.Outcome))
	case NodeAction, NodeVerification:
		return advanceAlong(run, node.ID, selectOutcome(outgoing[node.ID], evaluation.Outcome))
	default:
		return Decision{}, validationProblem("node %q has unsupported type %q", node.ID, node.Type)
	}
}

func evaluateCondition(config ConditionConfig, variables map[string]string) bool {
	value, exists := variables[config.Field]
	switch config.Operator {
	case ConditionEqual:
		return exists && value == config.Value
	case ConditionNotEqual:
		return !exists || value != config.Value
	case ConditionExists:
		return exists
	case ConditionNotExists:
		return !exists
	default:
		return false
	}
}

func evaluateJoin(run Run, node Node, outgoing, incoming []Edge) (Decision, error) {
	succeeded := 0
	failed := 0
	for _, edge := range incoming {
		switch run.NodeStates[edge.From].Status {
		case NodeSucceeded, NodeSkipped, NodeCompensated:
			succeeded++
		case NodeFailed, NodeCancelled:
			failed++
		}
	}

	switch node.Join.Mode {
	case JoinAll:
		if failed > 0 {
			return Decision{
				Disposition:  DispositionFail,
				FromNodeID:   node.ID,
				ResultingRun: RunFailed,
				Reason:       "parallel join has a failed or cancelled branch",
			}, nil
		}
		if succeeded != len(incoming) {
			return waiting(node.ID, "parallel join is waiting for all branches"), nil
		}
	case JoinAny:
		if succeeded == 0 {
			if failed == len(incoming) {
				return Decision{
					Disposition:  DispositionFail,
					FromNodeID:   node.ID,
					ResultingRun: RunFailed,
					Reason:       "all parallel join branches failed or were cancelled",
				}, nil
			}
			return waiting(node.ID, "parallel join is waiting for a successful branch"), nil
		}
	}
	return advanceAlong(run, node.ID, outgoing)
}

func waiting(nodeID, reason string) Decision {
	return Decision{
		Disposition:  DispositionWait,
		FromNodeID:   nodeID,
		ResultingRun: RunWaiting,
		Reason:       reason,
	}
}

func selectOutcome(edges []Edge, requested string) []Edge {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, edge := range edges {
			if edge.Outcome == requested {
				return []Edge{edge}
			}
		}
		for _, edge := range edges {
			if edge.Outcome == OutcomeDefault {
				return []Edge{edge}
			}
		}
		return nil
	}
	if len(edges) == 1 {
		return []Edge{edges[0]}
	}
	for _, edge := range edges {
		if edge.Outcome == OutcomeDefault {
			return []Edge{edge}
		}
	}
	return nil
}

func advanceAlong(run Run, fromNodeID string, edges []Edge) (Decision, error) {
	if len(edges) == 0 {
		return Decision{}, errors.Join(ErrNoMatchingEdge, ErrOutcomeRequired)
	}
	sortEdges(edges)
	next := make([]NextNode, 0, len(edges))
	for _, edge := range edges {
		if edge.MaxTraversals > 0 && run.EdgeTraversals[edge.ID] >= edge.MaxTraversals {
			return Decision{}, fmt.Errorf(
				"%w: edge %q used %d of %d traversals",
				ErrTraversalLimit,
				edge.ID,
				run.EdgeTraversals[edge.ID],
				edge.MaxTraversals,
			)
		}
		next = append(next, NextNode{NodeID: edge.To, EdgeID: edge.ID})
	}
	return Decision{
		Disposition:  DispositionAdvance,
		FromNodeID:   fromNodeID,
		Next:         next,
		ResultingRun: RunRunning,
		Reason:       "eligible transition selected",
	}, nil
}

func indexDefinition(definition Definition) (map[string]Node, map[string][]Edge, map[string][]Edge) {
	nodes := make(map[string]Node, len(definition.Nodes))
	outgoing := make(map[string][]Edge, len(definition.Nodes))
	incoming := make(map[string][]Edge, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodes[node.ID] = node
	}
	for _, edge := range definition.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge)
		incoming[edge.To] = append(incoming[edge.To], edge)
	}
	return nodes, outgoing, incoming
}

func sortEdges(edges []Edge) {
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Order != edges[j].Order {
			return edges[i].Order < edges[j].Order
		}
		return edges[i].ID < edges[j].ID
	})
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
