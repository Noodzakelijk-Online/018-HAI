package haios

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Metric struct {
	Label  string `json:"label"`
	Value  int64  `json:"value"`
	Status string `json:"status"`
}

type PlaneStatus struct {
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	Links       []string `json:"links"`
}

type HAIOSOverview struct {
	GeneratedAt       time.Time     `json:"generatedAt"`
	LocalFirst        bool          `json:"localFirst"`
	CompletionFirst   bool          `json:"completionFirst"`
	PaidBudgetEUR     float64       `json:"paidBudgetEur"`
	PaidUsageAllowed  bool          `json:"paidUsageAllowed"`
	Metrics           []Metric      `json:"metrics"`
	Planes            []PlaneStatus `json:"planes"`
	NeedsReviewTotal  int64         `json:"needsReviewTotal"`
	EmergencyStopNote string        `json:"emergencyStopNote"`
}

type Handler struct {
	db *gorm.DB
}

func DefaultHandler() (*Handler, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return &Handler{db: db}, nil
}

func (h *Handler) Overview(c *gin.Context) {
	policy, err := llm.NewServiceFromEnv()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	llmPolicy := policy.Policy()
	reviewTotal := h.count(&models.VerificationClaim{}, "needs_review = ? OR status IN ?", true, []string{"unsupported", "uncertain", "conflicting", "needs_review"})
	reviewTotal += h.count(&models.SourceExtraction{}, "uncertain = ? OR sensitive = ?", true, true)
	reviewTotal += h.count(&models.AutomationAlert{}, "status = ?", "open")
	reviewTotal += h.count(&models.WorkflowItem{}, "current_state IN ? OR approval_status = ?", []string{"needs_approval", "blocked"}, "pending")

	c.JSON(http.StatusOK, HAIOSOverview{
		GeneratedAt:      time.Now().UTC(),
		LocalFirst:       true,
		CompletionFirst:  true,
		PaidBudgetEUR:    llmPolicy.DailyPaidBudgetEUR,
		PaidUsageAllowed: llmPolicy.PaidCallsAllowed,
		Metrics: []Metric{
			{Label: "automations", Value: h.count(&models.Automation{}), Status: statusForCount(h.count(&models.Automation{}), "ready")},
			{Label: "unhealthy automations", Value: h.count(&models.Automation{}, "status IN ?", []string{"warning", "degraded", "broken"}), Status: statusForZero(h.count(&models.Automation{}, "status IN ?", []string{"warning", "degraded", "broken"}))},
			{Label: "connected sources", Value: h.count(&models.ConnectedSource{}, "status <> ?", "revoked"), Status: statusForCount(h.count(&models.ConnectedSource{}, "status <> ?", "revoked"), "ready")},
			{Label: "source extractions", Value: h.count(&models.SourceExtraction{}, "archived = ?", false), Status: statusForCount(h.count(&models.SourceExtraction{}, "archived = ?", false), "indexed")},
			{Label: "workflow items", Value: h.count(&models.WorkflowItem{}, "archived = ?", false), Status: statusForCount(h.count(&models.WorkflowItem{}, "archived = ?", false), "active")},
			{Label: "workflow approvals", Value: h.count(&models.WorkflowItem{}, "archived = ? AND current_state = ?", false, "needs_approval"), Status: statusForZero(h.count(&models.WorkflowItem{}, "archived = ? AND current_state = ?", false, "needs_approval"))},
			{Label: "context memories", Value: h.count(&models.ContextMemory{}, "archived = ?", false), Status: statusForCount(h.count(&models.ContextMemory{}, "archived = ?", false), "available")},
			{Label: "llm providers", Value: int64(len(llmPolicy.Providers)), Status: "configured"},
			{Label: "verification runs", Value: h.count(&models.VerificationRun{}), Status: statusForCount(h.count(&models.VerificationRun{}), "active")},
			{Label: "needs review", Value: reviewTotal, Status: statusForZero(reviewTotal)},
		},
		Planes: []PlaneStatus{
			{Name: "Control", Status: "operational", Description: "Dashboard, automation registry, health, workflow inbox, policy, and queues.", Links: []string{"/home", "/control-center", "/workflow-engine"}},
			{Name: "Knowledge", Status: "operational", Description: "Connected-source ingestion, extraction, index entries, search, and provenance.", Links: []string{"/connected-sources"}},
			{Name: "Memory", Status: "operational", Description: "Editable local memory with retrieval, dedupe, merge, archive, export, and correction.", Links: []string{"/memory"}},
			{Name: "Reasoning", Status: "operational", Description: "Task classifier, criteria generator, context planner, model router, workflow state machine, priority, retry, and review.", Links: []string{"/task-blueprint", "/llm-policy", "/workflow-engine"}},
			{Name: "Execution", Status: "guarded", Description: "Automation launch, workflow worker execution, retry limits, and controlled task runs exist; autonomous execution remains approval-gated.", Links: []string{"/control-center", "/task-blueprint", "/workflow-engine"}},
			{Name: "Verification", Status: "operational", Description: "Source-grounded answers, claim checks, deterministic calculation checks, conflicts, and review statuses.", Links: []string{"/grounded-answers"}},
			{Name: "Governance", Status: "guarded", Description: "Local-only source controls, paid budget policy, sensitive flags, workflow approvals, and audit logs.", Links: []string{"/llm-policy", "/connected-sources", "/grounded-answers", "/workflow-engine"}},
			{Name: "Observability", Status: "operational", Description: "Health summaries, sync logs, workflow events, verification runs, routing logs, diagnostics, and review indicators.", Links: []string{"/control-center", "/connected-sources", "/grounded-answers", "/workflow-engine"}},
		},
		NeedsReviewTotal:  reviewTotal,
		EmergencyStopNote: "Pause sources, revoke access, keep paid usage disabled, and block high-risk task execution until human approval is recorded.",
	})
}

func (h *Handler) count(model interface{}, query ...interface{}) int64 {
	var total int64
	db := h.db.Model(model)
	if len(query) > 0 {
		db = db.Where(query[0], query[1:]...)
	}
	_ = db.Count(&total).Error
	return total
}

func statusForCount(value int64, ready string) string {
	if value == 0 {
		return "empty"
	}
	return ready
}

func statusForZero(value int64) string {
	if value == 0 {
		return "clear"
	}
	return "needs_review"
}
