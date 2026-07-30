package knowledgegraph

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultRetrievalDepth = 2
	maxRetrievalDepth     = 5
	defaultRetrievalLimit = 20
	maxRetrievalLimit     = 100
)

type retrievalState struct {
	node    Node
	score   float64
	depth   int
	direct  bool
	path    []string
	factors []string
}

func (s *Service) Retrieve(ctx context.Context, request RetrieveRequest) (SubgraphResult, error) {
	if err := requireOwner(request.OwnerIdentity); err != nil {
		return SubgraphResult{}, err
	}
	request.OwnerIdentity = strings.TrimSpace(request.OwnerIdentity)
	request.ProjectKeys = normalizeStrings(request.ProjectKeys)
	if request.At.IsZero() {
		request.At = s.clock().UTC()
	}
	request.MaxDepth = bounded(request.MaxDepth, defaultRetrievalDepth, maxRetrievalDepth)
	request.Limit = bounded(request.Limit, defaultRetrievalLimit, maxRetrievalLimit)

	options := ListOptions{IncludeArchived: request.IncludeArchived}
	nodes, err := s.repo.ListNodes(ctx, request.OwnerIdentity, options)
	if err != nil {
		return SubgraphResult{}, err
	}
	edges, err := s.repo.ListEdges(ctx, request.OwnerIdentity, options)
	if err != nil {
		return SubgraphResult{}, err
	}

	eligibleNodes := make(map[string]Node)
	for _, node := range nodes {
		if !visibleAt(node.LocalOnly, request.AllowLocalOnly, node.ValidFrom, node.ValidUntil, request.At) {
			continue
		}
		eligibleNodes[node.ID] = node
	}
	eligibleEdges := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if !visibleAt(edge.LocalOnly, request.AllowLocalOnly, edge.ValidFrom, edge.ValidUntil, request.At) {
			continue
		}
		if _, ok := eligibleNodes[edge.FromNodeID]; !ok {
			continue
		}
		if _, ok := eligibleNodes[edge.ToNodeID]; !ok {
			continue
		}
		eligibleEdges = append(eligibleEdges, edge)
	}

	queryTokens := tokenSet(request.Query)
	states := make(map[string]retrievalState)
	queue := make([]string, 0)
	for _, node := range nodes {
		if _, ok := eligibleNodes[node.ID]; !ok {
			continue
		}
		score, factors, matches := directScore(node, queryTokens, request.ProjectKeys, request.At)
		if !matches {
			continue
		}
		states[node.ID] = retrievalState{
			node: node, score: score, depth: 0, direct: true,
			path: []string{node.ID}, factors: factors,
		}
		queue = append(queue, node.ID)
	}
	sort.Strings(queue)

	adjacency := buildAdjacency(eligibleEdges)
	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		currentID := queue[queueIndex]
		current := states[currentID]
		if current.depth >= request.MaxDepth {
			continue
		}
		for _, connection := range adjacency[currentID] {
			neighbor, ok := eligibleNodes[connection.neighborID]
			if !ok {
				continue
			}
			nextDepth := current.depth + 1
			nextScore := clampScore(current.score * 0.68 * confidenceFloor(connection.edge.Confidence) * verificationRetrievalWeight(connection.edge.VerificationStatus))
			factors := []string{
				fmt.Sprintf("connected at depth %d through %s", nextDepth, connection.edge.Relationship),
				fmt.Sprintf("edge confidence %.2f", connection.edge.Confidence),
			}
			known, exists := states[neighbor.ID]
			if exists && known.score >= nextScore {
				continue
			}
			path := append(append([]string(nil), current.path...), connection.edge.ID, neighbor.ID)
			states[neighbor.ID] = retrievalState{
				node: neighbor, score: nextScore, depth: nextDepth,
				direct: false, path: path, factors: factors,
			}
			queue = append(queue, neighbor.ID)
		}
	}

	ranked := make([]retrievalState, 0, len(states))
	for _, state := range states {
		ranked = append(ranked, state)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].depth != ranked[j].depth {
			return ranked[i].depth < ranked[j].depth
		}
		return ranked[i].node.ID < ranked[j].node.ID
	})

	truncated := len(ranked) > request.Limit
	if truncated {
		ranked = ranked[:request.Limit]
	}

	resultNodes := make([]RetrievedNode, 0, len(ranked))
	selectedIDs := make(map[string]struct{}, len(ranked))
	for _, state := range ranked {
		selectedIDs[state.node.ID] = struct{}{}
		summary := strings.Join(state.factors, "; ")
		if state.direct {
			summary = "directly relevant: " + summary
		} else {
			summary = "retrieved by bounded graph traversal: " + summary
		}
		resultNodes = append(resultNodes, RetrievedNode{
			Node: state.node,
			Explanation: RetrievalExplanation{
				NodeID: state.node.ID, Score: roundScore(state.score),
				Depth: state.depth, DirectMatch: state.direct,
				Path:    append([]string(nil), state.path...),
				Factors: append([]string(nil), state.factors...),
				Summary: summary,
			},
		})
	}

	resultEdges := make([]Edge, 0)
	for _, edge := range eligibleEdges {
		_, fromSelected := selectedIDs[edge.FromNodeID]
		_, toSelected := selectedIDs[edge.ToNodeID]
		if fromSelected && toSelected {
			resultEdges = append(resultEdges, edge)
		}
	}
	sort.Slice(resultEdges, func(i, j int) bool { return resultEdges[i].ID < resultEdges[j].ID })

	return SubgraphResult{
		Nodes: resultNodes, Edges: resultEdges,
		Query:       strings.TrimSpace(request.Query),
		ProjectKeys: request.ProjectKeys,
		MaxDepth:    request.MaxDepth, Limit: request.Limit, Truncated: truncated,
		Explanation: fmt.Sprintf(
			"owner-scoped retrieval selected %d nodes and %d connecting edges using lexical relevance, project match, recency, confidence, verification, and traversal depth",
			len(resultNodes), len(resultEdges),
		),
	}, nil
}

type adjacencyEntry struct {
	neighborID string
	edge       Edge
}

func buildAdjacency(edges []Edge) map[string][]adjacencyEntry {
	result := make(map[string][]adjacencyEntry)
	for _, edge := range edges {
		result[edge.FromNodeID] = append(result[edge.FromNodeID], adjacencyEntry{neighborID: edge.ToNodeID, edge: edge})
		result[edge.ToNodeID] = append(result[edge.ToNodeID], adjacencyEntry{neighborID: edge.FromNodeID, edge: edge})
	}
	for nodeID := range result {
		sort.Slice(result[nodeID], func(i, j int) bool {
			if result[nodeID][i].edge.ID != result[nodeID][j].edge.ID {
				return result[nodeID][i].edge.ID < result[nodeID][j].edge.ID
			}
			return result[nodeID][i].neighborID < result[nodeID][j].neighborID
		})
	}
	return result
}

func directScore(node Node, queryTokens map[string]struct{}, projectKeys []string, at time.Time) (float64, []string, bool) {
	nodeTokens := tokenSet(strings.Join([]string{
		node.Label,
		node.Content,
		strings.Join(node.Tags, " "),
		strings.Join(node.ProjectKeys, " "),
		flattenMap(node.Properties),
	}, " "))
	lexical := tokenCoverage(queryTokens, nodeTokens)
	project := projectMatch(projectKeys, node.ProjectKeys)
	if len(queryTokens) > 0 && lexical == 0 && project == 0 {
		return 0, nil, false
	}
	if len(queryTokens) == 0 && len(projectKeys) > 0 && project == 0 {
		return 0, nil, false
	}

	recency := recencyScore(node.UpdatedAt, at)
	verification := verificationRetrievalWeight(node.VerificationStatus)
	score := clampScore(
		0.45*lexical +
			0.22*project +
			0.13*confidenceFloor(node.Confidence) +
			0.10*recency +
			0.10*verification,
	)
	factors := make([]string, 0, 5)
	if lexical > 0 {
		factors = append(factors, fmt.Sprintf("lexical relevance %.2f", lexical))
	}
	if project > 0 {
		factors = append(factors, fmt.Sprintf("project match %.2f", project))
	}
	factors = append(factors,
		fmt.Sprintf("confidence %.2f", node.Confidence),
		fmt.Sprintf("recency %.2f", recency),
		fmt.Sprintf("verification %s", node.VerificationStatus),
	)
	return score, factors, true
}

func visibleAt(localOnly, allowLocalOnly bool, validFrom, validUntil *time.Time, at time.Time) bool {
	if localOnly && !allowLocalOnly {
		return false
	}
	if validFrom != nil && at.Before(*validFrom) {
		return false
	}
	if validUntil != nil && at.After(*validUntil) {
		return false
	}
	return true
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(token)) > 1 {
			result[token] = struct{}{}
		}
	}
	return result
}

func tokenCoverage(query, candidate map[string]struct{}) float64 {
	if len(query) == 0 {
		return 0
	}
	matches := 0
	for token := range query {
		if _, ok := candidate[token]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(query))
}

func projectMatch(requested, candidate []string) float64 {
	if len(requested) == 0 {
		return 0
	}
	candidateSet := make(map[string]struct{}, len(candidate))
	for _, value := range candidate {
		candidateSet[normalizedValue(value)] = struct{}{}
	}
	matches := 0
	for _, value := range requested {
		if _, ok := candidateSet[normalizedValue(value)]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(requested))
}

func flattenMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result strings.Builder
	for _, key := range keys {
		result.WriteString(key)
		result.WriteByte(' ')
		result.WriteString(values[key])
		result.WriteByte(' ')
	}
	return result.String()
}

func recencyScore(updatedAt, at time.Time) float64 {
	if updatedAt.IsZero() || !updatedAt.Before(at) {
		return 1
	}
	age := at.Sub(updatedAt)
	switch {
	case age <= 7*24*time.Hour:
		return 1
	case age <= 30*24*time.Hour:
		return 0.8
	case age <= 180*24*time.Hour:
		return 0.5
	default:
		return 0.2
	}
}

func verificationRetrievalWeight(status VerificationStatus) float64 {
	switch status {
	case VerificationVerified, VerificationHumanApproved, VerificationTestPassed:
		return 1
	case VerificationSourceSupported, VerificationSchemaValidated:
		return 0.85
	case VerificationUnverified:
		return 0.55
	case VerificationUncertain, VerificationNeedsReview:
		return 0.4
	case VerificationConflicting:
		return 0.3
	case VerificationUnsupported:
		return 0.1
	default:
		return 0.2
	}
}

func confidenceFloor(value float64) float64 {
	if value == 0 {
		return 0.25
	}
	return value
}

func bounded(value, defaultValue, maximum int) int {
	if value <= 0 {
		return defaultValue
	}
	if value > maximum {
		return maximum
	}
	return value
}

func clampScore(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func roundScore(value float64) float64 {
	return math.Round(value*10000) / 10000
}
