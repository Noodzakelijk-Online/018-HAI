package plangraph

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxNodes = 500
	maxEdges = 2000
)

func normalizePlan(plan Plan) Plan {
	plan.OwnerIdentity = strings.TrimSpace(plan.OwnerIdentity)
	plan.Title = strings.TrimSpace(plan.Title)
	plan.CreatedBy = strings.TrimSpace(plan.CreatedBy)
	plan.IdempotencyKey = strings.TrimSpace(plan.IdempotencyKey)
	plan.RequestDigest = strings.ToLower(strings.TrimSpace(plan.RequestDigest))
	plan.CreatedAt = canonicalTime(plan.CreatedAt)
	if plan.AcceptedAt != nil {
		value := canonicalTime(*plan.AcceptedAt)
		plan.AcceptedAt = &value
	}
	for index := range plan.Nodes {
		node := &plan.Nodes[index]
		node.ID = strings.TrimSpace(node.ID)
		node.Type = strings.TrimSpace(node.Type)
		node.Title = strings.TrimSpace(node.Title)
		node.Owner = strings.TrimSpace(node.Owner)
		node.FrameworkDigest = strings.ToLower(strings.TrimSpace(node.FrameworkDigest))
		node.EvidenceDigest = strings.ToLower(strings.TrimSpace(node.EvidenceDigest))
		node.Bindings.PursuitID = strings.TrimSpace(node.Bindings.PursuitID)
		node.Bindings.WorkflowID = strings.TrimSpace(node.Bindings.WorkflowID)
		node.Bindings.TaskID = strings.TrimSpace(node.Bindings.TaskID)
		node.Bindings.AgentID = strings.TrimSpace(node.Bindings.AgentID)
		if node.EarliestStart != nil {
			value := canonicalTime(*node.EarliestStart)
			node.EarliestStart = &value
		}
		if node.Deadline != nil {
			value := canonicalTime(*node.Deadline)
			node.Deadline = &value
		}
	}
	for index := range plan.Edges {
		edge := &plan.Edges[index]
		edge.ID = strings.TrimSpace(edge.ID)
		edge.From = strings.TrimSpace(edge.From)
		edge.To = strings.TrimSpace(edge.To)
		edge.Type = strings.TrimSpace(edge.Type)
		if edge.Type == "" {
			edge.Type = "finish_to_start"
		}
	}
	if plan.Repair != nil {
		plan.Repair.Reason = strings.TrimSpace(plan.Repair.Reason)
		plan.Repair.Trigger = strings.TrimSpace(plan.Repair.Trigger)
		plan.Repair.CreatedBy = strings.TrimSpace(plan.Repair.CreatedBy)
		plan.Repair.PreviousDigest = strings.ToLower(strings.TrimSpace(plan.Repair.PreviousDigest))
		plan.Repair.CreatedAt = canonicalTime(plan.Repair.CreatedAt)
	}
	sort.Slice(plan.Nodes, func(i, j int) bool { return plan.Nodes[i].ID < plan.Nodes[j].ID })
	sort.Slice(plan.Edges, func(i, j int) bool { return plan.Edges[i].ID < plan.Edges[j].ID })
	plan.CanExecute = false
	return plan
}

// PostgreSQL timestamptz stores microsecond precision. Digest-bound times must
// use the same precision before insertion so a persisted row verifies after a
// round trip instead of changing identity at the storage boundary.
func canonicalTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func validatePlan(plan Plan) error {
	if plan.ID == uuid.Nil {
		return fmt.Errorf("plan id is required")
	}
	if plan.OwnerIdentity == "" {
		return fmt.Errorf("owner identity is required")
	}
	if plan.Title == "" || len(plan.Title) > 300 {
		return fmt.Errorf("plan title is required and must not exceed 300 characters")
	}
	if plan.Status != StatusDraft && plan.Status != StatusAccepted {
		return fmt.Errorf("invalid plan status %q", plan.Status)
	}
	if plan.Revision == 0 {
		return fmt.Errorf("plan revision must be positive")
	}
	if plan.Revision == 1 && (plan.ParentRevision != 0 || plan.ParentDigest != "") {
		return fmt.Errorf("first plan revision cannot have a parent")
	}
	if plan.Revision > 1 {
		if plan.ParentRevision+1 != plan.Revision || !validDigest(plan.ParentDigest) {
			return fmt.Errorf("plan parent revision and digest must bind the prior revision")
		}
	}
	if plan.CreatedBy == "" || plan.CreatedAt.IsZero() {
		return fmt.Errorf("plan creator and creation time are required")
	}
	if plan.Status == StatusAccepted && plan.AcceptedAt == nil {
		return fmt.Errorf("accepted plan revision requires acceptedAt")
	}
	if plan.Status == StatusDraft && plan.AcceptedAt != nil {
		return fmt.Errorf("draft plan revision cannot have acceptedAt")
	}
	if plan.CanExecute {
		return fmt.Errorf("plan graph cannot grant execution authority")
	}
	if plan.IdempotencyKey != "" && !validDigest(plan.RequestDigest) {
		return fmt.Errorf("idempotent plan revision requires a request digest")
	}
	if len(plan.Nodes) == 0 || len(plan.Nodes) > maxNodes {
		return fmt.Errorf("plan must contain between 1 and %d nodes", maxNodes)
	}
	if len(plan.Edges) > maxEdges {
		return fmt.Errorf("plan must not exceed %d dependency edges", maxEdges)
	}
	ids := make(map[string]struct{}, len(plan.Nodes))
	for index, node := range plan.Nodes {
		if err := validateNode(node); err != nil {
			return fmt.Errorf("node %d: %w", index, err)
		}
		if _, exists := ids[node.ID]; exists {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		ids[node.ID] = struct{}{}
	}
	edgeIDs := make(map[string]struct{}, len(plan.Edges))
	for index, edge := range plan.Edges {
		if edge.ID == "" || edge.From == "" || edge.To == "" {
			return fmt.Errorf("edge %d requires id, from, and to", index)
		}
		if len(edge.ID) > 160 || len(edge.Type) > 80 {
			return fmt.Errorf("edge %d identifier or type exceeds its limit", index)
		}
		if edge.From == edge.To {
			return fmt.Errorf("edge %q cannot depend on itself", edge.ID)
		}
		if _, ok := ids[edge.From]; !ok {
			return fmt.Errorf("edge %q references unknown from node %q", edge.ID, edge.From)
		}
		if _, ok := ids[edge.To]; !ok {
			return fmt.Errorf("edge %q references unknown to node %q", edge.ID, edge.To)
		}
		if edge.LagMinutes < 0 || edge.LagMinutes > 10*365*24*60 {
			return fmt.Errorf("edge %q lag minutes is outside the allowed range", edge.ID)
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return fmt.Errorf("duplicate edge id %q", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
	}
	if err := validateDAG(plan.Nodes, plan.Edges); err != nil {
		return err
	}
	if plan.Repair != nil {
		if plan.Repair.Reason == "" || plan.Repair.Trigger == "" || plan.Repair.CreatedBy == "" || plan.Repair.CreatedAt.IsZero() {
			return fmt.Errorf("repair provenance requires reason, trigger, creator, and timestamp")
		}
		if plan.Repair.PreviousRevision != plan.ParentRevision || plan.Repair.PreviousDigest != plan.ParentDigest {
			return fmt.Errorf("repair provenance must bind the parent revision")
		}
	}
	return nil
}

func validateNode(node Node) error {
	if node.ID == "" || node.Type == "" || node.Title == "" || node.Owner == "" {
		return fmt.Errorf("id, type, title, and owner are required")
	}
	if len(node.ID) > 160 || len(node.Type) > 80 || len(node.Title) > 300 || len(node.Owner) > 255 {
		return fmt.Errorf("identity or title exceeds its limit")
	}
	switch node.Status {
	case NodePlanned, NodeReady, NodeBlocked, NodeWaiting, NodeNeedsApproval, NodeCompleted, NodeFailed:
	default:
		return fmt.Errorf("invalid status %q", node.Status)
	}
	switch node.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("invalid risk %q", node.Risk)
	}
	switch node.ApprovalState {
	case ApprovalNotRequired, ApprovalRequired, ApprovalGranted, ApprovalRejected:
	default:
		return fmt.Errorf("invalid approval state %q", node.ApprovalState)
	}
	if node.EstimatedMinutes < 0 || node.EstimatedMinutes > 10*365*24*60 {
		return fmt.Errorf("estimated minutes is outside the allowed range")
	}
	if math.IsNaN(node.EstimatedCostEUR) || math.IsInf(node.EstimatedCostEUR, 0) || node.EstimatedCostEUR < 0 || node.EstimatedCostEUR > 1_000_000_000 {
		return fmt.Errorf("estimated cost is outside the allowed range")
	}
	if node.EarliestStart != nil && node.Deadline != nil && node.Deadline.Before(*node.EarliestStart) {
		return fmt.Errorf("deadline cannot precede earliest start")
	}
	for label, digest := range map[string]string{"framework": node.FrameworkDigest, "evidence": node.EvidenceDigest} {
		if digest != "" && !validDigest(digest) {
			return fmt.Errorf("%s digest must be a lowercase SHA-256 digest", label)
		}
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateDAG(nodes []Node, edges []Edge) error {
	indegree := make(map[string]int, len(nodes))
	adjacency := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range edges {
		indegree[edge.To]++
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	queue := make([]string, 0, len(nodes))
	for id, degree := range indegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		visited++
		for _, next := range adjacency[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(nodes) {
		return fmt.Errorf("plan dependency graph contains a cycle")
	}
	return nil
}

func clonePlan(plan Plan) Plan {
	copy := plan
	copy.Nodes = cloneNodes(plan.Nodes)
	copy.Edges = cloneEdges(plan.Edges)
	if plan.AcceptedAt != nil {
		value := *plan.AcceptedAt
		copy.AcceptedAt = &value
	}
	if plan.Repair != nil {
		value := *plan.Repair
		copy.Repair = &value
	}
	return copy
}

func cloneNodes(nodes []Node) []Node {
	if nodes == nil {
		return nil
	}
	cloned := make([]Node, len(nodes))
	for index := range nodes {
		cloned[index] = cloneNode(nodes[index])
	}
	return cloned
}

func cloneNode(node Node) Node {
	copy := node
	if node.EarliestStart != nil {
		value := *node.EarliestStart
		copy.EarliestStart = &value
	}
	if node.Deadline != nil {
		value := *node.Deadline
		copy.Deadline = &value
	}
	return copy
}

func cloneEdges(edges []Edge) []Edge {
	return append([]Edge(nil), edges...)
}

func utcNow(now func() time.Time) time.Time {
	return now().UTC().Round(0)
}
