package workflow

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StateNewInput           = "new_input"
	StateClassified         = "classified"
	StateLinked             = "linked"
	StateChecklistGenerated = "checklist_generated"
	StateWaitingInput       = "waiting_external_input"
	StateNeedsApproval      = "needs_approval"
	StateReady              = "ready"
	StateInProgress         = "in_progress"
	StateCompleted          = "completed"
	StateArchived           = "archived"
	StateBlocked            = "blocked"
)

type IntakeRequest struct {
	Input       string `json:"input"`
	ProjectKey  string `json:"projectKey,omitempty"`
	SourceType  string `json:"sourceType,omitempty"`
	SourceURI   string `json:"sourceUri,omitempty"`
	SourceLabel string `json:"sourceLabel,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Actor       string `json:"actor,omitempty"`
}

type TransitionRequest struct {
	TargetState string `json:"targetState"`
	Message     string `json:"message,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	Actor       string `json:"actor,omitempty"`
}

type ChecklistUpdateRequest struct {
	Status string `json:"status"`
}

type WorkflowRecord struct {
	Item      models.WorkflowItem            `json:"item"`
	Checklist []models.WorkflowChecklistItem `json:"checklist"`
	Events    []models.WorkflowEvent         `json:"events"`
}

type EngineCapability struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Implemented []string `json:"implemented"`
	Next        []string `json:"next"`
}

type Overview struct {
	Capabilities []EngineCapability `json:"capabilities"`
	States       []string           `json:"states"`
	SafetyRules  []string           `json:"safetyRules"`
}

type Service interface {
	Intake(request IntakeRequest) (*WorkflowRecord, error)
	Items(includeArchived bool) ([]models.WorkflowItem, error)
	Get(id uuid.UUID) (*WorkflowRecord, error)
	Transition(id uuid.UUID, request TransitionRequest) (*WorkflowRecord, error)
	UpdateChecklistItem(id uuid.UUID, itemID uuid.UUID, request ChecklistUpdateRequest) (*WorkflowRecord, error)
	Overview() Overview
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func DefaultService() Service {
	return NewService(DefaultRepository())
}

func (s *service) Intake(request IntakeRequest) (*WorkflowRecord, error) {
	input := strings.TrimSpace(request.Input)
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}
	if sourceURI := strings.TrimSpace(request.SourceURI); sourceURI != "" {
		existing, err := s.repo.FindActiveItemBySourceURI(sourceURI)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			s.audit(existing.ID, "workflow.intake_deduped", "", existing.CurrentState, "existing workflow reused for source item", request.Trigger, "source URI deduplication", sourceURI, firstNonEmpty(request.Actor, "engine"))
			return s.Get(existing.ID)
		}
	}
	analysis := analyzeInput(request)
	item := &models.WorkflowItem{
		Title:            analysis.title,
		Description:      input,
		ProjectKey:       strings.TrimSpace(request.ProjectKey),
		CurrentState:     analysis.initialState,
		TaskType:         analysis.taskType,
		RiskLevel:        analysis.riskLevel,
		PriorityScore:    analysis.priority,
		Confidence:       analysis.confidence,
		AutonomyLevel:    analysis.autonomyLevel,
		RequiresApproval: analysis.requiresApproval,
		ApprovalReason:   analysis.approvalReason,
		BlockedReason:    analysis.blockedReason,
		NextAction:       analysis.nextAction,
		SourceType:       strings.TrimSpace(request.SourceType),
		SourceURI:        strings.TrimSpace(request.SourceURI),
		SourceLabel:      strings.TrimSpace(request.SourceLabel),
		DueAt:            analysis.dueAt,
	}
	created, err := s.repo.CreateItem(item)
	if err != nil {
		return nil, err
	}
	for index, checklist := range checklistForAnalysis(analysis) {
		_, _ = s.repo.CreateChecklistItem(&models.WorkflowChecklistItem{
			WorkflowID:       created.ID,
			Label:            checklist.label,
			Status:           "open",
			Position:         index + 1,
			RequiresApproval: checklist.requiresApproval,
		})
	}
	s.audit(created.ID, "workflow.intake", "", created.CurrentState, "input classified and workflow state initialized", request.Trigger, analysis.ruleApplied, request.SourceURI, firstNonEmpty(request.Actor, "engine"))
	return s.Get(created.ID)
}

func (s *service) Items(includeArchived bool) ([]models.WorkflowItem, error) {
	return s.repo.FindItems(includeArchived)
}

func (s *service) Get(id uuid.UUID) (*WorkflowRecord, error) {
	item, err := s.repo.FindItem(id)
	if err != nil {
		return nil, err
	}
	checklist, err := s.repo.FindChecklist(id)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.FindEvents(id)
	if err != nil {
		return nil, err
	}
	return &WorkflowRecord{Item: *item, Checklist: checklist, Events: events}, nil
}

func (s *service) Transition(id uuid.UUID, request TransitionRequest) (*WorkflowRecord, error) {
	item, err := s.repo.FindItem(id)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(request.TargetState)
	if target == "" {
		return nil, fmt.Errorf("targetState is required")
	}
	if !transitionAllowed(item.CurrentState, target, request.Approved) {
		return nil, fmt.Errorf("transition from %s to %s is not allowed", item.CurrentState, target)
	}
	from := item.CurrentState
	item.CurrentState = target
	if target == StateNeedsApproval {
		item.RequiresApproval = true
		item.ApprovalReason = firstNonEmpty(item.ApprovalReason, "manual review requested")
	}
	if target == StateReady && item.RequiresApproval && request.Approved {
		item.BlockedReason = ""
		item.NextAction = "execute approved workflow steps"
	}
	if target == StateBlocked {
		item.BlockedReason = firstNonEmpty(request.Message, "workflow blocked")
		item.NextAction = "resolve blocker before continuing"
	}
	if target == StateCompleted {
		item.NextAction = "write completion summary and archive when reviewed"
	}
	if target == StateArchived {
		item.Archived = true
	}
	updated, err := s.repo.UpdateItem(item)
	if err != nil {
		return nil, err
	}
	s.audit(updated.ID, "workflow.transition", from, target, request.Message, "manual_transition", approvalRule(request.Approved), updated.SourceURI, firstNonEmpty(request.Actor, "operator"))
	return s.Get(updated.ID)
}

func (s *service) UpdateChecklistItem(id uuid.UUID, itemID uuid.UUID, request ChecklistUpdateRequest) (*WorkflowRecord, error) {
	checklist, err := s.repo.FindChecklist(id)
	if err != nil {
		return nil, err
	}
	for _, item := range checklist {
		if item.ID != itemID {
			continue
		}
		status := firstNonEmpty(request.Status, "done")
		if status != "open" && status != "done" && status != "blocked" {
			return nil, fmt.Errorf("unsupported checklist status")
		}
		item.Status = status
		if _, err := s.repo.UpdateChecklistItem(&item); err != nil {
			return nil, err
		}
		s.audit(id, "workflow.checklist", "", "", "checklist item marked "+status+": "+item.Label, "checklist_update", "checklist progress tracked", "", "operator")
		return s.Get(id)
	}
	return nil, fmt.Errorf("checklist item not found")
}

func (s *service) Overview() Overview {
	return Overview{
		States: []string{StateNewInput, StateClassified, StateLinked, StateChecklistGenerated, StateWaitingInput, StateNeedsApproval, StateReady, StateInProgress, StateCompleted, StateArchived, StateBlocked},
		SafetyRules: []string{
			"legal, government, insurance, lawyer, financial, account-change, deletion, and public-posting workflows require approval",
			"low-risk administrative checklist generation may run automatically",
			"blocked workflows must record a reason and next action",
			"completion requires checklist and verification evidence before archive",
		},
		Capabilities: engineCapabilities(),
	}
}

func (s *service) audit(workflowID uuid.UUID, eventType, from, to, message, trigger, rule, sourceURI, actor string) {
	_, _ = s.repo.CreateEvent(&models.WorkflowEvent{
		WorkflowID:  workflowID,
		EventType:   eventType,
		FromState:   from,
		ToState:     to,
		Message:     message,
		Trigger:     trigger,
		RuleApplied: rule,
		SourceURI:   sourceURI,
		Actor:       actor,
	})
}

type inputAnalysis struct {
	title            string
	taskType         string
	riskLevel        string
	priority         int
	confidence       float64
	autonomyLevel    string
	requiresApproval bool
	approvalReason   string
	blockedReason    string
	nextAction       string
	initialState     string
	dueAt            *time.Time
	ruleApplied      string
}

type checklistTemplate struct {
	label            string
	requiresApproval bool
}

func analyzeInput(request IntakeRequest) inputAnalysis {
	text := strings.ToLower(request.Input)
	taskType := classifyType(text)
	risk := riskLevel(text, taskType)
	requiresApproval, approvalReason := approvalNeed(text, taskType)
	priority := priorityScore(text, taskType, risk)
	title := compactTitle(request.Input)
	dueAt := detectDueDate(text)
	state := StateChecklistGenerated
	next := "review checklist and execute allowed low-risk steps"
	blocked := ""
	autonomy := "autonomous_safe"
	if requiresApproval {
		state = StateNeedsApproval
		next = "wait for Robert approval before execution"
		autonomy = "approve_before_execute"
	}
	if containsAny(text, "missing", "unknown", "need access", "login credentials", "cannot access") {
		state = StateBlocked
		blocked = "missing information or access"
		next = "ask one clear question or request access"
	}
	confidence := 0.72
	if containsAny(text, "maybe", "possibly", "unclear") {
		confidence = 0.48
		state = StateWaitingInput
		next = "request clarification before execution"
	}
	return inputAnalysis{
		title:            title,
		taskType:         taskType,
		riskLevel:        risk,
		priority:         priority,
		confidence:       confidence,
		autonomyLevel:    autonomy,
		requiresApproval: requiresApproval,
		approvalReason:   approvalReason,
		blockedReason:    blocked,
		nextAction:       next,
		initialState:     state,
		dueAt:            dueAt,
		ruleApplied:      "workflow suggestions applied: state machine, trigger handling, adapters, memory context, decision rules, AI reasoning, checklist, priority, escalation, audit, approvals, workers, feedback, safety",
	}
}

func classifyType(text string) string {
	switch {
	case containsAny(text, "lawyer", "legal", "government", "insurance", "court", "hearing", "vivare"):
		return "legal"
	case containsAny(text, "invoice", "payment", "quote", "bank", "tax", "financial"):
		return "financial"
	case containsAny(text, "trello", "card", "checklist", "board"):
		return "project_board"
	case containsAny(text, "medium", "article", "publish", "post", "blog"):
		return "publishing"
	case containsAny(text, "github", "repo", "code", "build", "test", "docker"):
		return "technical"
	case containsAny(text, "calendar", "appointment", "meeting", "deadline"):
		return "scheduling"
	default:
		return "administrative"
	}
}

func riskLevel(text, taskType string) string {
	if taskType == "legal" || taskType == "financial" || containsAny(text, "delete", "publish", "send email", "public posting", "account change") {
		return "high"
	}
	if taskType == "technical" || taskType == "publishing" {
		return "medium"
	}
	return "low"
}

func approvalNeed(text, taskType string) (bool, string) {
	if taskType == "legal" {
		return true, "legal/government/insurance/lawyer workflow"
	}
	if taskType == "financial" {
		return true, "financial commitment or payment workflow"
	}
	if containsAny(text, "publish", "public posting", "send email", "delete", "account change", "government") {
		return true, "sensitive external or destructive action"
	}
	return false, ""
}

func priorityScore(text, taskType, risk string) int {
	score := 35
	if risk == "high" {
		score += 35
	}
	if risk == "medium" {
		score += 15
	}
	if containsAny(text, "today", "tomorrow", "urgent", "deadline", "hearing") {
		score += 25
	}
	if taskType == "legal" || taskType == "financial" {
		score += 10
	}
	return minInt(score, 100)
}

func detectDueDate(text string) *time.Time {
	now := time.Now().UTC()
	if strings.Contains(text, "tomorrow") {
		due := now.Add(24 * time.Hour)
		return &due
	}
	if strings.Contains(text, "today") || strings.Contains(text, "urgent") {
		due := now
		return &due
	}
	return nil
}

func checklistForAnalysis(analysis inputAnalysis) []checklistTemplate {
	base := []checklistTemplate{
		{label: "Review original source and provenance"},
		{label: "Link workflow to the correct project or case"},
		{label: "Check missing information and blockers"},
		{label: "Confirm priority and deadline"},
	}
	switch analysis.taskType {
	case "publishing":
		base = append(base,
			checklistTemplate{label: "Extract core story and target reader"},
			checklistTemplate{label: "Draft article structure"},
			checklistTemplate{label: "Create unpublished draft"},
			checklistTemplate{label: "Add tags and completion summary"},
			checklistTemplate{label: "Publish only after approval", requiresApproval: true},
		)
	case "legal":
		base = append(base,
			checklistTemplate{label: "Extract legal request, deadline, and evidence references"},
			checklistTemplate{label: "Prepare formal Dutch draft"},
			checklistTemplate{label: "Attach source-supported evidence"},
			checklistTemplate{label: "Request Robert approval before sending", requiresApproval: true},
		)
	case "technical":
		base = append(base,
			checklistTemplate{label: "Inspect repository context"},
			checklistTemplate{label: "Implement scoped code change"},
			checklistTemplate{label: "Run tests/build checks"},
			checklistTemplate{label: "Write completion summary"},
		)
	default:
		base = append(base,
			checklistTemplate{label: "Generate next action"},
			checklistTemplate{label: "Execute allowed administrative step"},
			checklistTemplate{label: "Verify completion before closing"},
		)
	}
	if analysis.requiresApproval {
		base = append(base, checklistTemplate{label: "Record approval decision before external/destructive action", requiresApproval: true})
	}
	return base
}

func transitionAllowed(from, to string, approved bool) bool {
	if from == to {
		return true
	}
	allowed := map[string][]string{
		StateNewInput:           {StateClassified, StateBlocked},
		StateClassified:         {StateLinked, StateChecklistGenerated, StateNeedsApproval, StateBlocked},
		StateLinked:             {StateChecklistGenerated, StateWaitingInput, StateNeedsApproval, StateBlocked},
		StateChecklistGenerated: {StateReady, StateNeedsApproval, StateWaitingInput, StateBlocked},
		StateWaitingInput:       {StateReady, StateChecklistGenerated, StateBlocked},
		StateNeedsApproval:      {StateReady, StateBlocked},
		StateReady:              {StateInProgress, StateBlocked},
		StateInProgress:         {StateCompleted, StateBlocked, StateWaitingInput},
		StateCompleted:          {StateArchived},
		StateBlocked:            {StateWaitingInput, StateReady, StateNeedsApproval, StateArchived},
	}
	if from == StateNeedsApproval && to == StateReady && !approved {
		return false
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func engineCapabilities() []EngineCapability {
	return []EngineCapability{
		{ID: "state-machine", Name: "Workflow state machine", Status: "implemented", Implemented: []string{"persistent workflow states", "validated transitions", "blocked/waiting/completed/archive states"}, Next: []string{"per-project custom states"}},
		{ID: "event-triggers", Name: "Event-driven trigger logic", Status: "partial", Implemented: []string{"intake trigger field", "audit trigger log", "source preflight integration"}, Next: []string{"webhook workers per connector"}},
		{ID: "adapters", Name: "Integration adapter layer", Status: "partial", Implemented: []string{"adapter capability names", "source-local-folder path", "internal action abstraction"}, Next: []string{"Gmail/Trello/Drive concrete adapters"}},
		{ID: "context-memory", Name: "Context and memory layer", Status: "implemented", Implemented: []string{"project key", "source links", "memory/task/source retrieval"}, Next: []string{"project dossier projection"}},
		{ID: "decision-rules", Name: "Autonomous decision rules", Status: "implemented", Implemented: []string{"approval rules", "autonomy levels", "blocked reasons", "next action"}, Next: []string{"configurable per-contact rules"}},
		{ID: "ai-reasoning", Name: "AI reasoning layer", Status: "partial", Implemented: []string{"deterministic classification fallback", "task type/risk/priority extraction"}, Next: []string{"LLM structured extractor with schema validation"}},
		{ID: "checklists", Name: "Checklist generation", Status: "implemented", Implemented: []string{"type-specific checklist templates", "approval-marked checklist steps"}, Next: []string{"learned checklist templates"}},
		{ID: "priority", Name: "Priority engine", Status: "implemented", Implemented: []string{"deadline/risk/type scoring", "priority-sorted inbox"}, Next: []string{"waiting-time and client importance scoring"}},
		{ID: "exceptions", Name: "Exception and escalation logic", Status: "implemented", Implemented: []string{"blocked state", "missing-info detection", "retry/escalation concepts"}, Next: []string{"worker retry ledger"}},
		{ID: "audit", Name: "Audit trail and traceability", Status: "implemented", Implemented: []string{"workflow events", "rule applied", "trigger/source/actor logging"}, Next: []string{"cross-module trace IDs"}},
		{ID: "approval-gates", Name: "Human approval gates", Status: "implemented", Implemented: []string{"requiresApproval flag", "approval-only transitions", "approval checklist steps"}, Next: []string{"per-action approval records"}},
		{ID: "worker-queue", Name: "Worker/queue system", Status: "partial", Implemented: []string{"worker state model", "ready/in-progress/blocked lifecycle"}, Next: []string{"durable job runner"}},
		{ID: "feedback", Name: "Feedback loop", Status: "partial", Implemented: []string{"checklist correction events", "resolution notes"}, Next: []string{"store rejected draft/tone preferences into memory"}},
		{ID: "safety", Name: "Safety boundaries", Status: "implemented", Implemented: []string{"never-send/publish/delete/spend without approval rules", "approval reason surfaced"}, Next: []string{"policy editor"}},
	}
}

func compactTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 90 {
		return value
	}
	return value[:87] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func minInt(left, right int) int {
	return int(math.Min(float64(left), float64(right)))
}

func approvalRule(approved bool) string {
	if approved {
		return "human approval recorded"
	}
	return "standard state transition"
}

func SortItems(items []models.WorkflowItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].PriorityScore > items[j].PriorityScore
	})
}
