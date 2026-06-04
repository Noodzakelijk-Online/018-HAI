package haios

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"net/http"
	"os"
	"strings"
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

type ReferenceStackStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Use    string `json:"use"`
}

type ReadinessGate struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Next     string `json:"next"`
}

type HAIOSOverview struct {
	GeneratedAt         time.Time              `json:"generatedAt"`
	CanonicalStack      string                 `json:"canonicalStack"`
	ReferenceStacks     []ReferenceStackStatus `json:"referenceStacks"`
	LocalFirst          bool                   `json:"localFirst"`
	CompletionFirst     bool                   `json:"completionFirst"`
	PaidBudgetEUR       float64                `json:"paidBudgetEur"`
	PaidUsageAllowed    bool                   `json:"paidUsageAllowed"`
	Metrics             []Metric               `json:"metrics"`
	Planes              []PlaneStatus          `json:"planes"`
	ReadinessGates      []ReadinessGate        `json:"readinessGates"`
	NeedsReviewTotal    int64                  `json:"needsReviewTotal"`
	EmergencyStop       bool                   `json:"emergencyStop"`
	EmergencyStopReason string                 `json:"emergencyStopReason"`
	EmergencyStopNote   string                 `json:"emergencyStopNote"`
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
	now := time.Now().UTC()
	reviewTotal := h.count(&models.VerificationClaim{}, "needs_review = ? OR status IN ?", true, []string{"unsupported", "uncertain", "conflicting", "needs_review"})
	reviewTotal += h.count(&models.SourceExtraction{}, "uncertain = ? OR sensitive = ?", true, true)
	reviewTotal += h.count(&models.AutomationAlert{}, "status = ?", "open")
	reviewTotal += h.count(&models.WorkflowItem{}, "current_state IN ? OR approval_status = ?", []string{"needs_approval", "blocked"}, "pending")
	reviewTotal += h.count(&models.WorkflowProposal{}, "status = ?", "open")
	reviewTotal += h.count(&models.WorkflowQualityGate{}, "status IN ?", []string{"needs_review", "failed"})
	reviewTotal += h.count(&models.WorkflowOpenLoop{}, "status = ? AND (follow_up_at IS NULL OR follow_up_at <= ?)", "open", now)
	emergencyActive := safety.EmergencyStopActive()
	emergencyReason := "emergency stop is clear"
	if emergencyActive {
		emergencyReason = safety.EmergencyStopReason()
	}

	c.JSON(http.StatusOK, HAIOSOverview{
		GeneratedAt:    time.Now().UTC(),
		CanonicalStack: "Codex Go backend + Angular dashboard + Postgres is the canonical product stack",
		ReferenceStacks: []ReferenceStackStatus{
			{Name: "Manus React/tRPC/MySQL", Status: "reference_only", Use: "Use as a product/UX reference and port only deliberately selected behavior into the canonical Go/Angular stack."},
		},
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
			{Label: "due open loops", Value: h.count(&models.WorkflowOpenLoop{}, "status = ? AND (follow_up_at IS NULL OR follow_up_at <= ?)", "open", now), Status: statusForZero(h.count(&models.WorkflowOpenLoop{}, "status = ? AND (follow_up_at IS NULL OR follow_up_at <= ?)", "open", now))},
			{Label: "quality gates needing review", Value: h.count(&models.WorkflowQualityGate{}, "status IN ?", []string{"needs_review", "failed"}), Status: statusForZero(h.count(&models.WorkflowQualityGate{}, "status IN ?", []string{"needs_review", "failed"}))},
			{Label: "context memories", Value: h.count(&models.ContextMemory{}, "archived = ?", false), Status: statusForCount(h.count(&models.ContextMemory{}, "archived = ?", false), "available")},
			{Label: "llm providers", Value: int64(len(llmPolicy.Providers)), Status: "configured"},
			{Label: "verification runs", Value: h.count(&models.VerificationRun{}), Status: statusForCount(h.count(&models.VerificationRun{}), "active")},
			{Label: "needs review", Value: reviewTotal, Status: statusForZero(reviewTotal)},
		},
		Planes: []PlaneStatus{
			{Name: "Control", Status: "operational", Description: "Dashboard, automation registry, health, workflow inbox, policy, and queues.", Links: []string{"/home", "/control-center", "/workflow-engine"}},
			{Name: "Knowledge", Status: "partial", Description: "Local-folder ingestion is operational; real OAuth email/calendar/Drive/Trello/GitHub adapters are not yet wired end-to-end.", Links: []string{"/connected-sources"}},
			{Name: "Memory", Status: "operational", Description: "Editable local memory with retrieval, dedupe, merge, archive, export, and correction.", Links: []string{"/memory"}},
			{Name: "Reasoning", Status: "operational", Description: "Task classifier, criteria generator, context planner, model router, workflow state machine, priority, retry, and review.", Links: []string{"/task-blueprint", "/llm-policy", "/workflow-engine"}},
			{Name: "Execution", Status: "guarded", Description: "Automation launch, workflow worker execution, retry limits, and controlled task runs exist; autonomous execution remains approval-gated.", Links: []string{"/control-center", "/task-blueprint", "/workflow-engine"}},
			{Name: "Verification", Status: "partial", Description: "Claim/status checks exist, but real-world correctness still depends on connected sources and live provider validation.", Links: []string{"/grounded-answers"}},
			{Name: "Governance", Status: "guarded", Description: "Local-only source controls, paid budget policy, sensitive flags, workflow approvals, and audit logs.", Links: []string{"/llm-policy", "/connected-sources", "/grounded-answers", "/workflow-engine"}},
			{Name: "Observability", Status: "operational", Description: "Health summaries, sync logs, workflow events, verification runs, routing logs, diagnostics, and review indicators.", Links: []string{"/control-center", "/connected-sources", "/grounded-answers", "/workflow-engine"}},
		},
		ReadinessGates:      readinessGates(llmPolicy),
		NeedsReviewTotal:    reviewTotal,
		EmergencyStop:       emergencyActive,
		EmergencyStopReason: emergencyReason,
		EmergencyStopNote:   "Set HAI_EMERGENCY_STOP=true to block model generation, automation launches, task execution, workflow workers, and follow-up workers while keeping planning and review visible.",
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

func readinessGates(policy llm.Policy) []ReadinessGate {
	return []ReadinessGate{
		{
			Name:     "Canonical stack selected",
			Status:   "decided",
			Evidence: "Go/Angular/Postgres is canonical; Manus React/tRPC/MySQL is reference-only.",
			Next:     "Port useful Manus behavior through explicit issues instead of running two competing products.",
		},
		{
			Name:     "Live LLM provider configured",
			Status:   statusForBool(liveProviderConfigured(policy), "configured", "not_configured"),
			Evidence: liveProviderEvidence(policy),
			Next:     "Connect Ollama, LM Studio, or a configured free OpenAI-compatible endpoint and run provider smoke tests.",
		},
		{
			Name:     "Real account connectors",
			Status:   "partial",
			Evidence: "Manual import and local-folder sync are implemented; OAuth email/calendar/Drive/Trello/GitHub adapters remain connector contracts.",
			Next:     "Implement one real OAuth connector at a time with incremental sync, provenance, and delete/revoke behavior.",
		},
		{
			Name:     "Controlled runtime execution safety",
			Status:   runtimeSafetyStatus(),
			Evidence: runtimeSafetyEvidence(),
			Next:     "Keep script/Docker execution disabled until targets are allowlisted and reviewed; keep link-local API targets blocked.",
		},
		{
			Name:     "Emergency stop",
			Status:   statusForBool(safety.EmergencyStopActive(), "active", "clear"),
			Evidence: emergencyStopEvidence(),
			Next:     "Use HAI_EMERGENCY_STOP=true during incidents; clear it only after reviewing blocked work.",
		},
		{
			Name:     "External correctness proof",
			Status:   "unproven",
			Evidence: "Unit tests cover internal consistency; live provider/source fixtures and end-to-end account workflows are still required.",
			Next:     "Add integration tests against a local Ollama server, seeded local source folders, and one real account connector sandbox.",
		},
	}
}

func liveProviderConfigured(policy llm.Policy) bool {
	for _, provider := range policy.Providers {
		if provider.Enabled && provider.Configured {
			return true
		}
	}
	return false
}

func liveProviderEvidence(policy llm.Policy) string {
	configured := []string{}
	for _, provider := range policy.Providers {
		if provider.Enabled && provider.Configured {
			configured = append(configured, provider.Name)
		}
	}
	if len(configured) == 0 {
		return "No enabled provider has a configured endpoint and required credentials."
	}
	return "Configured provider endpoints: " + strings.Join(configured, ", ")
}

func runtimeSafetyStatus() string {
	if strings.EqualFold(os.Getenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED"), "true") || strings.EqualFold(os.Getenv("AUTOMATION_DOCKER_CONTROL_ENABLED"), "true") {
		return "guarded_enabled"
	}
	return "guarded_disabled"
}

func runtimeSafetyEvidence() string {
	parts := []string{
		"API launches require AUTOMATION_API_ALLOWED_HOSTS and reject link-local/metadata targets by default.",
		"Script execution enabled: " + boolWord(strings.EqualFold(os.Getenv("AUTOMATION_SCRIPT_EXECUTION_ENABLED"), "true")),
		"Docker control enabled: " + boolWord(strings.EqualFold(os.Getenv("AUTOMATION_DOCKER_CONTROL_ENABLED"), "true")),
	}
	return strings.Join(parts, " ")
}

func emergencyStopEvidence() string {
	if safety.EmergencyStopActive() {
		return safety.EmergencyStopReason()
	}
	return "Emergency stop is clear; autonomous execution still remains approval-gated by risk policy."
}

func statusForBool(value bool, positive, negative string) string {
	if value {
		return positive
	}
	return negative
}

func boolWord(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
