package task

import (
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/source"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type IntakeRequest struct {
	Request         string   `json:"request"`
	ProjectKey      string   `json:"projectKey,omitempty"`
	SuccessCriteria []string `json:"successCriteria,omitempty"`
	ExecuteAllowed  bool     `json:"executeAllowed,omitempty"`
}

type IntakeAnalysis struct {
	TaskType          string   `json:"taskType"`
	RiskLevel         string   `json:"riskLevel"`
	Difficulty        int      `json:"difficulty"`
	RequiredReasoning string   `json:"requiredReasoning"`
	SuccessCriteria   []string `json:"successCriteria"`
	NeedsMemory         bool     `json:"needsMemory"`
	NeedsTools          bool     `json:"needsTools"`
	NeedsDocuments      bool     `json:"needsDocuments"`
	NeedsWebAccess      bool     `json:"needsWebAccess"`
	NeedsLocalExecution bool     `json:"needsLocalExecution"`
	NeedsApproval       bool     `json:"needsApproval"`
	Reason              string   `json:"reason"`
}

type ContextPlan struct {
	Strategy      []string                  `json:"strategy"`
	UsedContext   []memory.RankedMemory     `json:"usedContext"`
	SourceContext []source.RankedExtraction `json:"sourceContext"`
	Explanation   string                    `json:"explanation"`
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
	MaxAttempts       int      `json:"maxAttempts"`
	EscalationPath    []string `json:"escalationPath"`
	EscalateWhen      []string `json:"escalateWhen"`
	CurrentAttempt    int      `json:"currentAttempt"`
	RetryAvailable    bool     `json:"retryAvailable"`
}

type ReviewQueueItem struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"taskId"`
	Reason    string    `json:"reason"`
	Priority  string    `json:"priority"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type TaskEvent struct {
	At      time.Time `json:"at"`
	Stage   string    `json:"stage"`
	Message string    `json:"message"`
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
	CreatedAt             time.Time              `json:"createdAt"`
	Request               string                 `json:"request"`
	ProjectKey            string                 `json:"projectKey,omitempty"`
	RealGoal              string                 `json:"realGoal"`
	Intake                IntakeAnalysis         `json:"intake"`
	ContextPlan           ContextPlan            `json:"contextPlan"`
	ModelDecision         llm.RouteDecision      `json:"modelDecision"`
	ToolDecision          ToolRouteDecision      `json:"toolDecision"`
	Steps                 []TaskStep             `json:"steps"`
	RiskAssessment        RiskAssessment         `json:"riskAssessment"`
	ValidationPlan        ValidationPlan         `json:"validationPlan"`
	ValidationResult      ValidationResult       `json:"validationResult"`
	ExecutionPlan         ExecutionPlan          `json:"executionPlan"`
	RetryPolicy           RetryPolicy            `json:"retryPolicy"`
	ReviewQueueItem       *ReviewQueueItem       `json:"reviewQueueItem,omitempty"`
	MemoryUpdateProposals []MemoryUpdateProposal `json:"memoryUpdateProposals"`
	LessonsLearned         []MemoryUpdateProposal `json:"lessonsLearned"`
	StoredMemoryIDs        []string               `json:"storedMemoryIds"`
	Events                 []TaskEvent            `json:"events"`
	CompletionStatus       string                 `json:"completionStatus"`
}

type Service interface {
	Plan(request IntakeRequest) (*CompletionPlan, error)
	Run(request IntakeRequest) (*CompletionPlan, error)
	Logs() []CompletionPlan
	ReviewQueue() []ReviewQueueItem
}

type service struct {
	memoryService memory.Service
	sourceService source.Service
	llmService    *llm.Service
	mu            sync.Mutex
	logs          []CompletionPlan
	reviewQueue   []ReviewQueueItem
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

func DefaultService() (Service, error) {
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		return nil, err
	}
	return NewService(memory.DefaultService(), llmService, source.DefaultService()), nil
}

func (s *service) Plan(request IntakeRequest) (*CompletionPlan, error) {
	plan, err := s.buildPlan(request, false)
	if err != nil {
		return nil, err
	}
	s.addLog(*plan)
	return plan, nil
}

func (s *service) Run(request IntakeRequest) (*CompletionPlan, error) {
	plan, err := s.buildPlan(request, true)
	if err != nil {
		return nil, err
	}
	plan.ValidationResult = validatePlan(plan, 1)
	plan.RetryPolicy.CurrentAttempt = 1
	plan.RetryPolicy.RetryAvailable = !plan.ValidationResult.Passed && plan.RetryPolicy.CurrentAttempt < plan.RetryPolicy.MaxAttempts

	if !plan.RiskAssessment.AllowedNow {
		plan.CompletionStatus = "review_required"
		plan.ValidationResult.Passed = false
		plan.ValidationResult.Status = "blocked"
		plan.ValidationResult.NextAction = "human review required before execution"
		item := newReviewItem(plan.ID, "approval required before task execution", plan.RiskAssessment.Level)
		plan.ReviewQueueItem = &item
		s.addReviewItem(item)
	} else if plan.ValidationResult.Passed {
		plan.CompletionStatus = "validated"
		plan.ValidationResult.Status = "passed"
		plan.ValidationResult.NextAction = "mark task complete"
		plan.Events = append(plan.Events, event("validation", "result validated against success criteria"))
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
			item := newReviewItem(plan.ID, "validation failed after retry", "medium")
			plan.ReviewQueueItem = &item
			s.addReviewItem(item)
		}
	} else {
		plan.CompletionStatus = "review_required"
		item := newReviewItem(plan.ID, "validation failed after available attempts", "medium")
		plan.ReviewQueueItem = &item
		s.addReviewItem(item)
	}

	s.addLog(*plan)
	return plan, nil
}

func (s *service) buildPlan(request IntakeRequest, runMode bool) (*CompletionPlan, error) {
	intake := analyzeIntake(request)
	contextResult, err := s.memoryService.Retrieve(memory.RetrieveRequest{
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
	risk := assessRisk(intake, request.ExecuteAllowed)
	steps := buildTaskSteps(intake, toolDecision, risk)
	validationPlan := buildValidationPlan(intake)
	memoryProposals := proposeMemoryUpdates(request, intake)
	plan := &CompletionPlan{
		ID:         uuid.New().String(),
		CreatedAt:  time.Now().UTC(),
		Request:    request.Request,
		ProjectKey: request.ProjectKey,
		RealGoal:   inferRealGoal(request, intake),
		Intake:     intake,
		ContextPlan: ContextPlan{
			Strategy: []string{
				"filter by project key when provided",
				"rank by keyword relevance, recency, confidence, and project match",
				"load only top relevant memories",
				"check connected-source extractions before task planning",
				"preserve source references on returned memories",
			},
			UsedContext:   contextResult.UsedContext,
			SourceContext: sourceContext,
			Explanation:   contextResult.Explanation + " " + sourceExplanation,
		},
		ModelDecision:         modelDecision,
		ToolDecision:          toolDecision,
		Steps:                 steps,
		RiskAssessment:        risk,
		ValidationPlan:        validationPlan,
		ValidationResult:      initialValidationResult(validationPlan),
		ExecutionPlan:         buildExecutionPlan(intake),
		RetryPolicy:           buildRetryPolicy(intake),
		MemoryUpdateProposals: memoryProposals,
		LessonsLearned:         proposeLessons(request, intake, toolDecision),
		Events: []TaskEvent{
			event("intake", "request classified and real goal inferred"),
			event("context", contextResult.Explanation),
			event("routing", modelDecision.Reason),
			event("tool-routing", toolDecision.Reason),
			event("risk", strings.Join(risk.Reasons, "; ")),
		},
		CompletionStatus:      "planned",
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
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]CompletionPlan, len(s.logs))
	copy(copied, s.logs)
	return copied
}

func (s *service) ReviewQueue() []ReviewQueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]ReviewQueueItem, len(s.reviewQueue))
	copy(copied, s.reviewQueue)
	return copied
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

func (s *service) storeLessons(plan *CompletionPlan) []string {
	stored := []string{}
	for _, lesson := range plan.LessonsLearned {
		created, err := s.memoryService.Create(memory.CreateRequest{
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

func (s *service) retrieveSourceContext(request IntakeRequest) ([]source.RankedExtraction, string) {
	if s.sourceService == nil {
		return []source.RankedExtraction{}, "Connected-source retrieval is not configured."
	}
	result, err := s.sourceService.Search(source.SearchRequest{
		Query:      request.Request,
		ProjectKey: request.ProjectKey,
		Limit:      6,
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

	needsTools := containsAny(text, "run", "execute", "test", "build", "deploy", "docker", "script", "api")
	needsDocs := containsAny(text, "document", "pdf", "spreadsheet", "slides", "docx")
	needsWeb := containsAny(text, "latest", "current", "today", "web", "browse", "search")
	needsLocal := containsAny(text, "local", "file", "repo", "docker", "windows", "build", "test")

	if containsAny(text, "code", "bug", "api", "compile", "build", "test") {
		taskType = "coding"
		difficulty = maxInt(difficulty, 3)
		reasoning = maxReasoning(reasoning, "medium")
		reasons = append(reasons, "coding/build terms detected")
	}
	if containsAny(text, "architecture", "blueprint", "multi-agent", "autonomous", "routing") {
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

func buildValidationPlan(intake IntakeAnalysis) ValidationPlan {
	steps := []string{
		"check every explicit success criterion",
		"verify required fields are present",
		"confirm context sources used are relevant",
	}
	if intake.TaskType == "coding" {
		steps = append(steps, "run applicable build and test commands")
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

func assessRisk(intake IntakeAnalysis, executeAllowed bool) RiskAssessment {
	reasons := []string{"read-only planning is allowed"}
	allowed := true
	if intake.NeedsApproval {
		reasons = append(reasons, "request contains high-risk action terms")
		allowed = false
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
	return RiskAssessment{
		Level:            intake.RiskLevel,
		ApprovalRequired: intake.NeedsApproval,
		Reasons:          reasons,
		AllowedNow:       allowed && (!intake.NeedsTools || executeAllowed || !intake.NeedsLocalExecution),
	}
}

func buildTaskSteps(intake IntakeAnalysis, tools ToolRouteDecision, risk RiskAssessment) []TaskStep {
	steps := []TaskStep{
		{ID: "understand", Name: "Understand request", Purpose: "identify the user's real goal", Allowed: true, Status: "planned"},
		{ID: "criteria", Name: "Define success criteria", Purpose: "make completion measurable", Allowed: true, Status: "planned"},
		{ID: "context", Name: "Gather context", Purpose: "retrieve only relevant memories and references", Allowed: true, Status: "planned"},
		{ID: "routing", Name: "Choose model and tools", Purpose: "select capable resources before optimizing cost", Allowed: true, Status: "planned"},
		{ID: "plan", Name: "Create plan", Purpose: "sequence safe actions and validation", Allowed: true, Status: "planned"},
		{ID: "risk", Name: "Check risk and approvals", Purpose: "block risky actions before execution", Allowed: true, Status: "planned"},
	}
	blockedHighRisk := len(tools.BlockedTools) > 0 && risk.ApprovalRequired
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
	if plan.ContextPlan.UsedContext == nil {
		failures = append(failures, "context retrieval did not run")
	}
	if len(plan.ToolDecision.SelectedTools) == 0 {
		failures = append(failures, "no tools were selected")
	}
	if plan.RiskAssessment.ApprovalRequired {
		failures = append(failures, "approval is required before execution")
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

func newReviewItem(taskID, reason, risk string) ReviewQueueItem {
	priority := "normal"
	if risk == "high" {
		priority = "high"
	}
	return ReviewQueueItem{
		ID:        uuid.New().String(),
		TaskID:    taskID,
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
