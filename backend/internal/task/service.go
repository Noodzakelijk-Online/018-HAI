package task

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"automation-hub-backend/internal/source"
	"automation-hub-backend/internal/verification"

	"github.com/google/uuid"
)

type IntakeRequest struct {
	OwnerIdentity   string   `json:"-"`
	PursuitID       string   `json:"pursuitId,omitempty"`
	Request         string   `json:"request"`
	ProjectKey      string   `json:"projectKey,omitempty"`
	AutomationID    string   `json:"automationId,omitempty"`
	SuccessCriteria []string `json:"successCriteria,omitempty"`
	ExecuteAllowed  bool     `json:"executeAllowed,omitempty"`
	HumanApproved   bool     `json:"humanApproved,omitempty"`
	ApprovalNote    string   `json:"approvalNote,omitempty"`
	reviewItemID    string
}

type IntakeAnalysis struct {
	TaskType            string   `json:"taskType"`
	RiskLevel           string   `json:"riskLevel"`
	Difficulty          int      `json:"difficulty"`
	RequiredReasoning   string   `json:"requiredReasoning"`
	SuccessCriteria     []string `json:"successCriteria"`
	NeedsMemory         bool     `json:"needsMemory"`
	NeedsTools          bool     `json:"needsTools"`
	NeedsDocuments      bool     `json:"needsDocuments"`
	NeedsWebAccess      bool     `json:"needsWebAccess"`
	NeedsLocalExecution bool     `json:"needsLocalExecution"`
	NeedsApproval       bool     `json:"needsApproval"`
	Reason              string   `json:"reason"`
}

type ContextPlan struct {
	Strategy                 []string                  `json:"strategy"`
	UsedContext              []memory.RankedMemory     `json:"usedContext"`
	SourceContext            []source.RankedExtraction `json:"sourceContext"`
	SourceRefresh            *source.ScheduledSyncRun  `json:"sourceRefresh,omitempty"`
	SourceRefreshExplanation string                    `json:"sourceRefreshExplanation,omitempty"`
	Explanation              string                    `json:"explanation"`
}

type ValidationPlan struct {
	Steps          []string `json:"steps"`
	FailurePolicy  string   `json:"failurePolicy"`
	CompletionGate string   `json:"completionGate"`
}

type ExecutionPlan struct {
	PlanningSeparatedFromExecution bool     `json:"planningSeparatedFromExecution"`
	ControlledExecutionMode        string   `json:"controlledExecutionMode"`
	ApprovalRequiredFor            []string `json:"approvalRequiredFor"`
	AuditEvents                    []string `json:"auditEvents"`
}

type ToolRouteDecision struct {
	SelectedTools []string `json:"selectedTools"`
	SkippedTools  []string `json:"skippedTools"`
	BlockedTools  []string `json:"blockedTools"`
	Reason        string   `json:"reason"`
}

type TaskStep struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Purpose          string `json:"purpose"`
	Allowed          bool   `json:"allowed"`
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
}

type RiskAssessment struct {
	Level            string   `json:"level"`
	ApprovalRequired bool     `json:"approvalRequired"`
	ApprovalGranted  bool     `json:"approvalGranted"`
	Reasons          []string `json:"reasons"`
	AllowedNow       bool     `json:"allowedNow"`
}

type ValidationResult struct {
	Passed        bool     `json:"passed"`
	Status        string   `json:"status"`
	Checked       []string `json:"checked"`
	Failures      []string `json:"failures"`
	NextAction    string   `json:"nextAction"`
	AttemptNumber int      `json:"attemptNumber"`
}

type RetryPolicy struct {
	MaxAttempts    int      `json:"maxAttempts"`
	EscalationPath []string `json:"escalationPath"`
	EscalateWhen   []string `json:"escalateWhen"`
	CurrentAttempt int      `json:"currentAttempt"`
	RetryAvailable bool     `json:"retryAvailable"`
}

type ReviewQueueItem struct {
	ID             string        `json:"id"`
	TaskID         string        `json:"taskId"`
	Request        IntakeRequest `json:"request"`
	Reason         string        `json:"reason"`
	Priority       string        `json:"priority"`
	Status         string        `json:"status"`
	Decision       string        `json:"decision,omitempty"`
	ResolutionNote string        `json:"resolutionNote,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
	ResolvedAt     *time.Time    `json:"resolvedAt,omitempty"`
}

type ApprovalDecision struct {
	Approved bool   `json:"approved"`
	Note     string `json:"note,omitempty"`
}

type ReviewResolutionResult struct {
	Item ReviewQueueItem `json:"item"`
	Plan *CompletionPlan `json:"plan,omitempty"`
}

type TaskEvent struct {
	At      time.Time `json:"at"`
	Stage   string    `json:"stage"`
	Message string    `json:"message"`
}

type ExecutedAction struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Input     string    `json:"input,omitempty"`
	Output    string    `json:"output,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type ToolExecutionRequest struct {
	AutomationID  string `json:"automationId"`
	Task          string `json:"task"`
	ProjectKey    string `json:"projectKey,omitempty"`
	HumanApproved bool   `json:"humanApproved"`
}

type ToolExecutionResult struct {
	AutomationID      string                              `json:"automationId"`
	LaunchEventID     string                              `json:"launchEventId,omitempty"`
	RuntimeType       string                              `json:"runtimeType,omitempty"`
	LaunchType        string                              `json:"launchType"`
	Target            string                              `json:"target,omitempty"`
	Status            string                              `json:"status"`
	Message           string                              `json:"message,omitempty"`
	Output            string                              `json:"output,omitempty"`
	RuntimeRouteTrace *models.AutomationRuntimeRouteTrace `json:"runtimeRouteTrace,omitempty"`
	ExitCode          int                                 `json:"exitCode"`
	DurationMs        int64                               `json:"durationMs"`
	RequiresApproval  bool                                `json:"requiresApproval"`
	AuditEvents       []string                            `json:"auditEvents"`
	ExecutedAt        time.Time                           `json:"executedAt"`
}

type ToolExecutor interface {
	Execute(request ToolExecutionRequest) (*ToolExecutionResult, error)
}

// PursuitAttemptRecorder stores a compact audit projection for task work that
// is explicitly scoped to a pursuit. Retrieved context and generated output
// stay in the existing task and verification paths.
type PursuitAttemptRecorder interface {
	UpsertTaskAttempt(attempt models.PursuitTaskAttempt) error
}

type ExecutionResult struct {
	StartedAt          time.Time                  `json:"startedAt"`
	CompletedAt        time.Time                  `json:"completedAt"`
	Mode               string                     `json:"mode"`
	Output             string                     `json:"output"`
	VerificationStatus string                     `json:"verificationStatus"`
	Claims             []models.VerificationClaim `json:"claims"`
	EvidenceCount      int                        `json:"evidenceCount"`
	UnsupportedClaims  int                        `json:"unsupportedClaims"`
	LLMGeneration      *llm.GenerationResult      `json:"llmGeneration,omitempty"`
	ToolExecution      *ToolExecutionResult       `json:"toolExecution,omitempty"`
	Actions            []ExecutedAction           `json:"actions"`
	BlockedReason      string                     `json:"blockedReason,omitempty"`
}

type MemoryUpdateProposal struct {
	Kind       string   `json:"kind"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags"`
	Reason     string   `json:"reason"`
	Confidence float64  `json:"confidence"`
}

type CompletionPlan struct {
	ID                    string                 `json:"id"`
	OwnerIdentity         string                 `json:"-"`
	PursuitID             string                 `json:"pursuitId,omitempty"`
	CreatedAt             time.Time              `json:"createdAt"`
	Request               string                 `json:"request"`
	ProjectKey            string                 `json:"projectKey,omitempty"`
	RealGoal              string                 `json:"realGoal"`
	Intake                IntakeAnalysis         `json:"intake"`
	ContextPlan           ContextPlan            `json:"contextPlan"`
	MinimalityDecision    MinimalityDecision     `json:"minimalityDecision"`
	ModelDecision         llm.RouteDecision      `json:"modelDecision"`
	ToolDecision          ToolRouteDecision      `json:"toolDecision"`
	Steps                 []TaskStep             `json:"steps"`
	RiskAssessment        RiskAssessment         `json:"riskAssessment"`
	ValidationPlan        ValidationPlan         `json:"validationPlan"`
	ValidationResult      ValidationResult       `json:"validationResult"`
	ExecutionPlan         ExecutionPlan          `json:"executionPlan"`
	ExecutionResult       *ExecutionResult       `json:"executionResult,omitempty"`
	RetryPolicy           RetryPolicy            `json:"retryPolicy"`
	ReviewQueueItem       *ReviewQueueItem       `json:"reviewQueueItem,omitempty"`
	MemoryUpdateProposals []MemoryUpdateProposal `json:"memoryUpdateProposals"`
	LessonsLearned        []MemoryUpdateProposal `json:"lessonsLearned"`
	StoredMemoryIDs       []string               `json:"storedMemoryIds"`
	Events                []TaskEvent            `json:"events"`
	CompletionStatus      string                 `json:"completionStatus"`
}

type Service interface {
	Plan(request IntakeRequest) (*CompletionPlan, error)
	Run(request IntakeRequest) (*CompletionPlan, error)
	Logs() []CompletionPlan
	ReviewQueue() []ReviewQueueItem
	ResolveReviewItem(id string, decision ApprovalDecision) (*ReviewResolutionResult, error)
}

// OwnerScopedService is the authenticated view over task history and approvals.
// It is intentionally separate from Service so background workers can retain
// their system-level access without becoming an HTTP data-leak path.
type OwnerScopedService interface {
	LogsForOwner(ownerIdentity string) []CompletionPlan
	ReviewQueueForOwner(ownerIdentity string) []ReviewQueueItem
	ResolveReviewItemForOwner(ownerIdentity, id string, decision ApprovalDecision) (*ReviewResolutionResult, error)
}

type service struct {
	memoryService       memory.Service
	sourceService       source.Service
	verificationService verification.Service
	llmService          *llm.Service
	toolExecutor        ToolExecutor
	pursuitAttempts     PursuitAttemptRecorder
	mu                  sync.Mutex
	logs                []CompletionPlan
	reviewQueue         []ReviewQueueItem
}

func NewService(memoryService memory.Service, llmService *llm.Service, sourceServices ...source.Service) Service {
	var sourceService source.Service
	if len(sourceServices) > 0 && sourceServices[0] != nil {
		sourceService = sourceServices[0]
	}
	return &service{
		memoryService: memoryService,
		sourceService: sourceService,
		llmService:    llmService,
		logs:          []CompletionPlan{},
		reviewQueue:   []ReviewQueueItem{},
	}
}

func NewServiceWithEngines(memoryService memory.Service, llmService *llm.Service, sourceService source.Service, verificationService verification.Service, toolExecutors ...ToolExecutor) Service {
	var toolExecutor ToolExecutor
	if len(toolExecutors) > 0 {
		toolExecutor = toolExecutors[0]
	}
	return &service{
		memoryService:       memoryService,
		sourceService:       sourceService,
		verificationService: verificationService,
		llmService:          llmService,
		toolExecutor:        toolExecutor,
		logs:                []CompletionPlan{},
		reviewQueue:         []ReviewQueueItem{},
	}
}

func NewServiceWithEnginesAndPursuitAttempts(memoryService memory.Service, llmService *llm.Service, sourceService source.Service, verificationService verification.Service, toolExecutor ToolExecutor, pursuitAttempts PursuitAttemptRecorder) Service {
	return &service{
		memoryService:       memoryService,
		sourceService:       sourceService,
		verificationService: verificationService,
		llmService:          llmService,
		toolExecutor:        toolExecutor,
		pursuitAttempts:     pursuitAttempts,
		logs:                []CompletionPlan{},
		reviewQueue:         []ReviewQueueItem{},
	}
}

func DefaultService() (Service, error) {
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		return nil, err
	}
	return NewServiceWithEngines(
		memory.DefaultService(),
		llmService,
		source.DefaultService(),
		verification.DefaultService(),
		NewAutomationToolExecutor(automation.DefaultService()),
	), nil
}

func (s *service) Plan(request IntakeRequest) (*CompletionPlan, error) {
	if err := s.validatePursuitAttemptRequest(request); err != nil {
		return nil, err
	}
	plan, err := s.buildPlan(request, false)
	if err != nil {
		return nil, err
	}
	if err := s.persistPursuitAttempt(plan, request, "plan", true); err != nil {
		return nil, err
	}
	s.addLog(*plan)
	return plan, nil
}

func (s *service) Run(request IntakeRequest) (*CompletionPlan, error) {
	if err := s.validatePursuitAttemptRequest(request); err != nil {
		return nil, err
	}
	if safety.EmergencyStopActive() {
		request.ExecuteAllowed = false
		request.HumanApproved = false
		plan, err := s.buildPlan(request, false)
		if err != nil {
			return nil, err
		}
		reason := safety.EmergencyStopReason()
		started := time.Now().UTC()
		plan.ExecutionResult = &ExecutionResult{
			StartedAt:     started,
			CompletedAt:   started,
			Mode:          "blocked",
			Output:        "Execution was blocked by emergency stop.",
			BlockedReason: reason,
			Actions:       []ExecutedAction{executedAction("governance.emergency_stop", "blocked", plan.Request, reason, started)},
		}
		plan.ValidationResult.Passed = false
		plan.ValidationResult.Status = "blocked"
		plan.ValidationResult.Failures = append(plan.ValidationResult.Failures, reason)
		plan.ValidationResult.NextAction = "clear emergency stop before autonomous execution"
		plan.CompletionStatus = "review_required"
		plan.Events = append(plan.Events, event("governance", reason))
		s.attachReviewItem(plan, reason, "high", request)
		if err := s.persistPursuitAttempt(plan, request, "run", true); err != nil {
			return nil, err
		}
		s.addLog(*plan)
		return plan, nil
	}
	plan, err := s.buildPlan(request, true)
	if err != nil {
		return nil, err
	}
	if err := s.persistPursuitAttempt(plan, request, "run", false); err != nil {
		return nil, err
	}
	if plan.RiskAssessment.AllowedNow {
		plan.ExecutionResult = s.executeAllowedSteps(plan, request)
	}
	plan.ValidationResult = validatePlan(plan, 1)
	plan.RetryPolicy.CurrentAttempt = 1
	plan.RetryPolicy.RetryAvailable = !plan.ValidationResult.Passed && plan.RetryPolicy.CurrentAttempt < plan.RetryPolicy.MaxAttempts

	if !plan.RiskAssessment.AllowedNow {
		plan.CompletionStatus = "review_required"
		plan.ValidationResult.Passed = false
		plan.ValidationResult.Status = "blocked"
		plan.ValidationResult.NextAction = "human review required before execution"
		s.attachReviewItem(plan, "approval required before task execution", plan.RiskAssessment.Level, request)
	} else if plan.ExecutionResult != nil && plan.ExecutionResult.BlockedReason != "" {
		plan.CompletionStatus = "review_required"
		plan.ValidationResult.Passed = false
		plan.ValidationResult.Status = "blocked"
		plan.ValidationResult.NextAction = "resolve the execution blocker before retrying"
		plan.RetryPolicy.RetryAvailable = false
		s.attachReviewItem(plan, plan.ExecutionResult.BlockedReason, plan.RiskAssessment.Level, request)
	} else if plan.ValidationResult.Passed {
		plan.CompletionStatus = "validated"
		plan.ValidationResult.Status = "passed"
		plan.ValidationResult.NextAction = "mark task complete"
		plan.Events = append(plan.Events, event("validation", "execution result verified against success criteria"))
		plan.StoredMemoryIDs = s.storeLessons(plan)
	} else if plan.RetryPolicy.RetryAvailable {
		plan.Events = append(plan.Events, event("retry", "validation failed; retrying with fallback model route"))
		failed := false
		routeRequest := llm.RouteRequest{
			Task:              plan.Request,
			TaskType:          plan.Intake.TaskType,
			Difficulty:        plan.Intake.Difficulty,
			RequiredReasoning: plan.Intake.RequiredReasoning,
			ValidationPassed:  &failed,
			PreviousModelID:   plan.ModelDecision.SelectedModelID,
		}
		if retryDecision, errRetry := s.llmService.Route(routeRequest); errRetry == nil {
			plan.ModelDecision = retryDecision
			plan.Events = append(plan.Events, event("routing", "fallback model route evaluated after validation failure"))
		}
		plan.ExecutionResult = s.executeAllowedSteps(plan, request)
		plan.RetryPolicy.CurrentAttempt = 2
		plan.ValidationResult = validatePlan(plan, 2)
		plan.RetryPolicy.RetryAvailable = !plan.ValidationResult.Passed && plan.RetryPolicy.CurrentAttempt < plan.RetryPolicy.MaxAttempts
		if plan.ValidationResult.Passed {
			plan.CompletionStatus = "validated"
			plan.ValidationResult.Status = "passed"
			plan.ValidationResult.NextAction = "mark task complete"
			plan.Events = append(plan.Events, event("validation", "retry validated against success criteria"))
			plan.StoredMemoryIDs = s.storeLessons(plan)
		} else if plan.RetryPolicy.RetryAvailable {
			plan.CompletionStatus = "retry_needed"
		} else {
			plan.CompletionStatus = "review_required"
			s.attachReviewItem(plan, "validation failed after retry", "medium", request)
		}
	} else {
		plan.CompletionStatus = "review_required"
		s.attachReviewItem(plan, "validation failed after available attempts", "medium", request)
	}

	if err := s.persistPursuitAttempt(plan, request, "run", true); err != nil {
		return nil, err
	}
	s.addLog(*plan)
	return plan, nil
}

func (s *service) validatePursuitAttemptRequest(request IntakeRequest) error {
	pursuitID := strings.TrimSpace(request.PursuitID)
	if pursuitID == "" {
		return nil
	}
	if _, err := uuid.Parse(pursuitID); err != nil {
		return fmt.Errorf("invalid pursuit id")
	}
	if s.pursuitAttempts == nil {
		return fmt.Errorf("pursuit task-attempt persistence is not configured")
	}
	return nil
}

func (s *service) persistPursuitAttempt(plan *CompletionPlan, request IntakeRequest, mode string, completed bool) error {
	if plan == nil || strings.TrimSpace(request.PursuitID) == "" {
		return nil
	}
	pursuitID, err := uuid.Parse(strings.TrimSpace(request.PursuitID))
	if err != nil {
		return fmt.Errorf("invalid pursuit id")
	}
	if s.pursuitAttempts == nil {
		return fmt.Errorf("pursuit task-attempt persistence is not configured")
	}
	startedAt := plan.CreatedAt
	if plan.ExecutionResult != nil && !plan.ExecutionResult.StartedAt.IsZero() {
		startedAt = plan.ExecutionResult.StartedAt
	}
	status := "running"
	if mode == "plan" {
		status = "planned"
	}
	if completed {
		status = firstNonEmpty(plan.CompletionStatus, status)
	}
	var completedAt *time.Time
	if completed {
		when := time.Now().UTC()
		if plan.ExecutionResult != nil && !plan.ExecutionResult.CompletedAt.IsZero() {
			when = plan.ExecutionResult.CompletedAt
		}
		completedAt = &when
	}
	verificationStatus := strings.TrimSpace(plan.ValidationResult.Status)
	blockedReason := strings.Join(compactStrings(plan.ValidationResult.Failures, 3), "; ")
	if plan.ExecutionResult != nil {
		verificationStatus = firstNonEmpty(plan.ExecutionResult.VerificationStatus, verificationStatus)
		blockedReason = firstNonEmpty(plan.ExecutionResult.BlockedReason, blockedReason)
	}
	attempt := models.PursuitTaskAttempt{
		PursuitID:          pursuitID,
		TaskPlanID:         plan.ID,
		OwnerIdentity:      strings.TrimSpace(plan.OwnerIdentity),
		RequestSummary:     compactTaskRequest(plan.RealGoal),
		ProjectKey:         strings.TrimSpace(plan.ProjectKey),
		Mode:               mode,
		Status:             status,
		RiskLevel:          strings.TrimSpace(plan.RiskAssessment.Level),
		VerificationStatus: verificationStatus,
		AutomationID:       firstNonEmpty(request.AutomationID, planAutomationID(plan)),
		LaunchEventID:      planLaunchEventID(plan),
		BlockedReason:      compactTaskRequest(blockedReason),
		StartedAt:          &startedAt,
		CompletedAt:        completedAt,
	}
	if err := s.pursuitAttempts.UpsertTaskAttempt(attempt); err != nil {
		return fmt.Errorf("persist pursuit task attempt: %w", err)
	}
	return nil
}

func compactTaskRequest(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(safety.RedactSecrets(value))), " ")
	const limit = 500
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit]) + "..."
}

func compactStrings(values []string, limit int) []string {
	if limit <= 0 {
		return []string{}
	}
	result := make([]string, 0, limit)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func planAutomationID(plan *CompletionPlan) string {
	if plan == nil || plan.ExecutionResult == nil || plan.ExecutionResult.ToolExecution == nil {
		return ""
	}
	return strings.TrimSpace(plan.ExecutionResult.ToolExecution.AutomationID)
}

func planLaunchEventID(plan *CompletionPlan) string {
	if plan == nil || plan.ExecutionResult == nil || plan.ExecutionResult.ToolExecution == nil {
		return ""
	}
	return strings.TrimSpace(plan.ExecutionResult.ToolExecution.LaunchEventID)
}

func (s *service) buildPlan(request IntakeRequest, runMode bool) (*CompletionPlan, error) {
	intake := analyzeIntake(request)
	sourceRefresh, sourceRefreshExplanation := s.refreshSourcesForTask(request, intake)
	contextResult, err := memory.RetrieveForOwner(s.memoryService, request.OwnerIdentity, memory.RetrieveRequest{
		Query:      request.Request,
		ProjectKey: request.ProjectKey,
		Limit:      8,
	})
	if err != nil {
		return nil, err
	}
	sourceContext, sourceExplanation := s.retrieveSourceContext(request)
	modelDecision, err := s.llmService.Route(llm.RouteRequest{
		Task:              request.Request,
		TaskType:          intake.TaskType,
		Difficulty:        intake.Difficulty,
		RequiredReasoning: intake.RequiredReasoning,
	})
	if err != nil {
		return nil, err
	}

	toolDecision := routeTools(intake)
	minimalityDecision := decideMinimality(request, intake)
	risk := assessRisk(intake, request.ExecuteAllowed, request.HumanApproved)
	steps := buildTaskSteps(intake, toolDecision, risk, minimalityDecision)
	validationPlan := buildValidationPlan(intake, minimalityDecision)
	memoryProposals := proposeMemoryUpdates(request, intake)
	plan := &CompletionPlan{
		ID:            uuid.New().String(),
		OwnerIdentity: strings.TrimSpace(request.OwnerIdentity),
		PursuitID:     strings.TrimSpace(request.PursuitID),
		CreatedAt:     time.Now().UTC(),
		Request:       request.Request,
		ProjectKey:    request.ProjectKey,
		RealGoal:      inferRealGoal(request, intake),
		Intake:        intake,
		ContextPlan: ContextPlan{
			Strategy: []string{
				"filter by project key when provided",
				"rank by keyword relevance, recency, confidence, and project match",
				"load only top relevant memories",
				"refresh due connected sources when the task likely depends on project, local, or document context",
				"check connected-source extractions before task planning",
				"preserve source references on returned memories",
			},
			UsedContext:              contextResult.UsedContext,
			SourceContext:            sourceContext,
			SourceRefresh:            sourceRefresh,
			SourceRefreshExplanation: sourceRefreshExplanation,
			Explanation:              strings.TrimSpace(contextResult.Explanation + " " + sourceRefreshExplanation + " " + sourceExplanation),
		},
		MinimalityDecision:    minimalityDecision,
		ModelDecision:         modelDecision,
		ToolDecision:          toolDecision,
		Steps:                 steps,
		RiskAssessment:        risk,
		ValidationPlan:        validationPlan,
		ValidationResult:      initialValidationResult(validationPlan),
		ExecutionPlan:         buildExecutionPlan(intake),
		RetryPolicy:           buildRetryPolicy(intake),
		MemoryUpdateProposals: memoryProposals,
		LessonsLearned:        proposeLessons(request, intake, toolDecision),
		Events: []TaskEvent{
			event("intake", "request classified and real goal inferred"),
			event("source-refresh", sourceRefreshExplanation),
			event("context", contextResult.Explanation),
			event("minimality", minimalityDecision.SelectedLevel+": "+minimalityDecision.Reason),
			event("routing", modelDecision.Reason),
			event("tool-routing", toolDecision.Reason),
			event("risk", strings.Join(risk.Reasons, "; ")),
		},
		CompletionStatus: "planned",
	}
	if request.HumanApproved {
		plan.Events = append(plan.Events, event("approval", "human approval recorded: "+compact(request.ApprovalNote)))
	}

	if runMode {
		plan.Events = append(plan.Events, event("execution", "only allowed low-risk planning and verification steps were executed"))
		for i := range plan.Steps {
			if plan.Steps[i].Allowed {
				plan.Steps[i].Status = "completed"
			}
		}
	}
	return plan, nil
}

func (s *service) Logs() []CompletionPlan {
	return s.LogsForOwner("")
}

func (s *service) LogsForOwner(ownerIdentity string) []CompletionPlan {
	s.mu.Lock()
	defer s.mu.Unlock()

	ownerIdentity = strings.TrimSpace(ownerIdentity)
	copied := make([]CompletionPlan, 0, len(s.logs))
	for _, plan := range s.logs {
		if ownerIdentity != "" && plan.OwnerIdentity != ownerIdentity {
			continue
		}
		copied = append(copied, plan)
	}
	return copied
}

func (s *service) ReviewQueue() []ReviewQueueItem {
	return s.ReviewQueueForOwner("")
}

func (s *service) ReviewQueueForOwner(ownerIdentity string) []ReviewQueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	ownerIdentity = strings.TrimSpace(ownerIdentity)
	copied := make([]ReviewQueueItem, 0, len(s.reviewQueue))
	for _, item := range s.reviewQueue {
		if ownerIdentity != "" && item.Request.OwnerIdentity != ownerIdentity {
			continue
		}
		copied = append(copied, item)
	}
	return copied
}

func (s *service) ResolveReviewItem(id string, decision ApprovalDecision) (*ReviewResolutionResult, error) {
	return s.resolveReviewItemForOwner("", id, decision)
}

func (s *service) ResolveReviewItemForOwner(ownerIdentity, id string, decision ApprovalDecision) (*ReviewResolutionResult, error) {
	return s.resolveReviewItemForOwner(ownerIdentity, id, decision)
}

func (s *service) resolveReviewItemForOwner(ownerIdentity, id string, decision ApprovalDecision) (*ReviewResolutionResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	s.mu.Lock()
	index := -1
	var item ReviewQueueItem
	for i := range s.reviewQueue {
		if s.reviewQueue[i].ID == id && (ownerIdentity == "" || s.reviewQueue[i].Request.OwnerIdentity == ownerIdentity) {
			index = i
			item = s.reviewQueue[i]
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("review item not found")
	}
	if item.Status != "open" && item.Status != "needs_review" {
		s.mu.Unlock()
		return nil, fmt.Errorf("review item is already resolved")
	}
	now := time.Now().UTC()
	item.ResolvedAt = &now
	item.ResolutionNote = strings.TrimSpace(decision.Note)
	if !decision.Approved {
		item.Status = "rejected"
		item.Decision = "rejected"
		s.reviewQueue[index] = item
		s.mu.Unlock()
		return &ReviewResolutionResult{Item: item}, nil
	}
	item.Status = "approved"
	item.Decision = "approved"
	s.reviewQueue[index] = item
	approvedRequest := item.Request
	approvedRequest.ExecuteAllowed = true
	approvedRequest.HumanApproved = true
	approvedRequest.ApprovalNote = item.ResolutionNote
	approvedRequest.reviewItemID = item.ID
	s.mu.Unlock()

	plan, err := s.Run(approvedRequest)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	completedAt := time.Now().UTC()
	if plan.CompletionStatus == "validated" {
		item.Status = "completed"
		item.ResolvedAt = &completedAt
	} else {
		item.Status = "needs_review"
		item.ResolvedAt = nil
		if plan.ReviewQueueItem != nil && plan.ReviewQueueItem.ID == item.ID {
			item.Reason = plan.ReviewQueueItem.Reason
		}
	}
	plan.ReviewQueueItem = &item
	for i := range s.reviewQueue {
		if s.reviewQueue[i].ID == item.ID {
			s.reviewQueue[i] = item
			break
		}
	}
	s.mu.Unlock()
	return &ReviewResolutionResult{Item: item, Plan: plan}, nil
}

func (s *service) addLog(plan CompletionPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = append([]CompletionPlan{plan}, s.logs...)
	if len(s.logs) > 50 {
		s.logs = s.logs[:50]
	}
}

func (s *service) addReviewItem(item ReviewQueueItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reviewQueue = append([]ReviewQueueItem{item}, s.reviewQueue...)
	if len(s.reviewQueue) > 50 {
		s.reviewQueue = s.reviewQueue[:50]
	}
}

func (s *service) attachReviewItem(plan *CompletionPlan, reason, risk string, request IntakeRequest) {
	if request.reviewItemID == "" {
		item := newReviewItem(plan.ID, reason, risk, request)
		plan.ReviewQueueItem = &item
		s.addReviewItem(item)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.reviewQueue {
		if s.reviewQueue[i].ID != request.reviewItemID {
			continue
		}
		item := s.reviewQueue[i]
		item.TaskID = plan.ID
		item.Request = request
		item.Reason = reason
		item.Priority = "normal"
		if risk == "high" {
			item.Priority = "high"
		}
		item.Status = "needs_review"
		item.ResolvedAt = nil
		s.reviewQueue[i] = item
		plan.ReviewQueueItem = &item
		return
	}

	item := newReviewItem(plan.ID, reason, risk, request)
	plan.ReviewQueueItem = &item
	s.reviewQueue = append([]ReviewQueueItem{item}, s.reviewQueue...)
	if len(s.reviewQueue) > 50 {
		s.reviewQueue = s.reviewQueue[:50]
	}
}

func (s *service) storeLessons(plan *CompletionPlan) []string {
	stored := []string{}
	if plan.ExecutionResult == nil || !verificationStatusAcceptsMemory(plan.ExecutionResult.VerificationStatus) {
		plan.Events = append(plan.Events, event("memory", "lesson storage skipped because execution was not verified"))
		return stored
	}
	for _, lesson := range plan.LessonsLearned {
		created, err := memory.CreateForOwner(s.memoryService, plan.OwnerIdentity, memory.CreateRequest{
			ProjectKey:  plan.ProjectKey,
			Kind:        lesson.Kind,
			Content:     lesson.Content,
			Summary:     compact(lesson.Content),
			Tags:        lesson.Tags,
			Confidence:  lesson.Confidence,
			SourceLabel: "task-success-engine",
		})
		if err == nil && created != nil {
			stored = append(stored, created.ID.String())
		}
	}
	if len(stored) > 0 {
		plan.Events = append(plan.Events, event("memory", "stored useful lessons for future tasks"))
	}
	return stored
}

func (s *service) executeAllowedSteps(plan *CompletionPlan, request IntakeRequest) *ExecutionResult {
	started := time.Now().UTC()
	result := &ExecutionResult{
		StartedAt:          started,
		Mode:               executionMode(plan, request),
		VerificationStatus: verification.StatusNeedsReview,
		Actions:            []ExecutedAction{},
	}

	if !plan.RiskAssessment.AllowedNow {
		result.CompletedAt = time.Now().UTC()
		result.BlockedReason = "risk gate blocked execution until approval is recorded"
		result.Output = "Execution was blocked before action because approval is required."
		result.Actions = append(result.Actions, executedAction("risk.approval_gate", "blocked", plan.Request, result.BlockedReason, started))
		plan.Events = append(plan.Events, event("execution", result.BlockedReason))
		return result
	}

	evidence := evidenceFromPlan(plan)
	if plan.Intake.NeedsTools || plan.Intake.NeedsLocalExecution {
		toolStarted := time.Now().UTC()
		toolResult := completedToolExecution(plan.ExecutionResult)
		if toolResult != nil {
			result.ToolExecution = toolResult
			result.Actions = append(result.Actions, executedAction(
				"automation.launch",
				"reused",
				toolResult.AutomationID,
				"reused successful runtime evidence without repeating external execution",
				toolStarted,
			))
			plan.Events = append(plan.Events, event("execution", "reused successful controlled-runtime evidence during validation retry"))
		} else {
			if s.toolExecutor == nil {
				return blockExecution(result, "controlled runtime executor is not configured", plan, toolStarted)
			}
			if strings.TrimSpace(request.AutomationID) == "" {
				return blockExecution(result, "task requires controlled runtime execution but no automationId was provided", plan, toolStarted)
			}
			executed, err := s.toolExecutor.Execute(ToolExecutionRequest{
				AutomationID:  request.AutomationID,
				Task:          plan.RealGoal,
				ProjectKey:    plan.ProjectKey,
				HumanApproved: plan.RiskAssessment.ApprovalGranted || !plan.RiskAssessment.ApprovalRequired,
			})
			if err != nil {
				return blockExecution(result, "controlled runtime execution failed: "+err.Error(), plan, toolStarted)
			}
			if executed == nil {
				return blockExecution(result, "controlled runtime execution returned no result", plan, toolStarted)
			}
			result.ToolExecution = executed
			result.Actions = append(result.Actions, executedAction("automation.launch", executed.Status, executed.AutomationID, firstNonEmpty(executed.Output, executed.Message), toolStarted))
			plan.Events = append(plan.Events, event("execution", "controlled automation runtime returned status "+executed.Status))
			if executed.Status != "completed" {
				reason := firstNonEmpty(executed.Message, "controlled runtime did not complete successfully")
				return blockExecution(result, reason, plan, toolStarted)
			}
		}
		evidence = append(evidence, toolExecutionEvidence(result.ToolExecution))
	}
	result.EvidenceCount = len(evidence)
	result.Actions = append(result.Actions,
		executedAction("memory.retrieve", "completed", request.Request, countLabel(len(plan.ContextPlan.UsedContext), "memory item"), started),
		executedAction("source.search", "completed", request.Request, countLabel(len(plan.ContextPlan.SourceContext), "source extraction"), started),
	)
	draft := ""
	generateStarted := time.Now().UTC()
	if s.llmService != nil {
		context := generationContext(plan)
		if result.ToolExecution != nil {
			context = append(context, toolExecutionSnippet(result.ToolExecution))
		}
		generation, err := s.llmService.Generate(llm.GenerateRequest{
			Task:         plan.RealGoal,
			SystemPrompt: "Produce a concise draft answer using only the provided context. Do not invent facts; unsupported details will be rejected by verification." + minimalitySystemContract(plan.MinimalityDecision),
			Context:      context,
			RouteRequest: &llm.RouteRequest{
				Task:              plan.Request,
				TaskType:          plan.Intake.TaskType,
				Difficulty:        plan.Intake.Difficulty,
				RequiredReasoning: plan.Intake.RequiredReasoning,
			},
			RouteDecision: &plan.ModelDecision,
			Temperature:   0.1,
			MaxTokens:     900,
		})
		if err == nil && generation != nil {
			result.LLMGeneration = generation
			if generation.Status == "completed" {
				draft = generation.Output
			}
			result.Actions = append(result.Actions, executedAction("llm.generate", generation.Status, plan.ModelDecision.SelectedModelID, generation.Reason, generateStarted))
			plan.Events = append(plan.Events, event("llm", "model generation "+generation.Status+": "+generation.Reason))
		} else if err != nil {
			result.Actions = append(result.Actions, executedAction("llm.generate", "failed", plan.ModelDecision.SelectedModelID, err.Error(), generateStarted))
			plan.Events = append(plan.Events, event("llm", "model generation failed; falling back to source-grounded evidence synthesis"))
		}
	}

	verifyStarted := time.Now().UTC()
	if s.verificationService == nil {
		result.Output, result.Claims, result.VerificationStatus = localGroundedResult(plan, evidence)
		result.UnsupportedClaims = unsupportedClaimCount(result.Claims)
		result.CompletedAt = time.Now().UTC()
		result.Actions = append(result.Actions, executedAction("verification.answer", "completed", request.Request, "used local evidence verifier", verifyStarted))
		plan.Events = append(plan.Events, event("execution", "produced grounded result from retrieved context"))
		return result
	}

	verificationResult, err := s.verificationService.Answer(verification.AnswerRequest{
		OwnerIdentity:     plan.OwnerIdentity,
		Question:          plan.RealGoal,
		ProjectKey:        plan.ProjectKey,
		Mode:              result.Mode,
		DraftAnswer:       draft,
		ExternalEvidence:  evidence,
		IncludeSensitive:  false,
		HumanApproved:     plan.RiskAssessment.ApprovalGranted || !plan.RiskAssessment.ApprovalRequired,
		AllowMemoryUpdate: false,
	})
	if err != nil {
		result.CompletedAt = time.Now().UTC()
		result.Output = "Verification engine failed before a grounded answer could be accepted: " + err.Error()
		result.VerificationStatus = verification.StatusNeedsReview
		result.BlockedReason = "verification engine unavailable"
		result.Actions = append(result.Actions, executedAction("verification.answer", "failed", request.Request, err.Error(), verifyStarted))
		plan.Events = append(plan.Events, event("verification", "verification engine failed; task requires review"))
		return result
	}

	result.Output = verificationResult.Run.Answer
	result.VerificationStatus = verificationResult.Run.Status
	result.Claims = verificationResult.Claims
	result.UnsupportedClaims = len(verificationResult.UnsupportedClaims)
	result.CompletedAt = time.Now().UTC()
	result.Actions = append(result.Actions, executedAction("verification.answer", "completed", request.Request, verificationResult.Run.Status, verifyStarted))
	plan.Events = append(plan.Events, event("verification", "claims were checked against retrieved evidence before completion"))
	return result
}

func completedToolExecution(previous *ExecutionResult) *ToolExecutionResult {
	if previous == nil || previous.ToolExecution == nil || previous.ToolExecution.Status != "completed" {
		return nil
	}
	copied := *previous.ToolExecution
	copied.AuditEvents = append([]string{}, previous.ToolExecution.AuditEvents...)
	copied.RuntimeRouteTrace = copyAutomationRuntimeRouteTrace(previous.ToolExecution.RuntimeRouteTrace)
	return &copied
}

func blockExecution(result *ExecutionResult, reason string, plan *CompletionPlan, started time.Time) *ExecutionResult {
	result.CompletedAt = time.Now().UTC()
	result.Output = "Execution stopped before completion: " + reason
	result.VerificationStatus = verification.StatusNeedsReview
	result.BlockedReason = reason
	if len(result.Actions) == 0 || result.Actions[len(result.Actions)-1].Name != "automation.launch" {
		result.Actions = append(result.Actions, executedAction("automation.launch", "blocked", plan.Request, reason, started))
	}
	plan.Events = append(plan.Events, event("execution", reason))
	return result
}

func toolExecutionEvidence(result *ToolExecutionResult) verification.EvidenceInput {
	sourceID := result.AutomationID
	sourceURI := "automation://" + result.AutomationID
	if strings.TrimSpace(result.LaunchEventID) != "" {
		sourceID = result.LaunchEventID
		sourceURI = "automation-launch://" + result.LaunchEventID
	}
	return verification.EvidenceInput{
		SourceType:  "controlled_runtime",
		SourceID:    sourceID,
		SourceURI:   sourceURI,
		SourceLabel: firstNonEmpty(result.RuntimeType, result.LaunchType, "controlled automation runtime"),
		Snippet:     toolExecutionSnippet(result),
		Authority:   "deterministic_runtime",
		Primary:     true,
	}
}

func toolExecutionSnippet(result *ToolExecutionResult) string {
	if result == nil {
		return ""
	}
	snippet := compact(firstNonEmpty(result.Output, result.Message, "controlled runtime completed successfully"))
	if route := runtimeRouteTraceSnippet(result.RuntimeRouteTrace); route != "" {
		return compact(snippet + " | " + route)
	}
	return snippet
}

func runtimeRouteTraceSnippet(trace *models.AutomationRuntimeRouteTrace) string {
	if trace == nil {
		return ""
	}
	parts := []string{}
	if value := strings.TrimSpace(trace.RuntimeID); value != "" {
		parts = append(parts, "runtime="+value)
	}
	if value := strings.TrimSpace(trace.Intent); value != "" {
		parts = append(parts, "intent="+value)
	}
	if value := strings.TrimSpace(trace.ExecutionMode); value != "" {
		parts = append(parts, "mode="+value)
	}
	if value := strings.TrimSpace(trace.RiskLevel); value != "" {
		parts = append(parts, "risk="+value)
	}
	if value := compactRuntimeRouteTraceList("skills", trace.RecommendedSkills, 3); value != "" {
		parts = append(parts, value)
	}
	if value := compactRuntimeRouteTraceList("providers", trace.VisibleProviders, 2); value != "" {
		parts = append(parts, value)
	}
	if value := compactRuntimeRouteTraceList("tools", trace.VisibleTools, 2); value != "" {
		parts = append(parts, value)
	}
	if value := compactRuntimeRouteTraceList("maps", trace.RelevantMaps, 2); value != "" {
		parts = append(parts, value)
	}
	if value := compactRuntimeRouteTraceList("blocked", trace.BlockedSurfaces, 3); value != "" {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return ""
	}
	return "route: " + strings.Join(parts, "; ")
}

func compactRuntimeRouteTraceList(label string, values []string, limit int) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(cleaned) {
		limit = len(cleaned)
	}
	summary := strings.Join(cleaned[:limit], ", ")
	if len(cleaned) > limit {
		summary += fmt.Sprintf(" +%d", len(cleaned)-limit)
	}
	return label + "=" + summary
}

func copyAutomationRuntimeRouteTrace(trace *models.AutomationRuntimeRouteTrace) *models.AutomationRuntimeRouteTrace {
	if trace == nil {
		return nil
	}
	return &models.AutomationRuntimeRouteTrace{
		RuntimeID:           trace.RuntimeID,
		Intent:              trace.Intent,
		ExecutionMode:       trace.ExecutionMode,
		RiskLevel:           trace.RiskLevel,
		RecommendedSkills:   append([]string{}, trace.RecommendedSkills...),
		VisibleProviders:    append([]string{}, trace.VisibleProviders...),
		VisibleTools:        append([]string{}, trace.VisibleTools...),
		RelevantMaps:        append([]string{}, trace.RelevantMaps...),
		BlockedSurfaces:     append([]string{}, trace.BlockedSurfaces...),
		RequiredControls:    append([]string{}, trace.RequiredControls...),
		ValidationChecklist: append([]string{}, trace.ValidationChecklist...),
	}
}

func executionMode(plan *CompletionPlan, request IntakeRequest) string {
	if plan.Intake.NeedsApproval || plan.Intake.NeedsTools || request.ExecuteAllowed {
		return verification.ModeAction
	}
	return verification.ModeGrounded
}

func evidenceFromPlan(plan *CompletionPlan) []verification.EvidenceInput {
	evidence := []verification.EvidenceInput{}
	for _, ranked := range plan.ContextPlan.UsedContext {
		mem := ranked.Memory
		snippet := firstNonEmpty(mem.Summary, mem.Content)
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		evidence = append(evidence, verification.EvidenceInput{
			SourceType:  "memory",
			SourceID:    mem.ID.String(),
			SourceURI:   mem.SourceURI,
			SourceLabel: firstNonEmpty(mem.SourceLabel, mem.Kind, "context memory"),
			Snippet:     snippet,
			Authority:   "local_memory",
			Primary:     true,
		})
	}
	for _, ranked := range plan.ContextPlan.SourceContext {
		extraction := ranked.Extraction
		snippet := firstNonEmpty(extraction.Summary, extraction.Text)
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		evidence = append(evidence, verification.EvidenceInput{
			SourceType:  "connected_source",
			SourceID:    extraction.ID.String(),
			SourceURI:   extraction.SourceURI,
			SourceLabel: firstNonEmpty(extraction.SourceLabel, extraction.ContentType, "connected source"),
			Snippet:     snippet,
			Authority:   "connected_account",
			Primary:     true,
		})
	}
	return evidence
}

func generationContext(plan *CompletionPlan) []string {
	context := []string{}
	for _, ranked := range plan.ContextPlan.UsedContext {
		snippet := firstNonEmpty(ranked.Memory.Summary, ranked.Memory.Content)
		if strings.TrimSpace(snippet) != "" {
			context = append(context, compact(snippet))
		}
	}
	for _, ranked := range plan.ContextPlan.SourceContext {
		snippet := firstNonEmpty(ranked.Extraction.Summary, ranked.Extraction.Text)
		if strings.TrimSpace(snippet) != "" {
			context = append(context, compact(snippet))
		}
	}
	return context
}

func localGroundedResult(plan *CompletionPlan, evidence []verification.EvidenceInput) (string, []models.VerificationClaim, string) {
	if len(evidence) == 0 {
		return "No grounded answer can be produced because no supporting context or source evidence was retrieved.", []models.VerificationClaim{
			{
				ID:                 uuid.New(),
				ClaimText:          "No supporting context or source evidence was retrieved.",
				Status:             verification.StatusNeedsReview,
				SupportExplanation: "task output requires evidence before it can be accepted",
				Confidence:         0.1,
				NeedsReview:        true,
			},
		}, verification.StatusNeedsReview
	}

	lines := []string{}
	claims := []models.VerificationClaim{}
	for _, item := range evidence {
		lines = append(lines, compact(item.Snippet))
		claims = append(claims, models.VerificationClaim{
			ID:                 uuid.New(),
			ClaimText:          compact(item.Snippet),
			Status:             verification.StatusSourceSupported,
			SourceRefs:         firstNonEmpty(item.SourceURI, item.SourceID, item.SourceLabel),
			SupportExplanation: "claim is directly derived from retrieved task context",
			Confidence:         0.7,
		})
		if len(lines) >= 5 {
			break
		}
	}
	return strings.Join(lines, ". "), claims, verification.StatusSourceSupported
}

func executedAction(name, status, input, output string, started time.Time) ExecutedAction {
	return ExecutedAction{
		Name:      name,
		Status:    status,
		Input:     compact(input),
		Output:    compact(output),
		StartedAt: started,
		EndedAt:   time.Now().UTC(),
	}
}

func countLabel(count int, label string) string {
	if count == 1 {
		return "1 " + label
	}
	return strconv.Itoa(count) + " " + label + "s"
}

func (s *service) refreshSourcesForTask(request IntakeRequest, intake IntakeAnalysis) (*source.ScheduledSyncRun, string) {
	if s.sourceService == nil {
		return nil, "Connected-source refresh is not configured."
	}
	if !shouldRefreshSourcesForTask(request, intake) {
		return nil, "Connected-source refresh skipped because the task does not appear to need source-backed context."
	}
	if strings.TrimSpace(request.OwnerIdentity) != "" {
		result, err := s.sourceService.RunDueScheduledSyncsForOwner(time.Now().UTC(), request.OwnerIdentity)
		if err != nil {
			return nil, "Owner-scoped connected-source refresh failed before context retrieval: " + err.Error()
		}
		return result, fmt.Sprintf("Owner-scoped connected-source preflight checked %d sources; %d due, %d completed, %d failed, %d skipped.", result.Checked, result.Due, result.Completed, result.Failed, result.Skipped)
	}
	result, err := s.sourceService.RunDueScheduledSyncs(time.Now().UTC())
	if err != nil {
		return nil, "Connected-source refresh failed before context retrieval: " + err.Error()
	}
	return result, fmt.Sprintf("Connected-source preflight checked %d sources; %d due, %d completed, %d failed, %d skipped.", result.Checked, result.Due, result.Completed, result.Failed, result.Skipped)
}

func shouldRefreshSourcesForTask(request IntakeRequest, intake IntakeAnalysis) bool {
	text := strings.ToLower(request.Request + " " + request.ProjectKey)
	if intake.NeedsDocuments || intake.NeedsLocalExecution {
		return true
	}
	return containsAny(text, "source", "context", "project", "file", "folder", "document", "github", "email", "calendar", "trello", "board", "repo")
}

func unsupportedClaimCount(claims []models.VerificationClaim) int {
	count := 0
	for _, claim := range claims {
		if claim.NeedsReview || !verificationStatusAcceptsCompletion(claim.Status) {
			count++
		}
	}
	return count
}

func (s *service) retrieveSourceContext(request IntakeRequest) ([]source.RankedExtraction, string) {
	if s.sourceService == nil {
		return []source.RankedExtraction{}, "Connected-source retrieval is not configured."
	}
	result, err := s.sourceService.Search(source.SearchRequest{
		OwnerIdentity: request.OwnerIdentity,
		Query:         request.Request,
		ProjectKey:    request.ProjectKey,
		Limit:         6,
	})
	if err != nil {
		return []source.RankedExtraction{}, "Connected-source retrieval failed or has no available index."
	}
	return result.UsedContext, result.Explanation
}

func analyzeIntake(request IntakeRequest) IntakeAnalysis {
	text := strings.ToLower(request.Request)
	taskType := "general"
	difficulty := 2
	reasoning := "medium"
	risk := "low"
	reasons := []string{"default completion-first intake"}

	needsTools := requiresControlledExecution(text)
	needsDocs := containsAny(text, "document", "pdf", "spreadsheet", "slides", "docx")
	needsWeb := containsAny(text, "latest", "current", "today", "web", "browse", "search")
	needsLocal := needsTools && containsWordOrPhrase(text, "local", "file", "files", "repo", "repository", "docker", "windows", "code", "build", "test", "tests", "script", "command", "commit", "push")

	if containsWordOrPhrase(
		text,
		"code", "coding", "bug", "api", "compile", "build", "test", "tests",
		"implement", "implementation", "refactor", "function", "package", "dependency",
		"library", "json", "parser", "endpoint", "repository", "golang", "go",
	) {
		taskType = "coding"
		difficulty = maxInt(difficulty, 3)
		reasoning = maxReasoning(reasoning, "medium")
		reasons = append(reasons, "coding/build terms detected")
	}
	if containsWordOrPhrase(
		text,
		"architecture", "blueprint", "multi-agent", "autonomous", "routing",
		"new service", "new module", "new runtime", "new adapter", "new connector",
	) {
		taskType = "architecture"
		difficulty = maxInt(difficulty, 4)
		reasoning = maxReasoning(reasoning, "high")
		reasons = append(reasons, "architecture terms detected")
	}
	if containsAny(text, "delete", "financial", "legal", "government", "email sending", "public posting", "account change") {
		risk = "high"
		difficulty = maxInt(difficulty, 4)
		reasoning = maxReasoning(reasoning, "high")
		reasons = append(reasons, "approval-sensitive terms detected")
	}

	successCriteria := request.SuccessCriteria
	if len(successCriteria) == 0 {
		successCriteria = inferSuccessCriteria(taskType, needsTools)
	}

	return IntakeAnalysis{
		TaskType:            taskType,
		RiskLevel:           risk,
		Difficulty:          difficulty,
		RequiredReasoning:   reasoning,
		SuccessCriteria:     successCriteria,
		NeedsMemory:         true,
		NeedsTools:          needsTools,
		NeedsDocuments:      needsDocs,
		NeedsWebAccess:      needsWeb,
		NeedsLocalExecution: needsLocal,
		NeedsApproval:       risk == "high",
		Reason:              strings.Join(reasons, "; "),
	}
}

func buildValidationPlan(intake IntakeAnalysis, minimality MinimalityDecision) ValidationPlan {
	steps := []string{
		"check every explicit success criterion",
		"verify required fields are present",
		"confirm context sources used are relevant",
	}
	if intake.TaskType == "coding" {
		steps = append(steps, "run applicable build and test commands")
	}
	if minimality.Applicable {
		steps = append(steps,
			"verify the implementation follows the selected YAGNI ladder rung",
			"reject new dependencies or abstractions without explicit evidence that simpler rungs are insufficient",
		)
	}
	if intake.NeedsWebAccess {
		steps = append(steps, "verify time-sensitive claims against current sources")
	}
	if intake.NeedsApproval {
		steps = append(steps, "pause before high-risk execution until human approval is recorded")
	}
	return ValidationPlan{
		Steps:          steps,
		FailurePolicy:  "retry with stronger context or model; escalate to human review if validation still fails",
		CompletionGate: "task is complete only after validation passes against success criteria",
	}
}

func buildExecutionPlan(intake IntakeAnalysis) ExecutionPlan {
	approval := []string{}
	if intake.NeedsApproval {
		approval = append(approval, "high-risk action")
	}
	if intake.NeedsLocalExecution {
		approval = append(approval, "destructive local execution")
	}
	return ExecutionPlan{
		PlanningSeparatedFromExecution: true,
		ControlledExecutionMode:        "plan_first_then_execute_with_validation",
		ApprovalRequiredFor:            approval,
		AuditEvents: []string{
			"intake classified",
			"context retrieved",
			"model selected",
			"execution attempted",
			"validation completed",
			"memory update proposed",
		},
	}
}

func routeTools(intake IntakeAnalysis) ToolRouteDecision {
	selected := []string{"memory.retrieve", "llm.route", "validator.criteria"}
	skipped := []string{}
	blocked := []string{}
	reasons := []string{"selected tools needed for verified completion"}

	if intake.NeedsTools {
		selected = append(selected, "tool-router")
	}
	if intake.NeedsDocuments {
		selected = append(selected, "document-context-reader")
	} else {
		skipped = append(skipped, "document-context-reader: task does not require documents")
	}
	if intake.NeedsWebAccess {
		selected = append(selected, "web-verification")
	} else {
		skipped = append(skipped, "web-verification: task is not time-sensitive")
	}
	if intake.NeedsLocalExecution {
		selected = append(selected, "local-readonly-executor")
		blocked = append(blocked, "destructive-local-executor: approval required")
		reasons = append(reasons, "local execution limited to read-only or explicitly approved steps")
	}
	if intake.NeedsApproval {
		blocked = append(blocked, "public-posting", "email-sending", "financial-actions", "account-changes", "delete-actions")
		reasons = append(reasons, "high-risk tools blocked until human approval")
	}

	return ToolRouteDecision{
		SelectedTools: uniqueStrings(selected),
		SkippedTools:  uniqueStrings(skipped),
		BlockedTools:  uniqueStrings(blocked),
		Reason:        strings.Join(reasons, "; "),
	}
}

func assessRisk(intake IntakeAnalysis, executeAllowed bool, humanApproved bool) RiskAssessment {
	reasons := []string{"read-only planning is allowed"}
	allowed := true
	approvalGranted := false
	needsExplicitExecution := intake.NeedsTools || intake.NeedsLocalExecution
	if intake.NeedsApproval {
		reasons = append(reasons, "request contains high-risk action terms")
		approvalGranted = executeAllowed && humanApproved
		allowed = approvalGranted
		if approvalGranted {
			reasons = append(reasons, "human approval recorded for this run")
		}
	}
	if intake.NeedsLocalExecution {
		reasons = append(reasons, "local execution is constrained to non-destructive steps")
	}
	if executeAllowed && !intake.NeedsApproval {
		reasons = append(reasons, "caller allowed low-risk execution")
	}
	if !executeAllowed && (intake.NeedsTools || intake.NeedsLocalExecution) {
		reasons = append(reasons, "execution not requested; plan remains non-executing")
	}
	allowedNow := allowed
	if needsExplicitExecution && !executeAllowed {
		allowedNow = false
	}
	return RiskAssessment{
		Level:            intake.RiskLevel,
		ApprovalRequired: intake.NeedsApproval,
		ApprovalGranted:  approvalGranted,
		Reasons:          reasons,
		AllowedNow:       allowedNow,
	}
}

func buildTaskSteps(intake IntakeAnalysis, tools ToolRouteDecision, risk RiskAssessment, minimality MinimalityDecision) []TaskStep {
	steps := []TaskStep{
		{ID: "understand", Name: "Understand request", Purpose: "identify the user's real goal", Allowed: true, Status: "planned"},
		{ID: "criteria", Name: "Define success criteria", Purpose: "make completion measurable", Allowed: true, Status: "planned"},
		{ID: "minimality", Name: "Apply YAGNI gate", Purpose: "select the least complex capable implementation strategy", Allowed: minimality.Necessary, RequiresApproval: !minimality.Necessary, Status: "planned"},
		{ID: "context", Name: "Gather context", Purpose: "retrieve only relevant memories and references", Allowed: true, Status: "planned"},
		{ID: "routing", Name: "Choose model and tools", Purpose: "select capable resources before optimizing cost", Allowed: true, Status: "planned"},
		{ID: "plan", Name: "Create plan", Purpose: "sequence safe actions and validation", Allowed: true, Status: "planned"},
		{ID: "risk", Name: "Check risk and approvals", Purpose: "block risky actions before execution", Allowed: true, Status: "planned"},
	}
	blockedHighRisk := len(tools.BlockedTools) > 0 && risk.ApprovalRequired && !risk.ApprovalGranted
	executionAllowed := risk.AllowedNow && !blockedHighRisk
	steps = append(steps,
		TaskStep{ID: "execute", Name: "Execute allowed steps", Purpose: "perform only approved or low-risk actions", Allowed: executionAllowed, RequiresApproval: !executionAllowed, Status: "planned"},
		TaskStep{ID: "verify", Name: "Verify result", Purpose: "validate output before completion", Allowed: true, Status: "planned"},
		TaskStep{ID: "memory", Name: "Update memory", Purpose: "store useful lessons without bloating context", Allowed: true, Status: "planned"},
	)
	return steps
}

func buildRetryPolicy(intake IntakeAnalysis) RetryPolicy {
	maxAttempts := 2
	if intake.Difficulty >= 4 {
		maxAttempts = 3
	}
	return RetryPolicy{
		MaxAttempts: maxAttempts,
		EscalationPath: []string{
			"retry with stronger context",
			"retry with stronger free/local model",
			"queue human review",
		},
		EscalateWhen: []string{
			"validation fails",
			"required context is missing",
			"model or tool capability is insufficient",
			"approval is required",
		},
		CurrentAttempt: 0,
		RetryAvailable: true,
	}
}

func initialValidationResult(plan ValidationPlan) ValidationResult {
	return ValidationResult{
		Passed:        false,
		Status:        "not_run",
		Checked:       plan.Steps,
		Failures:      []string{},
		NextAction:    "execute allowed steps, then validate",
		AttemptNumber: 0,
	}
}

func validatePlan(plan *CompletionPlan, attempt int) ValidationResult {
	failures := []string{}
	checked := append([]string{}, plan.ValidationPlan.Steps...)
	if len(plan.Intake.SuccessCriteria) == 0 {
		failures = append(failures, "success criteria are missing")
	}
	if plan.ModelDecision.SelectedModelID == "" {
		failures = append(failures, "no capable model was selected")
	}
	if len(plan.ToolDecision.SelectedTools) == 0 {
		failures = append(failures, "no tools were selected")
	}
	if plan.MinimalityDecision.Applicable {
		if !plan.MinimalityDecision.Necessary {
			failures = append(failures, "implementation was rejected by the necessity gate")
		}
		if strings.TrimSpace(plan.MinimalityDecision.SelectedLevel) == "" {
			failures = append(failures, "minimality strategy was not selected")
		}
	}
	if plan.RiskAssessment.ApprovalRequired && !plan.RiskAssessment.ApprovalGranted {
		failures = append(failures, "approval is required before execution")
	}
	if attempt > 0 {
		if plan.ExecutionResult == nil {
			failures = append(failures, "no execution result was produced")
		} else {
			if plan.Intake.NeedsTools || plan.Intake.NeedsLocalExecution {
				if plan.ExecutionResult.ToolExecution == nil {
					failures = append(failures, "required controlled runtime execution did not run")
				} else if plan.ExecutionResult.ToolExecution.Status != "completed" {
					failures = append(failures, "controlled runtime execution did not complete: "+plan.ExecutionResult.ToolExecution.Status)
				}
			}
			if strings.TrimSpace(plan.ExecutionResult.Output) == "" {
				failures = append(failures, "execution produced no output")
			}
			if !verificationStatusAcceptsCompletion(plan.ExecutionResult.VerificationStatus) {
				failures = append(failures, "execution output is not verified: "+plan.ExecutionResult.VerificationStatus)
			}
			if plan.ExecutionResult.UnsupportedClaims > 0 {
				failures = append(failures, "execution has unsupported or review-needed claims")
			}
			for _, claim := range plan.ExecutionResult.Claims {
				if claim.NeedsReview || !verificationStatusAcceptsCompletion(claim.Status) {
					failures = append(failures, "claim requires review: "+compact(claim.ClaimText))
					break
				}
			}
		}
	}

	passed := len(failures) == 0
	status := "passed"
	next := "mark task complete"
	if !passed {
		status = "failed"
		next = "retry, escalate, or request review"
	}
	return ValidationResult{
		Passed:        passed,
		Status:        status,
		Checked:       checked,
		Failures:      failures,
		NextAction:    next,
		AttemptNumber: attempt,
	}
}

func verificationStatusAcceptsCompletion(status string) bool {
	switch status {
	case verification.StatusVerified, verification.StatusSourceSupported, verification.StatusSchemaValidated, verification.StatusTestPassed, verification.StatusHumanApproved:
		return true
	default:
		return false
	}
}

func verificationStatusAcceptsMemory(status string) bool {
	switch status {
	case verification.StatusVerified, verification.StatusSourceSupported, verification.StatusTestPassed, verification.StatusHumanApproved:
		return true
	default:
		return false
	}
}

func proposeMemoryUpdates(request IntakeRequest, intake IntakeAnalysis) []MemoryUpdateProposal {
	proposals := []MemoryUpdateProposal{}
	if request.ProjectKey != "" {
		proposals = append(proposals, MemoryUpdateProposal{
			Kind:       "project",
			Content:    "Task planned for project " + request.ProjectKey + ": " + compact(request.Request),
			Tags:       []string{"task-plan", intake.TaskType},
			Reason:     "completed task plans can improve future project context",
			Confidence: 0.55,
		})
	}
	return proposals
}

func proposeLessons(request IntakeRequest, intake IntakeAnalysis, tools ToolRouteDecision) []MemoryUpdateProposal {
	lesson := MemoryUpdateProposal{
		Kind:       "procedural",
		Content:    "For " + intake.TaskType + " tasks, define success criteria, retrieve relevant context, route a capable model, use tools " + strings.Join(tools.SelectedTools, ", ") + ", validate before completion, and queue review when blocked.",
		Tags:       []string{"task-success-engine", intake.TaskType},
		Reason:     "successful task handling should improve future workflow selection",
		Confidence: 0.62,
	}
	if request.ProjectKey != "" {
		lesson.Tags = append(lesson.Tags, strings.ToLower(request.ProjectKey))
	}
	return []MemoryUpdateProposal{lesson}
}

func inferRealGoal(request IntakeRequest, intake IntakeAnalysis) string {
	clean := compact(request.Request)
	switch intake.TaskType {
	case "coding":
		return "Deliver a working code change and verify it against the requested behavior: " + clean
	case "architecture":
		return "Define an implementable architecture path with visible validation and safety gates: " + clean
	default:
		return "Complete and verify the requested outcome: " + clean
	}
}

func inferSuccessCriteria(taskType string, needsTools bool) []string {
	criteria := []string{
		"the user request is answered or implemented",
		"the result is validated before being marked complete",
		"the selected context and model choice are explained",
	}
	if taskType == "coding" || needsTools {
		criteria = append(criteria, "relevant checks or tests are run when available")
	}
	return criteria
}

func newReviewItem(taskID, reason, risk string, request IntakeRequest) ReviewQueueItem {
	priority := "normal"
	if risk == "high" {
		priority = "high"
	}
	return ReviewQueueItem{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Request:   request,
		Reason:    reason,
		Priority:  priority,
		Status:    "open",
		CreatedAt: time.Now().UTC(),
	}
}

func event(stage, message string) TaskEvent {
	if strings.TrimSpace(message) == "" {
		message = "completed"
	}
	return TaskEvent{
		At:      time.Now().UTC(),
		Stage:   stage,
		Message: message,
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func requiresControlledExecution(value string) bool {
	if containsWordOrPhrase(value, "run", "execute", "deploy", "install", "launch", "invoke") {
		return true
	}
	action := containsWordOrPhrase(value,
		"add", "apply", "build", "call", "change", "commit", "create", "delete", "fix",
		"implement", "merge", "modify", "move", "post", "publish", "push", "rename",
		"send", "start", "update", "write",
	)
	target := containsWordOrPhrase(value,
		"account", "api", "build", "code", "command", "deployment", "docker", "email",
		"file", "files", "message", "post", "posting", "repo", "repository", "request",
		"script", "test", "tests",
	)
	return action && target
}

func containsWordOrPhrase(value string, terms ...string) bool {
	normalized := " " + strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}), " ") + " "
	for _, term := range terms {
		normalizedTerm := strings.Join(strings.Fields(strings.ToLower(term)), " ")
		if normalizedTerm != "" && strings.Contains(normalized, " "+normalizedTerm+" ") {
			return true
		}
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

var reasoningRank = map[string]int{"low": 1, "medium": 2, "high": 3, "very_high": 4}

func maxReasoning(left, right string) string {
	if reasoningRank[left] >= reasoningRank[right] {
		return left
	}
	return right
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func compact(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 180 {
		return value
	}
	return value[:177] + "..."
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
