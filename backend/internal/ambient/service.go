package ambient

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/models"
	pursuitpkg "automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/safety"
	"automation-hub-backend/internal/workflow"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	StatusProposed  = "proposed"
	StatusAccepted  = "accepted"
	StatusDismissed = "dismissed"
	StatusCompleted = "completed"
)

var ErrScanInProgress = errors.New("ambient scan already in progress")

type Policy struct {
	SchedulerEnabled       bool   `json:"schedulerEnabled"`
	SuggestionOnly         bool   `json:"suggestionOnly"`
	ExecutionEnabled       bool   `json:"executionEnabled"`
	MinimumScore           int    `json:"minimumScore"`
	MinimumConfidence      int    `json:"minimumConfidence"`
	OpportunityLimit       int    `json:"opportunityLimit"`
	ExecutionLimit         int    `json:"executionLimit"`
	CooldownHours          int    `json:"cooldownHours"`
	ScanIntervalSeconds    int    `json:"scanIntervalSeconds"`
	ScanRetention          int    `json:"scanRetention"`
	DualPathKVCacheMode    string `json:"dualPathKvCacheMode"`
	DualPathInfrastructure bool   `json:"dualPathInfrastructureAvailable"`
}

type Overview struct {
	GeneratedAt   time.Time                   `json:"generatedAt"`
	Policy        Policy                      `json:"policy"`
	Needs         []models.AmbientNeed        `json:"needs"`
	Opportunities []models.AmbientOpportunity `json:"opportunities"`
	Scans         []models.AmbientScan        `json:"scans"`
	Warnings      []string                    `json:"warnings"`
}

type NeedUpdateRequest struct {
	CurrentLevel   *int    `json:"currentLevel,omitempty"`
	TargetLevel    *int    `json:"targetLevel,omitempty"`
	PriorityWeight *int    `json:"priorityWeight,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	Notes          *string `json:"notes,omitempty"`
}

type ResolutionRequest struct {
	Note          string `json:"note,omitempty"`
	OwnerIdentity string `json:"-"`
}

type WorkflowService interface {
	Dashboard() (*workflow.WorkflowDashboard, error)
	Items(includeArchived bool) ([]models.WorkflowItem, error)
	Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error)
	RunDue(request workflow.RunDueRequest) (*workflow.WorkflowRunSummary, error)
	RunDueOpenLoops(request workflow.RunDueRequest) (*workflow.OpenLoopRunSummary, error)
}

type PursuitService interface {
	Dashboard() (*pursuitpkg.Dashboard, error)
	List(includeArchived bool) ([]models.Pursuit, error)
	Link(id uuid.UUID, request pursuitpkg.LinkRequest) (*models.PursuitLink, error)
	DetailForOwner(ownerIdentity string, id uuid.UUID) (*pursuitpkg.PursuitDetail, error)
}

type Service interface {
	Overview() (*Overview, error)
	OverviewForOwner(ownerIdentity string) (*Overview, error)
	Scan(trigger string) (*models.AmbientScan, error)
	UpdateNeed(key string, request NeedUpdateRequest) (*models.AmbientNeed, error)
	Accept(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error)
	Dismiss(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error)
}

type service struct {
	repo         Repository
	workflows    WorkflowService
	memoryEngine memoryengine.Service
	memory       memory.Service
	pursuits     PursuitService
	scanning     atomic.Bool
}

func NewService(repo Repository, workflows WorkflowService, memoryEngine memoryengine.Service, memoryServices ...memory.Service) Service {
	return NewServiceWithPursuits(repo, workflows, memoryEngine, nil, memoryServices...)
}

func NewServiceWithPursuits(repo Repository, workflows WorkflowService, memoryEngine memoryengine.Service, pursuits PursuitService, memoryServices ...memory.Service) Service {
	return &service{repo: repo, workflows: workflows, memoryEngine: memoryEngine, pursuits: pursuits, memory: firstMemoryService(memoryServices...)}
}

func (s *service) Overview() (*Overview, error) {
	return s.OverviewForOwner("")
}

func (s *service) OverviewForOwner(ownerIdentity string) (*Overview, error) {
	if err := s.ensureNeeds(); err != nil {
		return nil, err
	}
	needs, err := s.repo.Needs()
	if err != nil {
		return nil, err
	}
	opportunities, err := s.repo.Opportunities("", 75)
	if err != nil {
		return nil, err
	}
	opportunities = s.visibleOpportunities(ownerIdentity, opportunities)
	scans, err := s.repo.Scans(12)
	if err != nil {
		return nil, err
	}
	warnings := []string{
		"Ambient mode proposes work by default; it cannot approve its own high-risk actions.",
		"Need scores are user planning preferences, not medical, psychological, financial, or social judgments.",
		"The normal workflow scheduler remains independent; ambient execution only controls whether a scan requests an immediate bounded workflow pass.",
	}
	policy := policyFromEnv()
	if policy.DualPathKVCacheMode != "disabled" && !policy.DualPathInfrastructure {
		warnings = append(warnings, "DualPath KV-cache mode is configured as a capability hint only; this deployment has no verified disaggregated prefill/decode and RDMA infrastructure.")
	}
	return &Overview{
		GeneratedAt:   time.Now().UTC(),
		Policy:        policy,
		Needs:         needs,
		Opportunities: opportunities,
		Scans:         scans,
		Warnings:      warnings,
	}, nil
}

func (s *service) Scan(trigger string) (*models.AmbientScan, error) {
	if !s.scanning.CompareAndSwap(false, true) {
		return nil, ErrScanInProgress
	}
	defer s.scanning.Store(false)
	if err := s.ensureNeeds(); err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	scan, err := s.repo.CreateScan(&models.AmbientScan{
		Trigger:   firstNonEmpty(strings.TrimSpace(trigger), "manual"),
		Status:    "running",
		StartedAt: started,
	})
	if err != nil {
		return nil, err
	}
	fail := func(scanErr error) (*models.AmbientScan, error) {
		completed := time.Now().UTC()
		scan.Status = "failed"
		scan.CompletedAt = &completed
		scan.ErrorMessage = scanErr.Error()
		_, _ = s.repo.UpdateScan(scan)
		return scan, scanErr
	}

	needs, err := s.repo.Needs()
	if err != nil {
		return fail(err)
	}
	needMap := map[string]models.AmbientNeed{}
	for _, need := range needs {
		needMap[need.Key] = need
	}
	dashboard, err := s.workflows.Dashboard()
	if err != nil {
		return fail(err)
	}
	items, err := s.workflows.Items(false)
	if err != nil {
		return fail(err)
	}
	workflowStates := map[uuid.UUID]string{}
	for _, item := range items {
		workflowStates[item.ID] = item.CurrentState
	}
	memoryDashboard, err := s.memoryEngine.Dashboard()
	if err != nil {
		return fail(err)
	}
	var pursuitDashboard *pursuitpkg.Dashboard
	var pursuits []models.Pursuit
	if s.pursuits != nil {
		pursuitDashboard, err = s.pursuits.Dashboard()
		if err != nil {
			return fail(err)
		}
		pursuits, err = s.pursuits.List(true)
		if err != nil {
			return fail(err)
		}
	}

	candidates := buildCandidates(dashboard, items, memoryDashboard, pursuitDashboard, needMap)
	scan.ItemsExamined = len(items) + len(memoryDashboard.DelegateToVA) + len(memoryDashboard.Contradictions) + len(dashboard.DueOpenLoops) + pursuitSignalCount(pursuitDashboard)
	scan.OpportunitiesFound = len(candidates)
	policy := policyFromEnv()
	now := time.Now().UTC()
	for _, candidate := range candidates {
		if candidate.PriorityScore < policy.MinimumScore || candidate.Confidence < policy.MinimumConfidence {
			scan.Filtered++
			continue
		}
		existing, findErr := s.repo.FindOpportunityByFingerprint(candidate.Fingerprint)
		if findErr != nil {
			return fail(findErr)
		}
		if existing != nil {
			existing.Title = candidate.Title
			existing.Rationale = candidate.Rationale
			existing.NextAction = candidate.NextAction
			existing.PriorityScore = candidate.PriorityScore
			existing.Urgency = candidate.Urgency
			existing.Impact = candidate.Impact
			existing.Effort = candidate.Effort
			existing.Confidence = candidate.Confidence
			existing.Risk = candidate.Risk
			existing.RequiresApproval = candidate.RequiresApproval
			existing.EvidenceManifest = candidate.EvidenceManifest
			existing.LastSeenAt = now
			if existing.Status == StatusDismissed && (existing.CooldownUntil == nil || existing.CooldownUntil.Before(now)) {
				existing.Status = StatusProposed
			}
			if _, saveErr := s.repo.SaveOpportunity(existing); saveErr != nil {
				return fail(saveErr)
			}
			scan.Updated++
			scan.Deduplicated++
			scan.DeduplicatedBytes += int64(len(candidate.Rationale) + len(candidate.NextAction) + len(candidate.EvidenceManifest))
		} else {
			candidate.LastSeenAt = now
			candidate.Status = StatusProposed
			if _, saveErr := s.repo.SaveOpportunity(&candidate); saveErr != nil {
				return fail(saveErr)
			}
			scan.Created++
		}
		scan.ManifestBytes += int64(len(candidate.EvidenceManifest))
	}
	storedOpportunities, listErr := s.repo.Opportunities("", 200)
	if listErr != nil {
		return fail(listErr)
	}
	for index := range storedOpportunities {
		item := &storedOpportunities[index]
		if item.WorkflowID == nil || item.Status == StatusCompleted || item.Status == StatusDismissed {
			continue
		}
		state := workflowStates[*item.WorkflowID]
		if state != workflow.StateCompleted && state != workflow.StateArchived {
			continue
		}
		item.Status = StatusCompleted
		item.ResolutionNote = appendNote(item.ResolutionNote, "Linked workflow reached "+state+".")
		if _, saveErr := s.repo.SaveOpportunity(item); saveErr != nil {
			return fail(saveErr)
		}
		scan.Updated++
	}
	for _, item := range closedPursuitOpportunityUpdates(storedOpportunities, pursuits, now) {
		if _, saveErr := s.repo.SaveOpportunity(&item); saveErr != nil {
			return fail(saveErr)
		}
		scan.Updated++
	}

	if policy.ExecutionEnabled && !policy.SuggestionOnly && !safety.EmergencyStopActive() {
		openLoops, runErr := s.workflows.RunDueOpenLoops(workflow.RunDueRequest{Limit: policy.ExecutionLimit})
		if runErr != nil {
			return fail(runErr)
		}
		runs, runErr := s.workflows.RunDue(workflow.RunDueRequest{Limit: policy.ExecutionLimit})
		if runErr != nil {
			return fail(runErr)
		}
		scan.Advanced = openLoops.Triggered + openLoops.Resolved + runs.Completed
		scan.Skipped = openLoops.Skipped + runs.Skipped
		scan.Blocked += runs.Blocked
	}
	completed := time.Now().UTC()
	scan.Status = "completed"
	scan.CompletedAt = &completed
	updated, err := s.repo.UpdateScan(scan)
	if err != nil {
		return nil, err
	}
	if err := s.repo.PruneScans(policy.ScanRetention); err != nil {
		log.Printf("ambient scan retention cleanup failed: %v", err)
	}
	return updated, nil
}

func (s *service) UpdateNeed(key string, request NeedUpdateRequest) (*models.AmbientNeed, error) {
	if err := s.ensureNeeds(); err != nil {
		return nil, err
	}
	needs, err := s.repo.Needs()
	if err != nil {
		return nil, err
	}
	for _, need := range needs {
		if need.Key != strings.TrimSpace(key) {
			continue
		}
		if request.CurrentLevel != nil {
			need.CurrentLevel = clamp(*request.CurrentLevel, 0, 100)
		}
		if request.TargetLevel != nil {
			need.TargetLevel = clamp(*request.TargetLevel, 0, 100)
		}
		if request.PriorityWeight != nil {
			need.PriorityWeight = clamp(*request.PriorityWeight, 0, 100)
		}
		if request.Enabled != nil {
			need.Enabled = *request.Enabled
		}
		if request.Notes != nil {
			need.Notes = strings.TrimSpace(*request.Notes)
		}
		return s.repo.UpdateNeed(&need)
	}
	return nil, fmt.Errorf("ambient need %q not found", key)
}

func (s *service) Accept(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error) {
	item, err := s.repo.FindOpportunity(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensurePursuitOpportunityVisible(*item, request.OwnerIdentity); err != nil {
		return nil, err
	}
	if item.Status == StatusCompleted {
		return item, nil
	}
	if item.Status == StatusAccepted {
		return item, nil
	}
	if item.Status != StatusProposed {
		return nil, fmt.Errorf("opportunity in %s state cannot be accepted", item.Status)
	}
	if item.WorkflowID == nil {
		if s.workflows == nil {
			return nil, fmt.Errorf("workflow service is not configured")
		}
		record, intakeErr := s.workflows.Intake(workflow.IntakeRequest{
			Input:          item.NextAction,
			SourceType:     "ambient_opportunity",
			SourceID:       item.ID.String(),
			SourceURI:      item.SourceURI,
			SourceLabel:    item.Title,
			ContentType:    "ambient_proposal",
			Trigger:        "ambient.accept",
			Actor:          "operator",
			RequiresReview: item.RequiresApproval,
			ReviewReason:   firstNonEmpty(strings.TrimSpace(request.Note), item.Rationale),
		})
		if intakeErr != nil {
			return nil, intakeErr
		}
		if record == nil || record.Item.ID == uuid.Nil {
			return nil, fmt.Errorf("workflow intake did not return a workflow record")
		}
		item.WorkflowID = &record.Item.ID
		if err := s.linkAcceptedPursuitOpportunity(item, record); err != nil {
			return nil, err
		}
	}
	item.Status = StatusAccepted
	item.ResolutionNote = appendNote(item.ResolutionNote, request.Note)
	saved, err := s.repo.SaveOpportunity(item)
	if err != nil {
		return nil, err
	}
	s.rememberOpportunityFeedback(saved, "ambient_opportunity_accepted", request.Note)
	return saved, nil
}

func (s *service) linkAcceptedPursuitOpportunity(item *models.AmbientOpportunity, record *workflow.WorkflowRecord) error {
	if s.pursuits == nil || item == nil || record == nil || record.Item.ID == uuid.Nil || !strings.HasPrefix(strings.TrimSpace(item.SourceType), "pursuit") {
		return nil
	}
	pursuitID, err := uuid.Parse(strings.TrimSpace(item.SourceID))
	if err != nil {
		return nil
	}
	_, err = s.pursuits.Link(pursuitID, pursuitpkg.LinkRequest{
		LinkType:     pursuitpkg.LinkWorkflow,
		LinkID:       record.Item.ID.String(),
		Relationship: "ambient_follow_up",
		SourceURI:    item.SourceURI,
		SourceLabel:  item.Title,
		Confidence:   float64(clamp(item.Confidence, 0, 100)) / 100,
		Actor:        "ambient",
	})
	if err != nil {
		return fmt.Errorf("link accepted ambient workflow to pursuit: %w", err)
	}
	sourceURI := firstNonEmpty(item.SourceURI, "ambient://opportunities/"+item.ID.String())
	_, err = s.pursuits.Link(pursuitID, pursuitpkg.LinkRequest{
		LinkType:     pursuitpkg.LinkAmbientOpportunity,
		LinkID:       item.ID.String(),
		Relationship: "ambient_proposal_accepted",
		SourceURI:    sourceURI,
		SourceLabel:  item.Title,
		Confidence:   float64(clamp(item.Confidence, 0, 100)) / 100,
		Actor:        "ambient",
	})
	if err != nil {
		return fmt.Errorf("link accepted ambient proposal to pursuit: %w", err)
	}
	return nil
}

func (s *service) Dismiss(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error) {
	item, err := s.repo.FindOpportunity(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensurePursuitOpportunityVisible(*item, request.OwnerIdentity); err != nil {
		return nil, err
	}
	if item.Status == StatusDismissed {
		return item, nil
	}
	if item.Status != StatusProposed {
		return nil, fmt.Errorf("opportunity in %s state cannot be dismissed", item.Status)
	}
	item.Status = StatusDismissed
	item.ResolutionNote = appendNote(item.ResolutionNote, request.Note)
	cooldown := time.Now().UTC().Add(time.Duration(policyFromEnv().CooldownHours) * time.Hour)
	item.CooldownUntil = &cooldown
	saved, err := s.repo.SaveOpportunity(item)
	if err != nil {
		return nil, err
	}
	s.rememberOpportunityFeedback(saved, "ambient_opportunity_dismissed", request.Note)
	return saved, nil
}

func (s *service) visibleOpportunities(ownerIdentity string, opportunities []models.AmbientOpportunity) []models.AmbientOpportunity {
	if strings.TrimSpace(ownerIdentity) == "" || s.pursuits == nil {
		return opportunities
	}
	visible := make([]models.AmbientOpportunity, 0, len(opportunities))
	for _, item := range opportunities {
		if s.ensurePursuitOpportunityVisible(item, ownerIdentity) == nil {
			visible = append(visible, item)
		}
	}
	return visible
}

func (s *service) ensurePursuitOpportunityVisible(item models.AmbientOpportunity, ownerIdentity string) error {
	if strings.TrimSpace(ownerIdentity) == "" || s.pursuits == nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.SourceType)), "pursuit") {
		return nil
	}
	pursuitID, err := uuid.Parse(strings.TrimSpace(item.SourceID))
	if err != nil {
		return fmt.Errorf("ambient opportunity not found")
	}
	if _, err := s.pursuits.DetailForOwner(ownerIdentity, pursuitID); err != nil {
		return fmt.Errorf("ambient opportunity not found")
	}
	return nil
}

func (s *service) rememberOpportunityFeedback(item *models.AmbientOpportunity, signal, note string) {
	if s.memory == nil || item == nil || !ambientFeedbackUseful(signal, note, *item) {
		return
	}
	sourceURI := firstNonEmpty(item.SourceURI, "ambient://opportunities/"+item.ID.String())
	_, _ = s.memory.Create(memory.CreateRequest{
		Kind:        "lesson",
		Content:     ambientFeedbackContent(*item, signal, note),
		Summary:     ambientFeedbackSummary(*item, signal, note),
		Tags:        ambientFeedbackTags(*item, signal),
		Confidence:  ambientFeedbackConfidence(signal, note),
		SourceURI:   sourceURI,
		SourceLabel: ambientFeedbackSourceLabel(*item),
	})
}

func (s *service) ensureNeeds() error {
	return s.repo.EnsureNeeds(defaultNeeds())
}

func defaultNeeds() []models.AmbientNeed {
	return []models.AmbientNeed{
		{Key: "physiological", Name: "Health and capacity", Description: "Protect time, energy, rest, food, housing basics, and sustainable workload.", CurrentLevel: 50, TargetLevel: 75, PriorityWeight: 90, Enabled: true},
		{Key: "safety", Name: "Safety and stability", Description: "Reduce legal, financial, account, deadline, security, and operational risk.", CurrentLevel: 50, TargetLevel: 85, PriorityWeight: 100, Enabled: true},
		{Key: "belonging", Name: "Relationships and belonging", Description: "Maintain important relationships, commitments, replies, and collaboration.", CurrentLevel: 50, TargetLevel: 75, PriorityWeight: 70, Enabled: true},
		{Key: "esteem", Name: "Reputation and capability", Description: "Improve reliability, professional standing, skills, and completed commitments.", CurrentLevel: 50, TargetLevel: 80, PriorityWeight: 65, Enabled: true},
		{Key: "growth", Name: "Growth and self-direction", Description: "Advance meaningful projects, learning, creativity, and long-term agency.", CurrentLevel: 50, TargetLevel: 85, PriorityWeight: 60, Enabled: true},
	}
}

func buildCandidates(
	dashboard *workflow.WorkflowDashboard,
	items []models.WorkflowItem,
	memoryDashboard *memoryengine.CommandDashboard,
	pursuitDashboard *pursuitpkg.Dashboard,
	needs map[string]models.AmbientNeed,
) []models.AmbientOpportunity {
	candidates := []models.AmbientOpportunity{}
	seen := map[string]bool{}
	add := func(candidate models.AmbientOpportunity) {
		if need, ok := needs[candidate.NeedKey]; !ok || !need.Enabled {
			return
		}
		candidate.PriorityScore = opportunityScore(candidate, needs[candidate.NeedKey])
		candidate.Fingerprint = fingerprint(candidate.SourceType, candidate.SourceID, candidate.NeedKey)
		if seen[candidate.Fingerprint] {
			return
		}
		seen[candidate.Fingerprint] = true
		candidates = append(candidates, candidate)
	}
	for index := range items {
		item := items[index]
		needKey := needForWorkflow(item)
		reason := ""
		urgency := 45
		impact := 55
		effort := 45
		risk := riskScore(item.RiskLevel)
		switch item.CurrentState {
		case workflow.StateNeedsApproval:
			reason = "A workflow is paused at a human approval gate."
			urgency = 80
		case workflow.StateBlocked:
			reason = "A workflow is blocked and cannot advance without resolving its stated constraint."
			urgency = 75
		case workflow.StateReady:
			reason = "A verified low-risk workflow is ready for the existing worker queue."
			urgency = 65
		default:
			if strings.TrimSpace(item.NextAction) == "" {
				reason = "An active workflow has no explicit next action."
				urgency = 60
			}
		}
		if reason == "" {
			continue
		}
		if item.DueAt != nil {
			hours := time.Until(*item.DueAt).Hours()
			if hours <= 24 {
				urgency = 100
			} else if hours <= 168 {
				urgency = max(urgency, 85)
			}
		}
		nextAction := firstNonEmpty(item.NextAction, "define and confirm the next responsible action")
		manifest := evidenceManifest("workflow", item.ID.String(), item.SourceURI, item.UpdatedAt)
		id := item.ID
		add(models.AmbientOpportunity{
			WorkflowID:       &id,
			NeedKey:          needKey,
			Title:            item.Title,
			Rationale:        reason + " " + scoreExplanation(urgency, impact, risk, int(math.Round(item.Confidence*100))),
			NextAction:       nextAction,
			SourceType:       "workflow",
			SourceID:         item.ID.String(),
			SourceURI:        safety.RedactURL(item.SourceURI),
			EvidenceManifest: manifest,
			Urgency:          urgency,
			Impact:           impact,
			Effort:           effort,
			Confidence:       clamp(int(math.Round(item.Confidence*100)), 20, 100),
			Risk:             risk,
			RequiresApproval: item.RequiresApproval || item.CurrentState == workflow.StateNeedsApproval || risk >= 60,
		})
	}
	for _, loop := range dashboard.DueOpenLoops {
		needKey := "belonging"
		add(models.AmbientOpportunity{
			WorkflowID:       &loop.WorkflowID,
			NeedKey:          needKey,
			Title:            "Follow up: " + compact(loop.WaitingFor, 180),
			Rationale:        "An external dependency reached its follow-up time. The existing open-loop engine remains responsible for safe execution.",
			NextAction:       loop.NextAction,
			SourceType:       "workflow_open_loop",
			SourceID:         loop.ID.String(),
			EvidenceManifest: evidenceManifest("workflow_open_loop", loop.ID.String(), "", loop.UpdatedAt),
			Urgency:          82,
			Impact:           62,
			Effort:           25,
			Confidence:       90,
			Risk:             45,
			RequiresApproval: true,
		})
	}
	for _, insight := range memoryDashboard.Contradictions {
		add(models.AmbientOpportunity{
			NeedKey:          "safety",
			Title:            "Resolve conflicting information",
			Rationale:        "A source-linked memory insight is marked as contradictory and must not silently guide execution.",
			NextAction:       "review the conflicting claim and its source before updating memory or acting",
			SourceType:       "memory_insight",
			SourceID:         insight.ID.String(),
			SourceURI:        safety.RedactURL(insight.SourceURI),
			EvidenceManifest: evidenceManifest("memory_insight", insight.ID.String(), insight.SourceURI, insight.UpdatedAt),
			Urgency:          78,
			Impact:           78,
			Effort:           35,
			Confidence:       clamp(int(math.Round(insight.Confidence*100)), 20, 100),
			Risk:             75,
			RequiresApproval: true,
		})
	}
	for _, insight := range memoryDashboard.DelegateToVA {
		add(models.AmbientOpportunity{
			NeedKey:          "esteem",
			Title:            compact(insight.Text, 220),
			Rationale:        "A source-linked action is suitable for structured delegation, subject to the workflow rule and approval engine.",
			NextAction:       "convert this action into a scoped delegation package with source links and authority limits",
			SourceType:       "memory_insight",
			SourceID:         insight.ID.String(),
			SourceURI:        safety.RedactURL(insight.SourceURI),
			EvidenceManifest: evidenceManifest("memory_insight", insight.ID.String(), insight.SourceURI, insight.UpdatedAt),
			Urgency:          50,
			Impact:           60,
			Effort:           30,
			Confidence:       clamp(int(math.Round(insight.Confidence*100)), 20, 100),
			Risk:             riskScore(insight.RiskLevel),
			RequiresApproval: insight.NeedsReview || insight.RobertNeeded,
		})
	}
	addPursuitCandidates(pursuitDashboard, add)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].PriorityScore > candidates[j].PriorityScore
	})
	policy := policyFromEnv()
	if len(candidates) > policy.OpportunityLimit {
		return candidates[:policy.OpportunityLimit]
	}
	return candidates
}

func addPursuitCandidates(dashboard *pursuitpkg.Dashboard, add func(models.AmbientOpportunity)) {
	if dashboard == nil {
		return
	}
	for _, item := range dashboard.NeedsRobert {
		add(pursuitOpportunity(item, "pursuit_decision", "Robert decision needed: "+compact(item.Pursuit.Title, 180), "A pursuit has pending approvals, high-risk next actions, or Robert-only decisions. HAI should present a clear Yes/No or small-option decision instead of asking Robert to re-plan.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "review the recommended pursuit action and approve, reject, or request revision"), 88, 78, 20, true))
	}
	for _, item := range dashboard.ReviewDue {
		add(pursuitOpportunity(item, "pursuit_review_due", "Review pursuit: "+compact(item.Pursuit.Title, 180), "A pursuit has reached its scheduled review time. HAI should resurface the current state, verify whether the next action still makes sense, and either record the review or propose a safe follow-up.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "review the pursuit and either record completion of the review or snooze it"), 74, 70, 22, riskScore(item.Pursuit.RiskLevel) >= 60))
	}
	for _, item := range dashboard.PlanningNeeded {
		add(pursuitOpportunity(item, "pursuit_planning_needed", "Plan pursuit: "+compact(item.Pursuit.Title, 180), "A pursuit has no linked workflow yet. HAI should convert the goal into the first concrete workflow, checklist, evidence requirement, and approval boundary instead of leaving Robert to manually re-plan.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "create the first concrete workflow plan for this pursuit"), 66, 72, 26, riskScore(item.Pursuit.RiskLevel) >= 60))
	}
	for _, item := range dashboard.Blocked {
		add(pursuitOpportunity(item, "pursuit_blocker", "Unblock pursuit: "+compact(item.Pursuit.Title, 180), "A pursuit is blocked or waiting on missing information. The ambient brain should surface the blocker and prepare the next safe follow-up.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "identify the blocker owner and prepare the next follow-up"), 78, 72, 35, riskScore(item.Pursuit.RiskLevel) >= 60))
	}
	for _, item := range dashboard.Stale {
		add(pursuitOpportunity(item, "pursuit_stale", "Restart stale pursuit: "+compact(item.Pursuit.Title, 180), "A pursuit has not moved recently. HAI should propose the smallest concrete action that restarts momentum without requiring Robert to reconstruct context.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "review stale pursuit context and choose the next concrete action"), 62, 64, 28, riskScore(item.Pursuit.RiskLevel) >= 60))
	}
	for _, item := range dashboard.VAReady {
		add(pursuitOpportunity(item, "pursuit_va_ready", "Delegate pursuit work: "+compact(item.Pursuit.Title, 180), "A pursuit appears suitable for VA-ready work. HAI should prepare a bounded delegation package with source links and authority limits.", firstNonEmpty(item.NextAction, "prepare a scoped VA delegation package with context, source links, checklist, and escalation rules"), 54, 66, 30, false))
	}
	for _, item := range dashboard.SystemReady {
		add(pursuitOpportunity(item, "pursuit_system_ready", "Run safe pursuit work: "+compact(item.Pursuit.Title, 180), "A low-risk pursuit has work that HAI can move through existing workflow and approval-safe execution paths.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "queue the next low-risk workflow step"), 58, 68, 25, false))
	}
	for _, item := range dashboard.CompletionCandidates {
		add(pursuitOpportunity(item, "pursuit_completion_candidate", "Verify completion: "+compact(item.Pursuit.Title, 180), "A pursuit may be complete, but completion must be verified against evidence before it is accepted as done.", firstNonEmpty(item.NextAction, "review linked evidence and confirm whether the pursuit meets its completion definition"), 72, 74, 32, true))
	}
	for _, item := range dashboard.HighRisk {
		if highRiskPursuitAlreadyCovered(item) {
			continue
		}
		add(pursuitOpportunity(item, "pursuit_high_risk", "Review high-risk pursuit: "+compact(item.Pursuit.Title, 180), "A high-risk pursuit is active without another immediate ambient queue marker. HAI should proactively verify that approval boundaries, source evidence, and the next action are still explicit before work advances.", firstNonEmpty(item.NextAction, item.Pursuit.NextRecommendedAction, "review approval boundaries, evidence, and next action for this high-risk pursuit"), 70, 76, 28, true))
	}
}

func highRiskPursuitAlreadyCovered(item pursuitpkg.PursuitListItem) bool {
	return item.NeedsRobert > 0 ||
		item.Blocked > 0 ||
		item.ReviewDue ||
		item.PlanningNeeded ||
		item.Stale ||
		item.CompletionCandidate
}

func pursuitOpportunity(item pursuitpkg.PursuitListItem, sourceType, title, rationale, nextAction string, urgency, impact, effort int, requiresApproval bool) models.AmbientOpportunity {
	pursuit := item.Pursuit
	sourceID := pursuit.ID.String()
	sourceURI := "pursuit://" + sourceID
	risk := riskScore(pursuit.RiskLevel)
	confidence := clamp(int(math.Round(pursuit.Confidence*100)), 35, 100)
	if pursuit.Confidence <= 0 {
		confidence = 60
	}
	impact = max(impact, clamp(pursuit.PriorityScore, 0, 100))
	return models.AmbientOpportunity{
		NeedKey:          needForPursuit(pursuit),
		Title:            title,
		Rationale:        rationale + " " + scoreExplanation(urgency, impact, risk, confidence),
		NextAction:       nextAction,
		SourceType:       sourceType,
		SourceID:         sourceID,
		SourceURI:        sourceURI,
		EvidenceManifest: evidenceManifest(sourceType, sourceID, sourceURI, pursuit.UpdatedAt),
		Urgency:          urgency,
		Impact:           impact,
		Effort:           effort,
		Confidence:       confidence,
		Risk:             risk,
		RequiresApproval: requiresApproval || risk >= 80,
	}
}

func pursuitSignalCount(dashboard *pursuitpkg.Dashboard) int {
	if dashboard == nil {
		return 0
	}
	return len(dashboard.NeedsRobert) + len(dashboard.ReviewDue) + len(dashboard.PlanningNeeded) + len(dashboard.Blocked) + len(dashboard.Stale) + len(dashboard.VAReady) + len(dashboard.SystemReady) + len(dashboard.CompletionCandidates) + len(dashboard.HighRisk)
}

// closedPursuitOpportunityUpdates prevents the ambient queue from resurfacing
// a proposal after its parent pursuit has been completed or archived. It only
// changes open/accepted pursuit-derived opportunities and preserves dismissed
// records as operator feedback.
func closedPursuitOpportunityUpdates(opportunities []models.AmbientOpportunity, pursuits []models.Pursuit, now time.Time) []models.AmbientOpportunity {
	byID := make(map[string]models.Pursuit, len(pursuits))
	for _, item := range pursuits {
		if item.ID != uuid.Nil {
			byID[item.ID.String()] = item
		}
	}

	updates := []models.AmbientOpportunity{}
	for _, opportunity := range opportunities {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(opportunity.SourceType)), "pursuit") ||
			opportunity.Status == StatusCompleted || opportunity.Status == StatusDismissed {
			continue
		}
		pursuit, found := byID[strings.TrimSpace(opportunity.SourceID)]
		if !found || (!pursuit.Archived && !strings.EqualFold(pursuit.Status, pursuitpkg.StatusCompleted)) {
			continue
		}
		opportunity.Status = StatusCompleted
		opportunity.LastSeenAt = now
		state := firstNonEmpty(strings.TrimSpace(pursuit.Status), "archived")
		if pursuit.Archived {
			state = "archived"
		}
		opportunity.ResolutionNote = appendNote(opportunity.ResolutionNote, "Linked pursuit closed ("+state+").")
		updates = append(updates, opportunity)
	}
	return updates
}

func opportunityScore(item models.AmbientOpportunity, need models.AmbientNeed) int {
	gap := clamp(need.TargetLevel-need.CurrentLevel, 0, 100)
	effortBenefit := 100 - clamp(item.Effort, 0, 100)
	score := 0.25*float64(item.Urgency) +
		0.24*float64(item.Impact) +
		0.18*float64(item.Confidence) +
		0.12*float64(effortBenefit) +
		0.12*float64(gap) +
		0.09*float64(need.PriorityWeight) -
		0.12*float64(item.Risk)
	if item.RequiresApproval {
		score -= 3
	}
	return clamp(int(math.Round(score)), 0, 100)
}

func policyFromEnv() Policy {
	dualMode := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_KV_CACHE_LOAD_STRATEGY")))
	switch dualMode {
	case "traditional", "storage_to_prefill", "storage_to_decode", "dual", "auto":
	default:
		dualMode = "disabled"
	}
	return Policy{
		SchedulerEnabled:       envBool("AMBIENT_SCHEDULER_ENABLED", true),
		SuggestionOnly:         !envBool("AMBIENT_EXECUTION_ENABLED", false),
		ExecutionEnabled:       envBool("AMBIENT_EXECUTION_ENABLED", false),
		MinimumScore:           envInt("AMBIENT_MINIMUM_SCORE", 35, 0, 100),
		MinimumConfidence:      envInt("AMBIENT_MINIMUM_CONFIDENCE", 35, 0, 100),
		OpportunityLimit:       envInt("AMBIENT_OPPORTUNITY_LIMIT", 50, 1, 200),
		ExecutionLimit:         envInt("AMBIENT_EXECUTION_LIMIT", 3, 1, 20),
		CooldownHours:          envInt("AMBIENT_DISMISS_COOLDOWN_HOURS", 168, 1, 8760),
		ScanIntervalSeconds:    envInt("AMBIENT_SCAN_INTERVAL_SECONDS", 3600, 30, 86400),
		ScanRetention:          envInt("AMBIENT_SCAN_RETENTION", 100, 10, 10000),
		DualPathKVCacheMode:    dualMode,
		DualPathInfrastructure: envBool("LLM_DUALPATH_INFRASTRUCTURE_VERIFIED", false),
	}
}

func needForWorkflow(item models.WorkflowItem) string {
	text := strings.ToLower(item.TaskType + " " + item.Title + " " + item.Description)
	switch {
	case item.RiskLevel == "high" || strings.Contains(text, "legal") || strings.Contains(text, "financial") || strings.Contains(text, "security") || strings.Contains(text, "government") || strings.Contains(text, "insurance"):
		return "safety"
	case strings.Contains(text, "reply") || strings.Contains(text, "client") || strings.Contains(text, "meeting") || strings.Contains(text, "follow"):
		return "belonging"
	case strings.Contains(text, "health") || strings.Contains(text, "rest") || strings.Contains(text, "housing"):
		return "physiological"
	case strings.Contains(text, "publish") || strings.Contains(text, "career") || strings.Contains(text, "reputation") || strings.Contains(text, "delegate"):
		return "esteem"
	default:
		return "growth"
	}
}

func needForPursuit(pursuit models.Pursuit) string {
	category := strings.ToLower(strings.TrimSpace(pursuit.NeedCategory))
	switch {
	case strings.Contains(category, "safety") || strings.Contains(category, "stability"):
		return "safety"
	case strings.Contains(category, "belong"):
		return "belonging"
	case strings.Contains(category, "esteem") || strings.Contains(category, "reputation") || strings.Contains(category, "capability"):
		return "esteem"
	case strings.Contains(category, "physiological") || strings.Contains(category, "health") || strings.Contains(category, "housing"):
		return "physiological"
	case strings.Contains(category, "growth"):
		return "growth"
	}
	text := strings.ToLower(pursuit.Domain + " " + pursuit.Title + " " + pursuit.Description + " " + pursuit.DesiredOutcome)
	switch {
	case strings.EqualFold(pursuit.RiskLevel, "high") || strings.Contains(text, "legal") || strings.Contains(text, "government") || strings.Contains(text, "insurance") || strings.Contains(text, "financial") || strings.Contains(text, "security"):
		return "safety"
	case strings.Contains(text, "client") || strings.Contains(text, "reply") || strings.Contains(text, "relationship") || strings.Contains(text, "household"):
		return "belonging"
	case strings.Contains(text, "reputation") || strings.Contains(text, "career") || strings.Contains(text, "developer") || strings.Contains(text, "delegate"):
		return "esteem"
	case strings.Contains(text, "health") || strings.Contains(text, "housing") || strings.Contains(text, "home"):
		return "physiological"
	default:
		return "growth"
	}
}

func evidenceManifest(kind, id, uri string, updatedAt time.Time) string {
	payload, _ := json.Marshal(map[string]string{
		"type":      kind,
		"id":        id,
		"sourceUri": safety.RedactURL(uri),
		"updatedAt": updatedAt.UTC().Format(time.RFC3339),
	})
	return string(payload)
}

func fingerprint(values ...string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(values, "\x00"))))
	return hex.EncodeToString(sum[:])
}

func riskScore(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return 100
	case "high":
		return 80
	case "medium":
		return 50
	default:
		return 20
	}
}

func scoreExplanation(urgency, impact, risk, confidence int) string {
	return fmt.Sprintf("Ranking inputs: urgency %d, impact %d, risk %d, confidence %d.", urgency, impact, risk, confidence)
}

func firstMemoryService(services ...memory.Service) memory.Service {
	for _, service := range services {
		if service != nil {
			return service
		}
	}
	return nil
}

func ambientFeedbackUseful(signal, note string, item models.AmbientOpportunity) bool {
	switch strings.TrimSpace(signal) {
	case "ambient_opportunity_accepted":
		return strings.TrimSpace(item.NextAction) != "" || strings.TrimSpace(note) != ""
	case "ambient_opportunity_dismissed":
		return len([]rune(strings.TrimSpace(note))) >= 12
	default:
		return false
	}
}

func ambientFeedbackContent(item models.AmbientOpportunity, signal, note string) string {
	parts := []string{
		"Robert gave feedback on HAI ambient proactive suggestions.",
		"Signal: " + strings.ReplaceAll(strings.TrimSpace(signal), "_", " ") + ".",
	}
	if item.NeedKey != "" {
		parts = append(parts, "Need area: "+item.NeedKey+".")
	}
	if item.Title != "" {
		parts = append(parts, "Suggestion: "+item.Title+".")
	}
	if item.NextAction != "" {
		parts = append(parts, "Proposed action: "+compact(item.NextAction, 420)+".")
	}
	if item.Rationale != "" {
		parts = append(parts, "Original rationale: "+compact(item.Rationale, 420)+".")
	}
	if strings.TrimSpace(note) != "" {
		parts = append(parts, "Robert note: "+strings.TrimSpace(note)+".")
	}
	switch strings.TrimSpace(signal) {
	case "ambient_opportunity_dismissed":
		parts = append(parts, "Future behavior: avoid similar proactive suggestions unless stronger source evidence, urgency, or explicit user intent is present.")
	default:
		parts = append(parts, "Future behavior: similar proactive suggestions may be useful when source evidence and approval gates are preserved.")
	}
	return strings.Join(parts, " ")
}

func ambientFeedbackSummary(item models.AmbientOpportunity, signal, note string) string {
	status := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(signal), "_", " "), "ambient opportunity ")
	base := "Ambient " + firstNonEmpty(status, "feedback")
	if item.Title != "" {
		base += ": " + item.Title
	}
	if strings.TrimSpace(note) != "" {
		base += " - " + strings.TrimSpace(note)
	}
	return compact(base, 240)
}

func ambientFeedbackTags(item models.AmbientOpportunity, signal string) []string {
	tags := []string{"ambient-feedback", strings.TrimSpace(signal)}
	for _, value := range []string{item.NeedKey, item.SourceType, item.Status} {
		if value = strings.TrimSpace(value); value != "" {
			tags = append(tags, value)
		}
	}
	return tags
}

func ambientFeedbackConfidence(signal, note string) float64 {
	switch strings.TrimSpace(signal) {
	case "ambient_opportunity_dismissed":
		if strings.TrimSpace(note) != "" {
			return 0.8
		}
		return 0.62
	case "ambient_opportunity_accepted":
		if strings.TrimSpace(note) != "" {
			return 0.72
		}
		return 0.6
	default:
		return 0.55
	}
}

func ambientFeedbackSourceLabel(item models.AmbientOpportunity) string {
	if title := strings.TrimSpace(item.Title); title != "" {
		return "Ambient feedback: " + title
	}
	return "Ambient opportunity feedback"
}

func appendNote(base, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return base
	}
	return base + " Operator note: " + compact(note, 500)
}

func compact(value string, maxLength int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength-3] + "..."
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

func envInt(name string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return clamp(value, minimum, maximum)
}
