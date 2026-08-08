package frameworkregistry

import (
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/agentcoordination"

	"github.com/google/uuid"
)

type AgentTeamService struct {
	repo  AgentTeamRepository
	now   func() time.Time
	newID func() string
}

func NewAgentTeamService(repo AgentTeamRepository) *AgentTeamService {
	return newAgentTeamService(repo, time.Now, uuid.NewString)
}

func newAgentTeamService(repo AgentTeamRepository, now func() time.Time, newID func() string) *AgentTeamService {
	if repo == nil {
		repo = NewMemoryAgentTeamRepository()
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &AgentTeamService{repo: repo, now: now, newID: newID}
}

func (s *AgentTeamService) CreateTeam(owner string, request CreateAgentTeamRequest) (*AgentTeamContract, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	actor, reason, err := normalizeTeamActorReason(request.Actor, "Register advisory agent team contract.")
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	team := AgentTeamContract{
		ID:                             s.newID(),
		Key:                            request.Key,
		Version:                        request.Version,
		Revision:                       1,
		Status:                         AgentTeamDraft,
		Name:                           request.Name,
		Purpose:                        request.Purpose,
		AuthorityCeiling:               request.AuthorityCeiling,
		RiskCeiling:                    request.RiskCeiling,
		MaximumDelegatedAuthority:      request.MaximumDelegatedAuthority,
		MaximumDelegatedRisk:           request.MaximumDelegatedRisk,
		AdvisoryOnly:                   true,
		GrantsExecutionAuthority:       false,
		ExecutionAuthorizationRequired: true,
		Roles:                          request.Roles,
		Capabilities:                   request.Capabilities,
		Members:                        []TeamMembership{},
		CoordinationPolicy:             request.CoordinationPolicy,
		Consensus:                      request.Consensus,
		EvidenceRefs:                   request.EvidenceRefs,
		Provenance:                     request.Provenance,
		CreatedAt:                      now,
		UpdatedAt:                      now,
	}
	team.Provenance.RegisteredBy = actor
	team.Provenance.RegisteredAt = now
	team, err = normalizeAgentTeamContract(team, now)
	if err != nil {
		return nil, err
	}
	event, err := s.newTeamEvent(team, TeamEventCreated, actor, "", reason, team.EvidenceRefs, "")
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateTeam(owner, team, event); err != nil {
		return nil, err
	}
	result := cloneAgentTeam(team)
	return &result, nil
}

func (s *AgentTeamService) CreateTeamVersion(owner, teamID string, request CreateAgentTeamVersionRequest) (*AgentTeamContract, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	actor, reason, err := normalizeTeamActorReason(request.Actor, "Register new advisory agent team version.")
	if err != nil {
		return nil, err
	}
	previous, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(request.PreviousVersion))
	if err != nil {
		return nil, err
	}
	if previous.Status == AgentTeamRevoked {
		return nil, fmt.Errorf("revoked team identity cannot create a new version")
	}
	if strings.ToLower(strings.TrimSpace(request.ExpectedPreviousDigest)) != previous.ContractDigest {
		return nil, fmt.Errorf("previous team version digest precondition failed")
	}
	if compareSemanticVersions(strings.TrimSpace(request.Version), previous.Version) <= 0 {
		return nil, fmt.Errorf("new team version must be newer than %s", previous.Version)
	}
	now := s.now().UTC()
	team := AgentTeamContract{
		ID:                             previous.ID,
		Key:                            previous.Key,
		Version:                        request.Version,
		Revision:                       1,
		Status:                         AgentTeamDraft,
		Name:                           request.Name,
		Purpose:                        request.Purpose,
		AuthorityCeiling:               request.AuthorityCeiling,
		RiskCeiling:                    request.RiskCeiling,
		MaximumDelegatedAuthority:      request.MaximumDelegatedAuthority,
		MaximumDelegatedRisk:           request.MaximumDelegatedRisk,
		AdvisoryOnly:                   true,
		GrantsExecutionAuthority:       false,
		ExecutionAuthorizationRequired: true,
		Roles:                          request.Roles,
		Capabilities:                   request.Capabilities,
		Members:                        []TeamMembership{},
		CoordinationPolicy:             request.CoordinationPolicy,
		Consensus:                      request.Consensus,
		EvidenceRefs:                   request.EvidenceRefs,
		Provenance:                     request.Provenance,
		PreviousVersionDigest:          previous.ContractDigest,
		CreatedAt:                      now,
		UpdatedAt:                      now,
	}
	team.Provenance.RegisteredBy = actor
	team.Provenance.RegisteredAt = now
	team, err = normalizeAgentTeamContract(team, now)
	if err != nil {
		return nil, err
	}
	event, err := s.newTeamEvent(team, TeamEventVersionCreated, actor, previous.Version, reason, team.EvidenceRefs, "")
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateTeam(owner, team, event); err != nil {
		return nil, err
	}
	result := cloneAgentTeam(team)
	return &result, nil
}

func (s *AgentTeamService) GetTeam(owner, teamID, version string) (*AgentTeamContract, error) {
	team, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version))
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *AgentTeamService) ListTeams(owner string) ([]AgentTeamContract, error) {
	return s.repo.ListTeams(owner)
}

func (s *AgentTeamService) ListTeamVersions(owner, teamID string) ([]AgentTeamContract, error) {
	return s.repo.ListTeamVersions(owner, strings.TrimSpace(teamID))
}

func (s *AgentTeamService) ActivateTeam(owner, teamID, version string, request TeamTransitionRequest) (*AgentTeamContract, error) {
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventActivated, "", request.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		if team.Status != AgentTeamDraft && team.Status != AgentTeamSuspended {
			return fmt.Errorf("only draft or suspended teams can be activated")
		}
		if activeTeamVoterCount(*team) < team.Consensus.Quorum {
			return fmt.Errorf("team cannot activate without quorum of active voting members")
		}
		team.Status = AgentTeamActive
		team.ActivatedAt = agentTeamTimePointer(now)
		team.SuspendedAt = nil
		return nil
	})
}

func (s *AgentTeamService) SuspendTeam(owner, teamID, version string, request TeamTransitionRequest) (*AgentTeamContract, error) {
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventSuspended, "", request.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		if team.Status != AgentTeamActive {
			return fmt.Errorf("only active teams can be suspended")
		}
		team.Status = AgentTeamSuspended
		team.SuspendedAt = agentTeamTimePointer(now)
		return nil
	})
}

func (s *AgentTeamService) RetireTeam(owner, teamID, version string, request TeamTransitionRequest) (*AgentTeamContract, error) {
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventRetired, "", request.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		if team.Status == AgentTeamRetired || team.Status == AgentTeamRevoked {
			return fmt.Errorf("terminal team cannot be retired")
		}
		team.Status = AgentTeamRetired
		team.RetiredAt = agentTeamTimePointer(now)
		return nil
	})
}

func (s *AgentTeamService) RevokeTeam(owner, teamID, version string, request TeamTransitionRequest) (*AgentTeamContract, error) {
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventRevoked, "", request.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		if team.Status == AgentTeamRevoked {
			return fmt.Errorf("team is already revoked")
		}
		team.Status = AgentTeamRevoked
		team.RevokedAt = agentTeamTimePointer(now)
		team.RevocationReason = request.Reason
		for index := range team.Members {
			member := &team.Members[index]
			if member.Status == TeamMemberLeft || member.Status == TeamMemberRevoked {
				continue
			}
			member.Status = TeamMemberRevoked
			member.StatusChangedAt = now
			member.RevokedAt = agentTeamTimePointer(now)
			member.RevocationReason = "team revoked: " + request.Reason
		}
		return nil
	})
}

func (s *AgentTeamService) AddMember(owner, teamID, version string, request AddTeamMemberRequest) (*AgentTeamContract, error) {
	if strings.TrimSpace(request.Member.ID) == "" {
		request.Member.ID = s.newID()
	}
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventMemberAdded, request.Member.ID, request.Member.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		if team.Status == AgentTeamRetired || team.Status == AgentTeamRevoked {
			return fmt.Errorf("terminal team cannot accept members")
		}
		if request.Member.Status == "" {
			request.Member.Status = TeamMemberInvited
		}
		request.Member.StatusChangedAt = now
		if request.Member.Status == TeamMemberActive && request.Member.JoinedAt == nil {
			request.Member.JoinedAt = agentTeamTimePointer(now)
		}
		team.Members = append(team.Members, request.Member)
		return nil
	})
}

func (s *AgentTeamService) ChangeMembership(owner, teamID, version, membershipID string, request ChangeTeamMembershipRequest) (*AgentTeamContract, error) {
	membershipID = strings.TrimSpace(membershipID)
	return s.mutateTeam(owner, teamID, version, request.ExpectedRevision, request.Actor, request.Reason, TeamEventMembershipChanged, membershipID, request.EvidenceRefs, func(team *AgentTeamContract, now time.Time) error {
		index := findTeamMember(*team, membershipID)
		if index < 0 {
			return fmt.Errorf("team membership not found")
		}
		member := &team.Members[index]
		next := normalizeIdentifier(request.Status)
		if !validMembershipTransition(member.Status, next) {
			return fmt.Errorf("invalid membership transition from %s to %s", member.Status, next)
		}
		member.Status = next
		member.StatusChangedAt = now
		member.EvidenceRefs = append(member.EvidenceRefs, request.EvidenceRefs...)
		member.ProvenanceDigest = ""
		if next == TeamMemberActive && member.JoinedAt == nil {
			member.JoinedAt = agentTeamTimePointer(now)
		}
		if next == TeamMemberRevoked {
			member.RevokedAt = agentTeamTimePointer(now)
			member.RevocationReason = request.Reason
		}
		if team.Status == AgentTeamActive && activeTeamVoterCount(*team) < team.Consensus.Quorum {
			return fmt.Errorf("membership change would leave active team below voting quorum; suspend the team first")
		}
		return nil
	})
}

// StoreCoordinationMessage validates and persists the canonical
// agentcoordination envelope. Persistence proves only accepted coordination,
// never task execution.
func (s *AgentTeamService) StoreCoordinationMessage(owner, teamID, version string, message agentcoordination.Message) (*agentcoordination.Message, bool, error) {
	team, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version))
	if err != nil {
		return nil, false, err
	}
	if err := validateTeamCoordinationMessage(team, message, s.now().UTC()); err != nil {
		return nil, false, err
	}
	stored, created, err := s.repo.AppendCoordinationMessage(owner, team.ID, team.Version, message)
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (s *AgentTeamService) CoordinationMessages(owner, teamID, version, correlationID string) ([]agentcoordination.Message, error) {
	return s.repo.ListCoordinationMessages(owner, teamID, version, correlationID)
}

// AcknowledgeCoordinationMessage appends a recipient response to an exact
// persisted message. Acknowledgment is communication state only; it never
// represents task completion, human approval, or execution authorization.
func (s *AgentTeamService) AcknowledgeCoordinationMessage(owner, teamID, version, messageID string, acknowledgment agentcoordination.Acknowledgment) (*agentcoordination.Acknowledgment, bool, error) {
	message, err := s.repo.GetCoordinationMessage(owner, strings.TrimSpace(teamID), strings.TrimSpace(version), strings.TrimSpace(messageID))
	if err != nil {
		return nil, false, err
	}
	if !message.RequiresAck {
		return nil, false, fmt.Errorf("coordination message does not require acknowledgment")
	}
	if err := agentcoordination.ValidateAcknowledgment(message, acknowledgment, s.now().UTC()); err != nil {
		return nil, false, err
	}
	stored, created, err := s.repo.AppendMessageAcknowledgment(owner, teamID, version, acknowledgment)
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (s *AgentTeamService) MessageAcknowledgments(owner, teamID, version, messageID string) ([]agentcoordination.Acknowledgment, error) {
	if _, err := s.repo.GetCoordinationMessage(owner, strings.TrimSpace(teamID), strings.TrimSpace(version), strings.TrimSpace(messageID)); err != nil {
		return nil, err
	}
	return s.repo.ListMessageAcknowledgments(owner, teamID, version, messageID)
}

// MessageAttention derives current response obligations from persisted facts.
// It intentionally has no side effects and routes overdue work to review.
func (s *AgentTeamService) MessageAttention(owner, teamID, version string) (*TeamMessageAttentionPage, error) {
	if _, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version)); err != nil {
		return nil, err
	}
	messages, err := s.repo.ListCoordinationMessages(owner, teamID, version, "")
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := make([]TeamMessageAttention, 0, len(messages))
	for _, message := range messages {
		acknowledgments, err := s.repo.ListMessageAcknowledgments(owner, teamID, version, message.ID)
		if err != nil {
			return nil, err
		}
		var latest *agentcoordination.Acknowledgment
		if len(acknowledgments) > 0 {
			item := cloneAcknowledgment(acknowledgments[len(acknowledgments)-1])
			latest = &item
		}
		attention, err := deriveTeamMessageAttention(message, latest, now)
		if err != nil {
			return nil, err
		}
		result = append(result, attention)
	}
	return &TeamMessageAttentionPage{GeneratedAt: now, Messages: result}, nil
}

func deriveTeamMessageAttention(message agentcoordination.Message, acknowledgment *agentcoordination.Acknowledgment, now time.Time) (TeamMessageAttention, error) {
	ttl := message.ExpiresAt.Sub(message.CreatedAt)
	acknowledgmentTimeout := ttl / 4
	if acknowledgmentTimeout > 15*time.Minute {
		acknowledgmentTimeout = 15 * time.Minute
	}
	if acknowledgmentTimeout < time.Minute {
		acknowledgmentTimeout = time.Minute
	}
	evaluation, err := agentcoordination.EvaluateMessageTimeout(
		message,
		acknowledgment,
		agentcoordination.EscalationPolicy{
			AcknowledgmentTimeout: acknowledgmentTimeout,
			CompletionTimeout:     ttl,
			ReminderInterval:      acknowledgmentTimeout,
			MaximumReminders:      0,
			MaximumEscalations:    1,
			EscalationRecipients:  []string{"owner-review-queue"},
		},
		0,
		0,
		now,
	)
	if err != nil {
		return TeamMessageAttention{}, err
	}
	state := TeamMessageAttentionWaiting
	switch evaluation.Action {
	case agentcoordination.TimeoutExpire:
		state = TeamMessageAttentionExpired
	case agentcoordination.TimeoutEscalate, agentcoordination.TimeoutManualReview:
		state = TeamMessageAttentionOverdue
	}
	if acknowledgment != nil {
		switch acknowledgment.Status {
		case agentcoordination.AcknowledgmentAccepted:
			state = TeamMessageAttentionAcknowledged
		case agentcoordination.AcknowledgmentRejected:
			state = TeamMessageAttentionRejected
		case agentcoordination.AcknowledgmentDeferred:
			if evaluation.Action == agentcoordination.TimeoutNone {
				state = TeamMessageAttentionDeferred
			}
		}
	}
	if !message.RequiresAck {
		state = TeamMessageAttentionNotRequired
	}
	reason := evaluation.Reason
	if state == TeamMessageAttentionAcknowledged {
		reason = "message acknowledged"
	} else if state == TeamMessageAttentionNotRequired {
		reason = "acknowledgment not required"
	}
	var dueAt *time.Time
	if !evaluation.DueAt.IsZero() {
		value := evaluation.DueAt.UTC()
		dueAt = &value
	}
	return TeamMessageAttention{
		MessageID:                      message.ID,
		CorrelationID:                  message.CorrelationID,
		RecipientID:                    message.Recipient.ID,
		Subject:                        message.Payload.Subject,
		RequiresAcknowledgment:         message.RequiresAck,
		State:                          state,
		Reason:                         reason,
		DueAt:                          dueAt,
		ExpiresAt:                      message.ExpiresAt.UTC(),
		LatestAcknowledgment:           acknowledgment,
		HumanReviewRequired:            message.RequiresAck && (state == TeamMessageAttentionRejected || state == TeamMessageAttentionOverdue || state == TeamMessageAttentionExpired),
		AdvisoryOnly:                   true,
		GrantsExecutionAuthority:       false,
		ExecutionAuthorizationRequired: true,
	}, nil
}

// AssessDelegation validates the canonical coordination envelope and team
// ceilings. A successful assessment explicitly still requires independent
// execution authorization.
func (s *AgentTeamService) AssessDelegation(owner, teamID, version, risk string, delegation agentcoordination.DelegationEnvelope) (*TeamDelegationAssessment, error) {
	team, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version))
	if err != nil {
		return nil, err
	}
	if team.Status != AgentTeamActive {
		return nil, fmt.Errorf("team must be active to assess delegation")
	}
	principal, principalOK := activeMemberByAgentID(team, delegation.Principal.ID)
	delegate, delegateOK := activeMemberByAgentID(team, delegation.Delegate.ID)
	if !principalOK || !delegateOK {
		return nil, fmt.Errorf("delegation agents must be active team members")
	}
	if !memberHasRole(principal, delegation.Principal.Role) || !memberHasRole(delegate, delegation.Delegate.Role) {
		return nil, fmt.Errorf("delegation roles do not match team memberships")
	}
	if delegation.RequiredAuthority > team.MaximumDelegatedAuthority || delegation.RequiredAuthority > principal.AuthorityCeiling || delegation.RequiredAuthority > delegate.AuthorityCeiling {
		return nil, fmt.Errorf("delegation exceeds team or membership authority ceiling")
	}
	if !agentTeamRiskAtOrBelow(risk, team.MaximumDelegatedRisk) || !agentTeamRiskAtOrBelow(risk, principal.RiskCeiling) || !agentTeamRiskAtOrBelow(risk, delegate.RiskCeiling) {
		return nil, fmt.Errorf("delegation exceeds team or membership risk ceiling")
	}
	if err := agentcoordination.ValidateDelegation(team.CoordinationPolicy, delegation, s.now().UTC()); err != nil {
		return nil, err
	}
	digest, err := agentTeamSHA256(struct {
		TeamDigest string                               `json:"teamDigest"`
		Risk       string                               `json:"risk"`
		Delegation agentcoordination.DelegationEnvelope `json:"delegation"`
	}{team.ContractDigest, normalizeIdentifier(risk), delegation})
	if err != nil {
		return nil, err
	}
	return &TeamDelegationAssessment{
		TeamID:                         team.ID,
		TeamVersion:                    team.Version,
		DelegationID:                   delegation.ID,
		ContractValid:                  true,
		AdvisoryOnly:                   true,
		GrantsExecutionAuthority:       false,
		ExecutionAuthorizationRequired: true,
		ContractDigest:                 digest,
	}, nil
}

func (s *AgentTeamService) RecordConsensus(owner, teamID, version, correlationID, idempotencyKey, issue string) (*TeamConsensusOutcome, bool, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, false, err
	}
	team, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version))
	if err != nil {
		return nil, false, err
	}
	existingOutcomes, err := s.repo.ListConsensusOutcomes(owner, team.ID, team.Version)
	if err != nil {
		return nil, false, err
	}
	for _, existing := range existingOutcomes {
		if existing.IdempotencyKey != strings.TrimSpace(idempotencyKey) {
			continue
		}
		if existing.CorrelationID != strings.TrimSpace(correlationID) || existing.Issue != compactContractText(issue) {
			return nil, false, ErrAgentTeamIdempotencyConflict
		}
		result := cloneTeamOutcome(existing)
		return &result, false, nil
	}
	messages, err := s.repo.ListCoordinationMessages(owner, team.ID, team.Version, correlationID)
	if err != nil {
		return nil, false, err
	}
	outcome, err := evaluateTeamConsensus(team, messages, correlationID, idempotencyKey, issue, s.now().UTC())
	if err != nil {
		return nil, false, err
	}
	next := cloneAgentTeam(team)
	next.Revision++
	next.UpdatedAt = s.now().UTC()
	next.ContractDigest = ""
	next, err = normalizeAgentTeamContract(next, next.UpdatedAt)
	if err != nil {
		return nil, false, err
	}
	events, err := s.repo.ListTeamEvents(owner, team.ID, team.Version)
	if err != nil {
		return nil, false, err
	}
	previousDigest := ""
	if len(events) > 0 {
		previousDigest = events[len(events)-1].EventDigest
	}
	event, err := s.newTeamEvent(next, TeamEventConsensusRecorded, "system:team-consensus", outcome.ID, "Persist advisory consensus outcome.", outcome.EvidenceRefs, previousDigest)
	if err != nil {
		return nil, false, err
	}
	stored, created, err := s.repo.RecordConsensusOutcome(owner, outcome, next, team.Revision, event)
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (s *AgentTeamService) ConsensusOutcomes(owner, teamID, version string) ([]TeamConsensusOutcome, error) {
	return s.repo.ListConsensusOutcomes(owner, teamID, version)
}

func (s *AgentTeamService) Events(owner, teamID, version string) ([]TeamLifecycleEvent, error) {
	return s.repo.ListTeamEvents(owner, teamID, version)
}

func (s *AgentTeamService) mutateTeam(owner, teamID, version string, expectedRevision uint64, actor, reason, eventType, subjectID string, evidenceRefs []string, mutate func(*AgentTeamContract, time.Time) error) (*AgentTeamContract, error) {
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	actor, reason, err = normalizeTeamActorReason(actor, reason)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetTeam(owner, strings.TrimSpace(teamID), strings.TrimSpace(version))
	if err != nil {
		return nil, err
	}
	if current.Revision != expectedRevision {
		return nil, fmt.Errorf("%w: expected %d, current %d", ErrAgentTeamRevisionConflict, expectedRevision, current.Revision)
	}
	now := s.now().UTC()
	next := cloneAgentTeam(current)
	if err := mutate(&next, now); err != nil {
		return nil, err
	}
	next.Revision++
	next.UpdatedAt = now
	next.ContractDigest = ""
	next, err = normalizeAgentTeamContract(next, now)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListTeamEvents(owner, next.ID, next.Version)
	if err != nil {
		return nil, err
	}
	previousDigest := ""
	if len(events) > 0 {
		previousDigest = events[len(events)-1].EventDigest
	}
	event, err := s.newTeamEvent(next, eventType, actor, subjectID, reason, evidenceRefs, previousDigest)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateTeam(owner, next, expectedRevision, event); err != nil {
		return nil, err
	}
	result := cloneAgentTeam(next)
	return &result, nil
}

func (s *AgentTeamService) newTeamEvent(team AgentTeamContract, eventType, actor, subjectID, reason string, evidenceRefs []string, previousDigest string) (TeamLifecycleEvent, error) {
	if containsAgentTeamSecret(struct {
		Actor    string
		Reason   string
		Evidence []string
	}{actor, reason, evidenceRefs}) {
		return TeamLifecycleEvent{}, fmt.Errorf("team lifecycle event contains secret material")
	}
	evidenceRefs = redactedSortedUnique(evidenceRefs)
	provenanceDigest, err := agentTeamSHA256(struct {
		ContractDigest string   `json:"contractDigest"`
		Evidence       []string `json:"evidence"`
	}{team.ContractDigest, evidenceRefs})
	if err != nil {
		return TeamLifecycleEvent{}, err
	}
	event := TeamLifecycleEvent{
		Sequence:            team.Revision,
		ID:                  s.newID(),
		TeamID:              team.ID,
		TeamVersion:         team.Version,
		Revision:            team.Revision,
		Type:                eventType,
		Actor:               actor,
		SubjectID:           strings.TrimSpace(subjectID),
		Reason:              compactContractText(reason),
		EvidenceRefs:        evidenceRefs,
		ProvenanceDigest:    provenanceDigest,
		OccurredAt:          team.UpdatedAt.UTC(),
		PreviousEventDigest: previousDigest,
	}
	if _, err := uuid.Parse(event.ID); err != nil {
		return TeamLifecycleEvent{}, fmt.Errorf("team lifecycle event ID must be a UUID")
	}
	if event.Type == "" || event.Actor == "" || event.Reason == "" {
		return TeamLifecycleEvent{}, fmt.Errorf("team lifecycle event type, actor, and reason are required")
	}
	event.EventDigest, err = teamLifecycleEventDigest(event)
	return event, err
}

func normalizeTeamActorReason(actor, reason string) (string, string, error) {
	if containsAgentTeamSecret(struct{ Actor, Reason string }{actor, reason}) {
		return "", "", fmt.Errorf("team lifecycle actor or reason contains secret material")
	}
	actor = normalizeIdentifier(actor)
	reason = compactContractText(reason)
	if actor == "" || reason == "" {
		return "", "", fmt.Errorf("team lifecycle actor and reason are required")
	}
	return actor, reason, nil
}

func activeTeamVoterCount(team AgentTeamContract) int {
	count := 0
	for _, member := range team.Members {
		if teamMemberMayVote(team, member.ID) {
			count++
		}
	}
	return count
}

func findTeamMember(team AgentTeamContract, membershipID string) int {
	for index, member := range team.Members {
		if member.ID == membershipID {
			return index
		}
	}
	return -1
}

func validMembershipTransition(current, next string) bool {
	switch current {
	case TeamMemberInvited:
		return next == TeamMemberActive || next == TeamMemberRevoked
	case TeamMemberActive:
		return next == TeamMemberSuspended || next == TeamMemberLeft || next == TeamMemberRevoked
	case TeamMemberSuspended:
		return next == TeamMemberActive || next == TeamMemberLeft || next == TeamMemberRevoked
	default:
		return false
	}
}

func agentTeamTimePointer(value time.Time) *time.Time {
	normalized := value.UTC()
	return &normalized
}
