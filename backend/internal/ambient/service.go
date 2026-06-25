package ambient

import (
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/models"
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
	Note string `json:"note,omitempty"`
}

type WorkflowService interface {
	Dashboard() (*workflow.WorkflowDashboard, error)
	Items(includeArchived bool) ([]models.WorkflowItem, error)
	Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error)
	RunDue(request workflow.RunDueRequest) (*workflow.WorkflowRunSummary, error)
	RunDueOpenLoops(request workflow.RunDueRequest) (*workflow.OpenLoopRunSummary, error)
}

type Service interface {
	Overview() (*Overview, error)
	Scan(trigger string) (*models.AmbientScan, error)
	UpdateNeed(key string, request NeedUpdateRequest) (*models.AmbientNeed, error)
	Accept(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error)
	Dismiss(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error)
}

type service struct {
	repo         Repository
	workflows    WorkflowService
	memoryEngine memoryengine.Service
	scanning     atomic.Bool
}

func NewService(repo Repository, workflows WorkflowService, memoryEngine memoryengine.Service) Service {
	return &service{repo: repo, workflows: workflows, memoryEngine: memoryEngine}
}

func (s *service) Overview() (*Overview, error) {
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

	candidates := buildCandidates(dashboard, items, memoryDashboard, needMap)
	scan.ItemsExamined = len(items) + len(memoryDashboard.DelegateToVA) + len(memoryDashboard.Contradictions) + len(dashboard.DueOpenLoops)
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
		item.WorkflowID = &record.Item.ID
	}
	item.Status = StatusAccepted
	item.ResolutionNote = appendNote(item.ResolutionNote, request.Note)
	return s.repo.SaveOpportunity(item)
}

func (s *service) Dismiss(id uuid.UUID, request ResolutionRequest) (*models.AmbientOpportunity, error) {
	item, err := s.repo.FindOpportunity(id)
	if err != nil {
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
	return s.repo.SaveOpportunity(item)
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
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].PriorityScore > candidates[j].PriorityScore
	})
	policy := policyFromEnv()
	if len(candidates) > policy.OpportunityLimit {
		return candidates[:policy.OpportunityLimit]
	}
	return candidates
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
		ScanIntervalSeconds:    envInt("AMBIENT_SCAN_INTERVAL_SECONDS", 300, 30, 86400),
		ScanRetention:          envInt("AMBIENT_SCAN_RETENTION", 500, 10, 10000),
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
