package knowledgegraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Clock func() time.Time

type Service struct {
	repo  Repository
	clock Clock
}

func NewService(repo Repository, clock Clock) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, clock: clock}
}

func (s *Service) CreateNode(ctx context.Context, request CreateNodeRequest) (NodeWriteResult, error) {
	now := s.clock().UTC()
	node := nodeFromRequest(request, now)
	if err := validateNode(node); err != nil {
		return NodeWriteResult{}, err
	}
	if err := s.validateSourceReferences(ctx, node.OwnerIdentity, node.Sources); err != nil {
		return NodeWriteResult{}, err
	}

	existing, err := s.repo.ListNodes(ctx, node.OwnerIdentity, ListOptions{})
	if err != nil {
		return NodeWriteResult{}, err
	}

	var conflicts []Node
	for _, candidate := range existing {
		if candidate.DeduplicationKey != node.DeduplicationKey || candidate.Kind != node.Kind {
			continue
		}
		if nodesCompatible(candidate, node) {
			merged := mergeNodes(candidate, node, now)
			updated, updateErr := s.repo.UpdateNode(ctx, merged)
			if updateErr != nil {
				return NodeWriteResult{}, updateErr
			}
			return NodeWriteResult{Node: updated, Action: WriteMerged}, nil
		}
		conflicts = append(conflicts, candidate)
	}

	if len(conflicts) > 0 {
		groupID := conflictGroupID(node.OwnerIdentity, node.Kind, node.DeduplicationKey)
		conflictingIDs := make([]string, 0, len(conflicts))
		for _, candidate := range conflicts {
			candidate.ConflictGroupID = groupID
			candidate.VerificationStatus = VerificationConflicting
			candidate.UpdatedAt = now
			if _, err := s.repo.UpdateNode(ctx, candidate); err != nil {
				return NodeWriteResult{}, err
			}
			conflictingIDs = append(conflictingIDs, candidate.ID)
		}
		sort.Strings(conflictingIDs)
		node.ConflictGroupID = groupID
		node.VerificationStatus = VerificationConflicting
		created, createErr := s.repo.CreateNode(ctx, node)
		if createErr != nil {
			return NodeWriteResult{}, createErr
		}
		return NodeWriteResult{
			Node:               created,
			Action:             WriteConflict,
			ConflictingNodeIDs: conflictingIDs,
		}, nil
	}

	created, err := s.repo.CreateNode(ctx, node)
	if err != nil {
		return NodeWriteResult{}, err
	}
	return NodeWriteResult{Node: created, Action: WriteCreated}, nil
}

func (s *Service) CreateEdge(ctx context.Context, request CreateEdgeRequest) (EdgeWriteResult, error) {
	now := s.clock().UTC()
	edge := edgeFromRequest(request, now)
	if err := validateEdge(edge); err != nil {
		return EdgeWriteResult{}, err
	}
	if _, err := s.repo.GetNode(ctx, edge.OwnerIdentity, edge.FromNodeID); err != nil {
		return EdgeWriteResult{}, fmt.Errorf("from node: %w", err)
	}
	if _, err := s.repo.GetNode(ctx, edge.OwnerIdentity, edge.ToNodeID); err != nil {
		return EdgeWriteResult{}, fmt.Errorf("to node: %w", err)
	}
	if err := s.validateSourceReferences(ctx, edge.OwnerIdentity, edge.Sources); err != nil {
		return EdgeWriteResult{}, err
	}

	existing, err := s.repo.ListEdges(ctx, edge.OwnerIdentity, ListOptions{})
	if err != nil {
		return EdgeWriteResult{}, err
	}
	for _, candidate := range existing {
		if candidate.FromNodeID == edge.FromNodeID &&
			candidate.ToNodeID == edge.ToNodeID &&
			candidate.Relationship == edge.Relationship &&
			edgesCompatible(candidate, edge) {
			merged := mergeEdges(candidate, edge, now)
			updated, updateErr := s.repo.UpdateEdge(ctx, merged)
			if updateErr != nil {
				return EdgeWriteResult{}, updateErr
			}
			return EdgeWriteResult{Edge: updated, Action: WriteMerged}, nil
		}
	}

	created, err := s.repo.CreateEdge(ctx, edge)
	if err != nil {
		return EdgeWriteResult{}, err
	}
	return EdgeWriteResult{Edge: created, Action: WriteCreated}, nil
}

func (s *Service) GetNode(ctx context.Context, ownerIdentity, id string) (Node, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return Node{}, err
	}
	return s.repo.GetNode(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(id))
}

func (s *Service) GetEdge(ctx context.Context, ownerIdentity, id string) (Edge, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return Edge{}, err
	}
	return s.repo.GetEdge(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(id))
}

func (s *Service) ListNodes(ctx context.Context, ownerIdentity string, options ListOptions) ([]Node, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return nil, err
	}
	return s.repo.ListNodes(ctx, strings.TrimSpace(ownerIdentity), options)
}

func (s *Service) ListEdges(ctx context.Context, ownerIdentity string, options ListOptions) ([]Edge, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return nil, err
	}
	return s.repo.ListEdges(ctx, strings.TrimSpace(ownerIdentity), options)
}

func (s *Service) ArchiveNode(ctx context.Context, ownerIdentity, id string, archived bool) (Node, error) {
	node, err := s.GetNode(ctx, ownerIdentity, id)
	if err != nil {
		return Node{}, err
	}
	if node.DeletedAt != nil {
		return Node{}, fmt.Errorf("cannot archive deleted node")
	}
	node.Archived = archived
	node.UpdatedAt = s.clock().UTC()
	return s.repo.UpdateNode(ctx, node)
}

func (s *Service) ArchiveEdge(ctx context.Context, ownerIdentity, id string, archived bool) (Edge, error) {
	edge, err := s.GetEdge(ctx, ownerIdentity, id)
	if err != nil {
		return Edge{}, err
	}
	if edge.DeletedAt != nil {
		return Edge{}, fmt.Errorf("cannot archive deleted edge")
	}
	edge.Archived = archived
	edge.UpdatedAt = s.clock().UTC()
	return s.repo.UpdateEdge(ctx, edge)
}

// CorrectNode preserves the old record, archives it, and creates a new record
// linked in both directions through explicit correction metadata.
func (s *Service) CorrectNode(ctx context.Context, ownerIdentity, id string, correction CreateNodeRequest) (NodeWriteResult, error) {
	old, err := s.GetNode(ctx, ownerIdentity, id)
	if err != nil {
		return NodeWriteResult{}, err
	}
	if old.DeletedAt != nil {
		return NodeWriteResult{}, fmt.Errorf("cannot correct deleted node")
	}

	correction.OwnerIdentity = old.OwnerIdentity
	if correction.Kind == "" {
		correction.Kind = old.Kind
	}
	if correction.DeduplicationKey == "" {
		correction.DeduplicationKey = old.DeduplicationKey
	}
	if correction.Label == "" {
		correction.Label = old.Label
	}
	if correction.Sensitivity == "" {
		correction.Sensitivity = old.Sensitivity
	}
	if correction.VerificationStatus == "" {
		correction.VerificationStatus = VerificationNeedsReview
	}
	correction.ProjectKeys = unionStrings(old.ProjectKeys, correction.ProjectKeys)
	correction.Tags = unionStrings(old.Tags, correction.Tags)
	correction.Sources = mergeSources(old.Sources, correction.Sources)
	correction.LocalOnly = old.LocalOnly || correction.LocalOnly

	now := s.clock().UTC()
	replacement := nodeFromRequest(correction, now)
	replacement.SupersedesID = old.ID
	if err := validateNode(replacement); err != nil {
		return NodeWriteResult{}, err
	}
	if err := s.validateSourceReferences(ctx, replacement.OwnerIdentity, replacement.Sources); err != nil {
		return NodeWriteResult{}, err
	}

	created, err := s.repo.CreateNode(ctx, replacement)
	if err != nil {
		return NodeWriteResult{}, err
	}
	old.Archived = true
	old.CorrectedByID = created.ID
	old.UpdatedAt = now
	if _, err := s.repo.UpdateNode(ctx, old); err != nil {
		return NodeWriteResult{}, err
	}
	return NodeWriteResult{Node: created, Action: WriteCorrected}, nil
}

func (s *Service) DeleteNode(ctx context.Context, ownerIdentity, id, reason string) (DeletionSignal, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return DeletionSignal{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return DeletionSignal{}, fmt.Errorf("deletion reason is required")
	}
	return s.repo.DeleteNode(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(id), strings.TrimSpace(reason), s.clock().UTC())
}

func (s *Service) DeleteEdge(ctx context.Context, ownerIdentity, id, reason string) (DeletionSignal, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return DeletionSignal{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return DeletionSignal{}, fmt.Errorf("deletion reason is required")
	}
	return s.repo.DeleteEdge(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(id), strings.TrimSpace(reason), s.clock().UTC())
}

func (s *Service) ListDeletionSignals(ctx context.Context, ownerIdentity string) ([]DeletionSignal, error) {
	if err := requireOwner(ownerIdentity); err != nil {
		return nil, err
	}
	return s.repo.ListDeletionSignals(ctx, strings.TrimSpace(ownerIdentity))
}

func (s *Service) validateSourceReferences(ctx context.Context, ownerIdentity string, references []SourceReference) error {
	for _, reference := range references {
		if strings.TrimSpace(reference.SourceNodeID) == "" {
			continue
		}
		source, err := s.repo.GetNode(ctx, ownerIdentity, strings.TrimSpace(reference.SourceNodeID))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("source node %q is not available to owner", reference.SourceNodeID)
			}
			return err
		}
		if source.Kind != NodeSource && source.Kind != NodeDocument {
			return fmt.Errorf("source node %q must be a source or document", reference.SourceNodeID)
		}
	}
	return nil
}

func nodeFromRequest(request CreateNodeRequest, now time.Time) Node {
	owner := strings.TrimSpace(request.OwnerIdentity)
	label := compact(request.Label)
	content := strings.TrimSpace(request.Content)
	kind := request.Kind
	verification := request.VerificationStatus
	if verification == "" {
		verification = VerificationUnverified
	}
	sensitivity := request.Sensitivity
	if sensitivity == "" {
		sensitivity = SensitivityInternal
	}
	projectKeys := normalizeStrings(request.ProjectKeys)
	dedupKey := compact(request.DeduplicationKey)
	if dedupKey == "" {
		dedupKey = defaultDeduplicationKey(kind, label, projectKeys)
	}
	return Node{
		OwnerIdentity:      owner,
		Kind:               kind,
		DeduplicationKey:   dedupKey,
		Label:              label,
		Content:            content,
		Properties:         normalizeMap(request.Properties),
		ProjectKeys:        projectKeys,
		Tags:               normalizeStrings(request.Tags),
		Confidence:         request.Confidence,
		VerificationStatus: verification,
		Sources:            normalizeSources(request.Sources),
		ValidFrom:          cloneTime(request.ValidFrom),
		ValidUntil:         cloneTime(request.ValidUntil),
		Sensitivity:        sensitivity,
		LocalOnly:          request.LocalOnly || hasLocalOnlySource(request.Sources),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func edgeFromRequest(request CreateEdgeRequest, now time.Time) Edge {
	verification := request.VerificationStatus
	if verification == "" {
		verification = VerificationUnverified
	}
	sensitivity := request.Sensitivity
	if sensitivity == "" {
		sensitivity = SensitivityInternal
	}
	return Edge{
		OwnerIdentity:      strings.TrimSpace(request.OwnerIdentity),
		FromNodeID:         strings.TrimSpace(request.FromNodeID),
		ToNodeID:           strings.TrimSpace(request.ToNodeID),
		Relationship:       request.Relationship,
		Label:              compact(request.Label),
		Properties:         normalizeMap(request.Properties),
		ProjectKeys:        normalizeStrings(request.ProjectKeys),
		Confidence:         request.Confidence,
		VerificationStatus: verification,
		Sources:            normalizeSources(request.Sources),
		ValidFrom:          cloneTime(request.ValidFrom),
		ValidUntil:         cloneTime(request.ValidUntil),
		Sensitivity:        sensitivity,
		LocalOnly:          request.LocalOnly || hasLocalOnlySource(request.Sources),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func nodesCompatible(left, right Node) bool {
	if normalizedValue(left.Content) != "" && normalizedValue(right.Content) != "" &&
		normalizedValue(left.Content) != normalizedValue(right.Content) {
		return false
	}
	return mapsCompatible(left.Properties, right.Properties)
}

func edgesCompatible(left, right Edge) bool {
	return mapsCompatible(left.Properties, right.Properties)
}

func mapsCompatible(left, right map[string]string) bool {
	for key, leftValue := range left {
		rightValue, exists := right[key]
		if exists && normalizedValue(leftValue) != normalizedValue(rightValue) {
			return false
		}
	}
	return true
}

func mergeNodes(existing, incoming Node, now time.Time) Node {
	if existing.Content == "" {
		existing.Content = incoming.Content
	}
	existing.Properties = mergeMaps(existing.Properties, incoming.Properties)
	existing.ProjectKeys = unionStrings(existing.ProjectKeys, incoming.ProjectKeys)
	existing.Tags = unionStrings(existing.Tags, incoming.Tags)
	existing.Sources = mergeSources(existing.Sources, incoming.Sources)
	existing.Confidence = max(existing.Confidence, incoming.Confidence)
	existing.VerificationStatus = strongerVerification(existing.VerificationStatus, incoming.VerificationStatus)
	existing.Sensitivity = strongerSensitivity(existing.Sensitivity, incoming.Sensitivity)
	existing.LocalOnly = existing.LocalOnly || incoming.LocalOnly || hasLocalOnlySource(existing.Sources)
	existing.ValidFrom = earliestTime(existing.ValidFrom, incoming.ValidFrom)
	existing.ValidUntil = latestTime(existing.ValidUntil, incoming.ValidUntil)
	existing.UpdatedAt = now
	return existing
}

func mergeEdges(existing, incoming Edge, now time.Time) Edge {
	existing.Properties = mergeMaps(existing.Properties, incoming.Properties)
	existing.ProjectKeys = unionStrings(existing.ProjectKeys, incoming.ProjectKeys)
	existing.Sources = mergeSources(existing.Sources, incoming.Sources)
	existing.Confidence = max(existing.Confidence, incoming.Confidence)
	existing.VerificationStatus = strongerVerification(existing.VerificationStatus, incoming.VerificationStatus)
	existing.Sensitivity = strongerSensitivity(existing.Sensitivity, incoming.Sensitivity)
	existing.LocalOnly = existing.LocalOnly || incoming.LocalOnly || hasLocalOnlySource(existing.Sources)
	existing.ValidFrom = earliestTime(existing.ValidFrom, incoming.ValidFrom)
	existing.ValidUntil = latestTime(existing.ValidUntil, incoming.ValidUntil)
	existing.UpdatedAt = now
	return existing
}

func defaultDeduplicationKey(kind NodeKind, label string, projectKeys []string) string {
	return strings.Join([]string{string(kind), normalizedValue(label), strings.Join(projectKeys, ",")}, "|")
}

func conflictGroupID(owner string, kind NodeKind, key string) string {
	sum := sha256.Sum256([]byte(owner + "|" + string(kind) + "|" + key))
	return "conflict-" + hex.EncodeToString(sum[:8])
}

func compact(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizedValue(value string) string {
	return strings.ToLower(compact(value))
}

func normalizeMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = normalizedValue(key)
		if key != "" {
			result[key] = compact(value)
		}
	}
	return result
}

func normalizeStrings(values []string) []string {
	set := make(map[string]string)
	for _, value := range values {
		normalized := normalizedValue(value)
		if normalized != "" {
			set[normalized] = compact(value)
		}
	}
	result := make([]string, 0, len(set))
	for normalized := range set {
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func normalizeSources(values []SourceReference) []SourceReference {
	result := append([]SourceReference(nil), values...)
	for i := range result {
		result[i].ID = compact(result[i].ID)
		result[i].URI = strings.TrimSpace(result[i].URI)
		result[i].Label = compact(result[i].Label)
		result[i].SourceNodeID = compact(result[i].SourceNodeID)
		result[i].Authority = compact(result[i].Authority)
	}
	sort.Slice(result, func(i, j int) bool { return sourceIdentity(result[i]) < sourceIdentity(result[j]) })
	return result
}

func mergeMaps(left, right map[string]string) map[string]string {
	result := cloneMap(left)
	if result == nil && len(right) > 0 {
		result = make(map[string]string, len(right))
	}
	for key, value := range right {
		if _, exists := result[key]; !exists {
			result[key] = value
		}
	}
	return result
}

func unionStrings(left, right []string) []string {
	return normalizeStrings(append(append([]string(nil), left...), right...))
}

func mergeSources(left, right []SourceReference) []SourceReference {
	combined := append(append([]SourceReference(nil), left...), right...)
	byIdentity := make(map[string]SourceReference)
	for _, source := range combined {
		key := sourceIdentity(source)
		if key != "||" {
			byIdentity[key] = source
		}
	}
	result := make([]SourceReference, 0, len(byIdentity))
	for _, source := range byIdentity {
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return sourceIdentity(result[i]) < sourceIdentity(result[j]) })
	return result
}

func sourceIdentity(source SourceReference) string {
	return strings.Join([]string{
		normalizedValue(source.ID),
		strings.TrimSpace(source.URI),
		normalizedValue(source.SourceNodeID),
	}, "|")
}

func hasLocalOnlySource(sources []SourceReference) bool {
	for _, source := range sources {
		if source.LocalOnly {
			return true
		}
	}
	return false
}

func strongerVerification(left, right VerificationStatus) VerificationStatus {
	if left == VerificationConflicting || right == VerificationConflicting {
		return VerificationConflicting
	}
	rank := map[VerificationStatus]int{
		VerificationUnsupported:     0,
		VerificationUnverified:      1,
		VerificationUncertain:       2,
		VerificationNeedsReview:     3,
		VerificationSchemaValidated: 4,
		VerificationSourceSupported: 5,
		VerificationTestPassed:      6,
		VerificationHumanApproved:   7,
		VerificationVerified:        8,
	}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func strongerSensitivity(left, right Sensitivity) Sensitivity {
	rank := map[Sensitivity]int{
		SensitivityPublic: 0, SensitivityInternal: 1,
		SensitivitySensitive: 2, SensitivityRestricted: 3,
	}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func earliestTime(left, right *time.Time) *time.Time {
	if left == nil {
		return cloneTime(right)
	}
	if right == nil || left.Before(*right) {
		return cloneTime(left)
	}
	return cloneTime(right)
}

func latestTime(left, right *time.Time) *time.Time {
	if left == nil || right == nil {
		return nil
	}
	if left.After(*right) {
		return cloneTime(left)
	}
	return cloneTime(right)
}

func requireOwner(ownerIdentity string) error {
	if strings.TrimSpace(ownerIdentity) == "" {
		return fmt.Errorf("owner identity is required")
	}
	return nil
}
