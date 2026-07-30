package agentregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type Clock func() time.Time

type Service struct {
	repository Repository
	clock      Clock
}

func NewService(repository Repository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("agent repository is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repository: repository, clock: clock}, nil
}

func (s *Service) Register(ctx context.Context, agent Agent) (Agent, error) {
	now := s.clock().UTC()
	agent.ContractVersion = ContractVersion
	agent.OwnerIdentity = strings.TrimSpace(agent.OwnerIdentity)
	agent.ID = strings.TrimSpace(agent.ID)
	agent.Name = strings.TrimSpace(agent.Name)
	agent.State = StateRegistered
	agent.Revision = 1
	agent.CreatedAt = now
	agent.UpdatedAt = now
	normalizeAgent(&agent)
	if err := ValidateAgent(agent, now); err != nil {
		return Agent{}, err
	}
	return s.repository.Create(ctx, agent)
}

func (s *Service) Get(ctx context.Context, owner, id string) (Agent, error) {
	if err := validateIdentity(owner); err != nil {
		return Agent{}, err
	}
	if err := validateIdentifier("agent id", id); err != nil {
		return Agent{}, err
	}
	return s.repository.Get(ctx, strings.TrimSpace(owner), strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, owner string) ([]Agent, error) {
	if err := validateIdentity(owner); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, strings.TrimSpace(owner))
}

func (s *Service) Update(ctx context.Context, owner string, replacement Agent, expectedRevision uint64) (Agent, error) {
	current, err := s.Get(ctx, owner, replacement.ID)
	if err != nil {
		return Agent{}, err
	}
	if current.Revision != expectedRevision {
		return Agent{}, ErrConflict
	}
	// Ownership, lifecycle and accumulated evidence are controlled by dedicated
	// operations and cannot be replaced through ordinary configuration updates.
	replacement.ContractVersion = ContractVersion
	replacement.OwnerIdentity = current.OwnerIdentity
	replacement.ID = current.ID
	replacement.State = current.State
	replacement.Reliability = current.Reliability
	// Capacity consumption is execution evidence, not user-editable
	// configuration. Preserve it while still allowing an owner to change the
	// availability switch and maximum capacity.
	replacement.Availability.ActiveAssignments = current.Availability.ActiveAssignments
	replacement.Revision = current.Revision + 1
	replacement.CreatedAt = current.CreatedAt
	replacement.UpdatedAt = s.clock().UTC()
	normalizeAgent(&replacement)
	if err := ValidateAgent(replacement, replacement.UpdatedAt); err != nil {
		return Agent{}, err
	}
	return s.repository.CompareAndSwap(ctx, replacement, expectedRevision)
}

func (s *Service) Transition(
	ctx context.Context,
	owner, id string,
	expectedRevision uint64,
	to LifecycleState,
	reason string,
) (Agent, error) {
	current, err := s.Get(ctx, owner, id)
	if err != nil {
		return Agent{}, err
	}
	if current.Revision != expectedRevision {
		return Agent{}, ErrConflict
	}
	if err := validateText("transition reason", reason, true); err != nil {
		return Agent{}, err
	}
	if !transitionAllowed(current.State, to) {
		return Agent{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current.State, to)
	}
	if to == StateEnabled && !agentReady(current, s.clock().UTC(), false) {
		return Agent{}, fmt.Errorf("%w: agent must be healthy, ready, fresh, and available before enablement", ErrInvalidTransition)
	}
	now := s.clock().UTC()
	updated := cloneAgent(current)
	updated.State = to
	updated.Revision++
	updated.UpdatedAt = now
	event := Transition{
		From: current.State, To: to, Reason: strings.TrimSpace(reason),
		OccurredAt: now, Revision: updated.Revision,
	}
	return s.repository.Transition(ctx, updated, expectedRevision, event)
}

func (s *Service) ListTransitions(ctx context.Context, owner, id string) ([]Transition, error) {
	if _, err := s.Get(ctx, owner, id); err != nil {
		return nil, err
	}
	return s.repository.ListTransitions(ctx, owner, id)
}

func (s *Service) Assign(ctx context.Context, request AssignmentRequest) (Assignment, error) {
	if err := ValidateAssignmentRequest(request); err != nil {
		return Assignment{}, err
	}
	now := s.clock().UTC()
	requestDigest, err := digestValue(request)
	if err != nil {
		return Assignment{}, fmt.Errorf("digest assignment request: %w", err)
	}
	// The assignment identity is based on the requested work, not on the
	// selected agent's mutable revision. This makes retried requests
	// idempotent and prevents a client retry from reserving another slot.
	idInput := strings.Join([]string{
		request.OwnerIdentity,
		request.TaskID,
		requestDigest,
	}, "\x00")
	idHash := sha256.Sum256([]byte(idInput))
	assignmentID := hex.EncodeToString(idHash[:16])
	if existing, err := s.repository.GetAssignment(
		ctx,
		request.OwnerIdentity,
		assignmentID,
	); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Assignment{}, err
	}

	agents, err := s.repository.List(ctx, request.OwnerIdentity)
	if err != nil {
		return Assignment{}, err
	}
	type candidate struct {
		agent       Agent
		score       float64
		explanation AssignmentExplanation
	}
	candidates := make([]candidate, 0, len(agents))
	for _, agent := range agents {
		score, explanation := scoreAgent(agent, request, now)
		if explanation.Eligible {
			candidates = append(candidates, candidate{agent: agent, score: score, explanation: explanation})
		}
	}
	if len(candidates) == 0 {
		return Assignment{}, ErrNoEligibleAgent
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].agent.ID < candidates[j].agent.ID
		}
		return candidates[i].score > candidates[j].score
	})
	selected := candidates[0]
	assignment := Assignment{
		ID:               assignmentID,
		OwnerIdentity:    request.OwnerIdentity,
		TaskID:           request.TaskID,
		AgentID:          selected.agent.ID,
		AgentRevision:    selected.agent.Revision,
		GrantedAuthority: request.RequiredAuthority,
		GrantedAutonomy:  request.RequiredAutonomy,
		Score:            roundScore(selected.score),
		Explanation:      selected.explanation,
		RequestDigest:    requestDigest,
		AssignedAt:       now,
	}
	if _, err := s.repository.CreateAssignment(ctx, assignment); err != nil {
		if errors.Is(err, ErrAssignmentExists) {
			return s.repository.GetAssignment(ctx, request.OwnerIdentity, assignment.ID)
		}
		return Assignment{}, err
	}
	return cloneAssignment(assignment), nil
}

func (s *Service) GetAssignment(ctx context.Context, owner, id string) (Assignment, error) {
	if err := validateIdentity(owner); err != nil {
		return Assignment{}, err
	}
	if err := validateIdentifier("assignment id", id); err != nil {
		return Assignment{}, err
	}
	return s.repository.GetAssignment(ctx, owner, id)
}

func (s *Service) RecordAssignmentOutcome(
	ctx context.Context,
	owner, assignmentID string,
	expectedRevision uint64,
	outcome Outcome,
) (Agent, error) {
	assignment, err := s.GetAssignment(ctx, owner, assignmentID)
	if err != nil {
		return Agent{}, err
	}
	current, err := s.Get(ctx, owner, assignment.AgentID)
	if err != nil {
		return Agent{}, err
	}
	if current.Revision != expectedRevision {
		return Agent{}, ErrConflict
	}
	if outcome.Latency < 0 {
		return Agent{}, fmt.Errorf("outcome latency cannot be negative")
	}
	now := s.clock().UTC()
	if outcome.RecordedAt.IsZero() {
		outcome.RecordedAt = now
	}
	if outcome.RecordedAt.After(now.Add(time.Minute)) {
		return Agent{}, fmt.Errorf("outcome timestamp cannot be in the future")
	}
	updated := cloneAgent(current)
	if updated.Availability.ActiveAssignments <= 0 {
		return Agent{}, ErrConflict
	}
	updated.Availability.ActiveAssignments--
	evidence := updated.Reliability
	totalBefore := evidence.Successes + evidence.Failures
	latencyMs := float64(outcome.Latency.Milliseconds())
	if totalBefore == 0 {
		evidence.MeanLatencyMs = latencyMs
	} else {
		evidence.MeanLatencyMs = ((evidence.MeanLatencyMs * float64(totalBefore)) + latencyMs) / float64(totalBefore+1)
	}
	if outcome.Success {
		evidence.Successes++
		evidence.ConsecutiveFailures = 0
	} else {
		evidence.Failures++
		evidence.ConsecutiveFailures++
	}
	evidence.LastOutcomeAt = outcome.RecordedAt.UTC()
	updated.Reliability = evidence
	updated.Revision++
	updated.UpdatedAt = now
	if err := ValidateAgent(updated, now); err != nil {
		return Agent{}, err
	}
	return s.repository.RecordAssignmentOutcome(ctx, AssignmentOutcome{
		AssignmentID:  assignment.ID,
		OwnerIdentity: assignment.OwnerIdentity,
		AgentID:       assignment.AgentID,
		Success:       outcome.Success,
		Latency:       outcome.Latency,
		RecordedAt:    outcome.RecordedAt.UTC(),
	}, updated, expectedRevision)
}

func scoreAgent(agent Agent, request AssignmentRequest, now time.Time) (float64, AssignmentExplanation) {
	explanation := AssignmentExplanation{Eligible: false, Constraints: []string{}}
	reject := func(reason string) (float64, AssignmentExplanation) {
		explanation.RejectedReason = reason
		explanation.Constraints = append(explanation.Constraints, reason)
		return 0, explanation
	}
	if agent.State != StateEnabled {
		return reject("agent lifecycle state is not enabled")
	}
	if !agentReady(agent, now, request.AllowDegraded) {
		return reject("agent is not healthy, ready, fresh, and available")
	}
	if len(request.AllowedAgentTypes) > 0 && !containsAgentType(request.AllowedAgentTypes, agent.Type) {
		return reject("agent type is not permitted")
	}
	if request.RequiredAuthority > agent.AuthorityCeiling {
		return reject("required authority exceeds agent ceiling")
	}
	if request.RequiredAutonomy > agent.AutonomyCeiling {
		return reject("required autonomy exceeds agent ceiling")
	}
	if request.RequiredAuthority > request.PolicyMaxAuthority || request.RequiredAutonomy > request.PolicyMaxAutonomy {
		return reject("assignment exceeds policy ceiling")
	}
	if !compatibilityMatches(agent, request.Compatibility) {
		return reject("runtime compatibility requirement is not met")
	}
	if !capabilitiesMatch(agent.Capabilities, request.Capabilities) {
		return reject("capability or version requirement is not met")
	}
	if !containsAll(agent.ToolAllowlist, request.RequiredTools) {
		return reject("required tool is not allowlisted")
	}
	if !containsAll(agent.DataAllowlist, request.RequiredData) {
		return reject("required data scope is not allowlisted")
	}
	if !containsAllFolders(agent.FolderAllowlist, request.RequiredFolders) {
		return reject("required folder is not allowlisted")
	}
	if request.RequireLocal && agent.Performance.Locality != LocalityLocal {
		return reject("local execution is required")
	}
	if request.MaxEstimatedCostEUR != nil && agent.Performance.EstimatedCostEUR > *request.MaxEstimatedCostEUR {
		return reject("estimated cost exceeds policy maximum")
	}

	healthScore := 15.0
	if agent.Health.Status == HealthDegraded {
		healthScore = 8
	}
	reliabilityScore := 15 * agent.Reliability.Score()
	loadFraction := float64(agent.Availability.ActiveAssignments) / float64(agent.Availability.MaxConcurrent)
	loadScore := 10 * (1 - loadFraction)
	authorityScore := math.Max(0, 10-float64(agent.AuthorityCeiling-request.RequiredAuthority)*0.5)
	costScore := 5.0
	if request.MaxEstimatedCostEUR != nil && *request.MaxEstimatedCostEUR > 0 {
		costScore = 5 * (1 - agent.Performance.EstimatedCostEUR / *request.MaxEstimatedCostEUR)
	}
	localityScore := map[Locality]float64{LocalityLocal: 10, LocalityLAN: 6, LocalityCloud: 3}[agent.Performance.Locality]
	components := []ScoreComponent{
		{Name: "capability", Score: 25, Reason: "all required capabilities, operations, and versions match"},
		{Name: "authority", Score: roundScore(authorityScore), Reason: "required authority and autonomy fit agent and policy ceilings"},
		{Name: "health", Score: roundScore(healthScore), Reason: "health and readiness checks passed"},
		{Name: "reliability", Score: roundScore(reliabilityScore), Reason: fmt.Sprintf("bounded evidence score %.4f", agent.Reliability.Score())},
		{Name: "load", Score: roundScore(loadScore), Reason: fmt.Sprintf("%d of %d assignment slots active", agent.Availability.ActiveAssignments, agent.Availability.MaxConcurrent)},
		{Name: "cost", Score: roundScore(costScore), Reason: fmt.Sprintf("estimated cost EUR %.6f", agent.Performance.EstimatedCostEUR)},
		{Name: "locality", Score: localityScore, Reason: fmt.Sprintf("%s execution locality", agent.Performance.Locality)},
	}
	total := 0.0
	for _, component := range components {
		total += component.Score
	}
	explanation.Eligible = true
	explanation.Components = components
	explanation.Constraints = []string{
		"lifecycle enabled",
		"health fresh and ready",
		"capabilities compatible",
		"allowlists satisfied",
		"authority and policy ceilings satisfied",
	}
	return roundScore(total / 90), explanation
}

func agentReady(agent Agent, now time.Time, allowDegraded bool) bool {
	if !agent.Health.Ready || !agent.Availability.Available ||
		agent.Availability.ActiveAssignments >= agent.Availability.MaxConcurrent {
		return false
	}
	if agent.Health.CheckedAt.IsZero() || agent.Health.FreshFor <= 0 ||
		now.After(agent.Health.CheckedAt.Add(agent.Health.FreshFor)) {
		return false
	}
	if agent.Health.Status == HealthHealthy {
		return true
	}
	return allowDegraded && agent.Health.Status == HealthDegraded
}

func transitionAllowed(from, to LifecycleState) bool {
	switch from {
	case StateRegistered:
		return to == StateEnabled || to == StateDisabled || to == StateQuarantined
	case StateEnabled:
		return to == StateDraining || to == StateDisabled || to == StateQuarantined
	case StateDraining:
		return to == StateEnabled || to == StateDisabled || to == StateQuarantined
	case StateDisabled:
		return to == StateEnabled || to == StateQuarantined
	case StateQuarantined:
		return to == StateDisabled
	default:
		return false
	}
}

func compatibilityMatches(agent Agent, required CompatibilityRequirement) bool {
	if required.RuntimeAdapterID != "" && !strings.EqualFold(required.RuntimeAdapterID, agent.Runtime.ID) {
		return false
	}
	if required.RuntimeType != "" && !strings.EqualFold(required.RuntimeType, agent.Runtime.Type) {
		return false
	}
	version, err := parseVersion(agent.Runtime.ProtocolVersion)
	if err != nil {
		return false
	}
	return versionInRange(version, required.MinProtocolVersion, required.MaxProtocolVersion)
}

func capabilitiesMatch(available []CapabilityDeclaration, required []CapabilityRequirement) bool {
	byID := make(map[string]CapabilityDeclaration, len(available))
	for _, capability := range available {
		byID[strings.ToLower(capability.ID)] = capability
	}
	for _, requirement := range required {
		capability, exists := byID[strings.ToLower(requirement.ID)]
		if !exists {
			return false
		}
		version, err := parseVersion(capability.Version)
		if err != nil || !versionInRange(version, requirement.MinVersion, requirement.MaxVersion) ||
			!containsAll(capability.Operations, requirement.Operations) {
			return false
		}
	}
	return true
}

func versionInRange(version semanticVersion, minimum, maximum string) bool {
	if minimum != "" {
		min, err := parseVersion(minimum)
		if err != nil || compareVersions(version, min) < 0 {
			return false
		}
	}
	if maximum != "" {
		max, err := parseVersion(maximum)
		if err != nil || compareVersions(version, max) > 0 {
			return false
		}
	}
	return true
}

func containsAll(available, required []string) bool {
	values := map[string]struct{}{}
	for _, value := range available {
		values[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range required {
		if _, exists := values[strings.ToLower(strings.TrimSpace(value))]; !exists {
			return false
		}
	}
	return true
}

func containsAllFolders(available, required []string) bool {
	normalized := make([]string, 0, len(available))
	for _, value := range available {
		normalized = append(normalized, normalizeFolder(value))
	}
	for _, requested := range required {
		requested = normalizeFolder(requested)
		matched := false
		for _, allowed := range normalized {
			if requested == allowed || strings.HasPrefix(requested, allowed+"/") {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func normalizeFolder(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")), "/")
}

func containsAgentType(values []AgentType, target AgentType) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeAgent(agent *Agent) {
	agent.ToolAllowlist = normalizeUnique(agent.ToolAllowlist)
	agent.DataAllowlist = normalizeUnique(agent.DataAllowlist)
	agent.FolderAllowlist = normalizeUnique(agent.FolderAllowlist)
	sort.Slice(agent.Capabilities, func(i, j int) bool {
		return strings.ToLower(agent.Capabilities[i].ID) < strings.ToLower(agent.Capabilities[j].ID)
	})
	for index := range agent.Capabilities {
		agent.Capabilities[index].Operations = normalizeUnique(agent.Capabilities[index].Operations)
	}
}

func digestValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func roundScore(value float64) float64 {
	return math.Round(value*10000) / 10000
}
