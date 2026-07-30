package workflowgraph

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateDefinition checks the complete static execution contract. In
// particular, removing all explicitly bounded edges must leave an acyclic
// graph, which proves that every possible cycle crosses a traversal bound.
func ValidateDefinition(definition Definition) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if definition.SchemaVersion != CurrentDefinitionSchemaVersion {
		add("schema version %d is unsupported", definition.SchemaVersion)
	}
	if strings.TrimSpace(definition.ID) == "" {
		add("definition id is required")
	}
	if definition.Version == 0 {
		add("definition version must be greater than zero")
	}
	if strings.TrimSpace(definition.EntryNodeID) == "" {
		add("entry node id is required")
	}
	if definition.MaxRunSteps == 0 {
		add("max run steps must be greater than zero")
	}
	if len(definition.Nodes) == 0 {
		add("at least one node is required")
	}

	nodes := make(map[string]Node, len(definition.Nodes))
	for _, node := range definition.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			add("node id is required")
			continue
		}
		if _, exists := nodes[node.ID]; exists {
			add("node %q is duplicated", node.ID)
			continue
		}
		nodes[node.ID] = node
		if !validNodeType(node.Type) {
			add("node %q has unsupported type %q", node.ID, node.Type)
		}
	}
	if _, exists := nodes[definition.EntryNodeID]; !exists && definition.EntryNodeID != "" {
		add("entry node %q does not exist", definition.EntryNodeID)
	}

	edges := make(map[string]Edge, len(definition.Edges))
	outgoing := make(map[string][]Edge, len(nodes))
	incoming := make(map[string][]Edge, len(nodes))
	for _, edge := range definition.Edges {
		if strings.TrimSpace(edge.ID) == "" {
			add("edge id is required")
			continue
		}
		if _, exists := edges[edge.ID]; exists {
			add("edge %q is duplicated", edge.ID)
			continue
		}
		edges[edge.ID] = edge
		if _, exists := nodes[edge.From]; !exists {
			add("edge %q references missing source node %q", edge.ID, edge.From)
		}
		if _, exists := nodes[edge.To]; !exists {
			add("edge %q references missing target node %q", edge.ID, edge.To)
		}
		if edge.From == "" || edge.To == "" {
			continue
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge)
		incoming[edge.To] = append(incoming[edge.To], edge)
	}

	if len(problems) == 0 {
		for _, node := range definition.Nodes {
			validateNode(node, nodes, outgoing[node.ID], add)
		}
		validateReachability(definition, nodes, outgoing, add)
		validateCycleBounds(nodes, outgoing, add)
		validateJoins(nodes, outgoing, incoming, add)
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &ValidationError{Problems: problems}
}

func validNodeType(nodeType NodeType) bool {
	switch nodeType {
	case NodeAction, NodeCondition, NodeHumanApproval, NodeWait, NodeTimer,
		NodeParallelSplit, NodeParallelJoin, NodeVerification, NodeCompensation,
		NodeTerminal:
		return true
	default:
		return false
	}
}

func validateNode(node Node, nodes map[string]Node, outgoing []Edge, add func(string, ...any)) {
	if node.CompensationNodeID != "" {
		if node.Type != NodeAction {
			add("node %q may reference compensation only when it is an action", node.ID)
		} else if compensation, exists := nodes[node.CompensationNodeID]; !exists {
			add("node %q references missing compensation node %q", node.ID, node.CompensationNodeID)
		} else if compensation.Type != NodeCompensation {
			add("node %q compensation target %q is not a compensation node", node.ID, node.CompensationNodeID)
		}
	}

	switch node.Type {
	case NodeAction, NodeVerification:
		if len(outgoing) == 0 {
			add("%s node %q requires at least one outgoing edge", node.Type, node.ID)
		}
		validateUniqueOutcomes(node.ID, outgoing, false, add)
		validateNoConfigs(node, node.Type, add)
	case NodeCondition:
		if node.Condition == nil {
			add("condition node %q requires condition configuration", node.ID)
		} else {
			if strings.TrimSpace(node.Condition.Field) == "" {
				add("condition node %q requires a field", node.ID)
			}
			switch node.Condition.Operator {
			case ConditionEqual, ConditionNotEqual:
			case ConditionExists, ConditionNotExists:
				if node.Condition.Value != "" {
					add("condition node %q must not set a value for operator %q", node.ID, node.Condition.Operator)
				}
			default:
				add("condition node %q has unsupported operator %q", node.ID, node.Condition.Operator)
			}
		}
		validateExactOutcomes(node.ID, outgoing, []string{OutcomeTrue, OutcomeFalse}, add)
		if node.Timer != nil || node.Join != nil || node.Terminal != nil {
			add("condition node %q contains configuration for another node type", node.ID)
		}
	case NodeHumanApproval:
		validateExactOutcomes(node.ID, outgoing, []string{OutcomeApproved, OutcomeRejected}, add)
		validateNoConfigs(node, node.Type, add)
	case NodeWait:
		if len(outgoing) == 0 {
			add("wait node %q requires at least one signal edge", node.ID)
		}
		validateUniqueOutcomes(node.ID, outgoing, true, add)
		validateNoConfigs(node, node.Type, add)
	case NodeTimer:
		if node.Timer == nil || node.Timer.After <= 0 {
			add("timer node %q requires a positive duration", node.ID)
		}
		validateExactOutcomes(node.ID, outgoing, []string{OutcomeElapsed}, add)
		if node.Condition != nil || node.Join != nil || node.Terminal != nil {
			add("timer node %q contains configuration for another node type", node.ID)
		}
	case NodeParallelSplit:
		if len(outgoing) < 2 {
			add("parallel split node %q requires at least two branches", node.ID)
		}
		targets := map[string]struct{}{}
		for _, edge := range outgoing {
			if _, duplicate := targets[edge.To]; duplicate {
				add("parallel split node %q has duplicate branch target %q", node.ID, edge.To)
			}
			targets[edge.To] = struct{}{}
		}
		validateNoConfigs(node, node.Type, add)
	case NodeParallelJoin:
		if node.Join == nil {
			add("parallel join node %q requires join configuration", node.ID)
		} else {
			if strings.TrimSpace(node.Join.SplitNodeID) == "" {
				add("parallel join node %q requires split node id", node.ID)
			}
			if node.Join.Mode != JoinAll && node.Join.Mode != JoinAny {
				add("parallel join node %q has unsupported mode %q", node.ID, node.Join.Mode)
			}
		}
		if len(outgoing) != 1 {
			add("parallel join node %q requires exactly one outgoing edge", node.ID)
		}
		if node.Condition != nil || node.Timer != nil || node.Terminal != nil {
			add("parallel join node %q contains configuration for another node type", node.ID)
		}
	case NodeCompensation:
		if len(outgoing) > 1 {
			add("compensation node %q may have at most one outgoing edge", node.ID)
		}
		validateUniqueOutcomes(node.ID, outgoing, false, add)
		validateNoConfigs(node, node.Type, add)
	case NodeTerminal:
		if node.Terminal == nil {
			add("terminal node %q requires terminal configuration", node.ID)
		} else if node.Terminal.Result != TerminalCompleted &&
			node.Terminal.Result != TerminalFailed &&
			node.Terminal.Result != TerminalCancelled {
			add("terminal node %q has unsupported result %q", node.ID, node.Terminal.Result)
		}
		if len(outgoing) != 0 {
			add("terminal node %q must not have outgoing edges", node.ID)
		}
		if node.Condition != nil || node.Timer != nil || node.Join != nil {
			add("terminal node %q contains configuration for another node type", node.ID)
		}
	}
}

func validateNoConfigs(node Node, expected NodeType, add func(string, ...any)) {
	if node.Condition != nil || node.Timer != nil || node.Join != nil || node.Terminal != nil {
		add("%s node %q contains configuration for another node type", expected, node.ID)
	}
}

func validateUniqueOutcomes(nodeID string, edges []Edge, requireNonEmpty bool, add func(string, ...any)) {
	outcomes := map[string]struct{}{}
	for _, edge := range edges {
		outcome := strings.TrimSpace(edge.Outcome)
		if requireNonEmpty && outcome == "" {
			add("node %q edge %q requires an outcome", nodeID, edge.ID)
		}
		if _, duplicate := outcomes[outcome]; duplicate {
			add("node %q has duplicate outcome %q", nodeID, outcome)
		}
		outcomes[outcome] = struct{}{}
	}
}

func validateExactOutcomes(nodeID string, edges []Edge, expected []string, add func(string, ...any)) {
	actual := map[string]int{}
	for _, edge := range edges {
		actual[edge.Outcome]++
	}
	for _, outcome := range expected {
		if actual[outcome] != 1 {
			add("node %q requires exactly one %q edge", nodeID, outcome)
		}
		delete(actual, outcome)
	}
	for outcome := range actual {
		add("node %q has unexpected outcome %q", nodeID, outcome)
	}
}

func validateReachability(
	definition Definition,
	nodes map[string]Node,
	outgoing map[string][]Edge,
	add func(string, ...any),
) {
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(nodeID string) {
		if reachable[nodeID] {
			return
		}
		reachable[nodeID] = true
		node := nodes[nodeID]
		if node.CompensationNodeID != "" {
			visit(node.CompensationNodeID)
		}
		for _, edge := range outgoing[nodeID] {
			visit(edge.To)
		}
	}
	visit(definition.EntryNodeID)
	for nodeID := range nodes {
		if !reachable[nodeID] {
			add("node %q is unreachable from the entry or a compensation reference", nodeID)
		}
	}

	canTerminate := map[string]bool{}
	reverse := make(map[string][]string, len(nodes))
	var terminals int
	for _, node := range nodes {
		if node.Type == NodeTerminal {
			terminals++
			canTerminate[node.ID] = true
		}
	}
	for from, edges := range outgoing {
		for _, edge := range edges {
			reverse[edge.To] = append(reverse[edge.To], from)
		}
	}
	queue := make([]string, 0, len(canTerminate))
	for nodeID := range canTerminate {
		queue = append(queue, nodeID)
	}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		for _, previous := range reverse[nodeID] {
			if !canTerminate[previous] {
				canTerminate[previous] = true
				queue = append(queue, previous)
			}
		}
	}
	if terminals == 0 {
		add("at least one terminal node is required")
		return
	}
	for _, node := range nodes {
		if node.Type != NodeCompensation && !canTerminate[node.ID] {
			add("node %q has no path to a terminal state", node.ID)
		}
	}
}

func validateCycleBounds(nodes map[string]Node, outgoing map[string][]Edge, add func(string, ...any)) {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(nodes))
	var cycleAt string
	var walk func(string) bool
	walk = func(nodeID string) bool {
		state[nodeID] = visiting
		for _, edge := range outgoing[nodeID] {
			if edge.MaxTraversals > 0 {
				continue
			}
			switch state[edge.To] {
			case visiting:
				cycleAt = edge.ID
				return true
			case unvisited:
				if walk(edge.To) {
					return true
				}
			}
		}
		state[nodeID] = visited
		return false
	}
	for nodeID := range nodes {
		if state[nodeID] == unvisited && walk(nodeID) {
			add("cycle containing edge %q has no explicit traversal bound", cycleAt)
			return
		}
	}
}

func validateJoins(
	nodes map[string]Node,
	outgoing map[string][]Edge,
	incoming map[string][]Edge,
	add func(string, ...any),
) {
	joinsBySplit := map[string][]Node{}
	for _, node := range nodes {
		if node.Type == NodeParallelJoin && node.Join != nil {
			joinsBySplit[node.Join.SplitNodeID] = append(joinsBySplit[node.Join.SplitNodeID], node)
		}
	}

	for _, node := range nodes {
		if node.Type != NodeParallelSplit {
			continue
		}
		joins := joinsBySplit[node.ID]
		if len(joins) != 1 {
			add("parallel split node %q must be paired with exactly one join", node.ID)
			continue
		}
		join := joins[0]
		if len(incoming[join.ID]) < 2 {
			add("parallel join node %q requires at least two incoming branches", join.ID)
		}
		branches := outgoing[node.ID]
		for _, branch := range branches {
			if !canReachBefore(branch.To, join.ID, "", outgoing) {
				add("parallel split node %q branch %q cannot reach join %q", node.ID, branch.ID, join.ID)
			}
		}
		for _, edge := range incoming[join.ID] {
			reachableFromBranch := false
			for _, branch := range branches {
				if canReachBefore(branch.To, edge.From, join.ID, outgoing) {
					reachableFromBranch = true
					break
				}
			}
			if !reachableFromBranch {
				add("parallel join node %q has incoming edge %q outside split %q branches", join.ID, edge.ID, node.ID)
			}
		}
	}

	for _, node := range nodes {
		if node.Type != NodeParallelJoin || node.Join == nil {
			continue
		}
		split, exists := nodes[node.Join.SplitNodeID]
		if !exists {
			add("parallel join node %q references missing split %q", node.ID, node.Join.SplitNodeID)
		} else if split.Type != NodeParallelSplit {
			add("parallel join node %q references non-split node %q", node.ID, node.Join.SplitNodeID)
		}
	}
}

func canReachBefore(start, target, stop string, outgoing map[string][]Edge) bool {
	if start == target {
		return true
	}
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if seen[nodeID] || (stop != "" && nodeID == stop) {
			continue
		}
		seen[nodeID] = true
		for _, edge := range outgoing[nodeID] {
			if edge.To == target {
				return true
			}
			queue = append(queue, edge.To)
		}
	}
	return false
}
