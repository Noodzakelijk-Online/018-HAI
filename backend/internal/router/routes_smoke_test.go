package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/config"

	"github.com/gin-gonic/gin"
)

// Mirrors the exact path set registered in initializeAutomationsRoutes to
// confirm gin builds the route tree without panicking (static + param at the
// same level) and resolves each new endpoint to the right handler.
func TestAutomationRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	hit := ""
	mark := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			hit = name
			c.Status(http.StatusOK)
		}
	}

	a := r.Group("/api/v1").Group("/automation")
	a.PATCH("/swap/:id1/:id2", mark("swap"))
	a.GET("/", mark("getAll"))
	a.GET("/health/summary", mark("summary"))
	a.GET("/health-summary", mark("summary"))
	a.GET("/images/:imageName", mark("image"))
	a.GET("/:id", mark("getByID"))
	a.POST("/:id/launch", mark("launch"))
	a.POST("/:id/stop-runtime", mark("stopRuntime"))
	a.POST("/:id/health-check", mark("healthCheck"))
	a.GET("/:id/diagnostics", mark("diagnostics"))
	a.POST("/", mark("create"))
	a.PATCH("/", mark("update"))
	a.DELETE("/:id", mark("delete"))

	agentRuntimes := r.Group("/api/v1").Group("/agent-runtimes")
	agentRuntimes.GET("/", mark("agentRuntimeRegistry"))
	agentRuntimes.GET("/health", mark("agentRuntimeHealth"))
	agentRuntimes.GET("/:id/skills", mark("agentRuntimeSkills"))
	agentRuntimes.POST("/:id/tasks/:taskId/stop", mark("agentRuntimeStopTask"))
	agentRuntimes.GET("/openclaw/ecosystem", mark("openclawEcosystem"))
	agentRuntimes.PATCH("/openclaw/ecosystem", mark("openclawEcosystemSet"))
	agentRuntimes.POST("/openclaw/ecosystem/refresh", mark("openclawEcosystemRefresh"))
	agentRuntimes.POST("/openclaw/ecosystem/upload", mark("openclawEcosystemUpload"))

	agentCycle := r.Group("/api/v1").Group("/agent-cycle")
	agentCycle.POST("/run", mark("agentCycleRun"))

	assistantRoutes := r.Group("/api/v1").Group("/assistant")
	assistantRoutes.POST("/command", mark("assistantCommand"))
	assistantRoutes.GET("/logs", mark("assistantLogs"))

	m := r.Group("/api/v1").Group("/memory")
	m.GET("/", mark("memoryList"))
	m.POST("/", mark("memoryCreate"))
	m.POST("/retrieve", mark("memoryRetrieve"))
	m.GET("/export", mark("memoryExport"))
	m.GET("/:id", mark("memoryGet"))

	memoryEngineRoutes := r.Group("/api/v1").Group("/memory-engine")
	memoryEngineRoutes.POST("/import", mark("memoryEngineImport"))
	memoryEngineRoutes.GET("/dashboard", mark("memoryEngineDashboard"))
	memoryEngineRoutes.POST("/search", mark("memoryEngineSearch"))
	memoryEngineRoutes.GET("/conversations", mark("memoryEngineConversations"))
	memoryEngineRoutes.GET("/conversations/:id", mark("memoryEngineConversation"))
	memoryEngineRoutes.DELETE("/conversations/:id", mark("memoryEngineConversationDelete"))
	memoryEngineRoutes.GET("/insights", mark("memoryEngineInsights"))

	llmRoutes := r.Group("/api/v1").Group("/llm")
	llmRoutes.GET("/policy", mark("llmPolicy"))
	llmRoutes.GET("/probes", mark("llmProbes"))
	llmRoutes.POST("/route", mark("llmRoute"))
	llmRoutes.POST("/generate", mark("llmGenerate"))
	llmRoutes.GET("/logs", mark("llmLogs"))

	tasks := r.Group("/api/v1").Group("/task")
	tasks.POST("/plan", mark("taskPlan"))
	tasks.POST("/run", mark("taskRun"))
	tasks.POST("/success", mark("taskRun"))
	tasks.GET("/logs", mark("taskLogs"))
	tasks.GET("/review-queue", mark("taskReviewQueue"))
	tasks.POST("/review-queue/:id/resolve", mark("taskReviewResolve"))

	sources := r.Group("/api/v1").Group("/sources")
	sources.GET("/connectors", mark("sourceConnectors"))
	sources.GET("/", mark("sourceList"))
	sources.POST("/", mark("sourceCreate"))
	sources.POST("/search", mark("sourceSearch"))
	sources.POST("/sync-due", mark("sourceSyncDue"))
	sources.GET("/extractions", mark("sourceExtractions"))
	sources.GET("/audit-logs", mark("sourceAuditLogs"))
	sources.PATCH("/extractions/:id", mark("sourceExtractionUpdate"))
	sources.POST("/extractions/:id/archive", mark("sourceExtractionArchive"))
	sources.DELETE("/extractions/:id", mark("sourceExtractionDelete"))
	sources.PATCH("/:id", mark("sourceUpdate"))
	sources.POST("/:id/sync", mark("sourceSync"))
	sources.POST("/:id/reindex", mark("sourceReindex"))
	sources.POST("/:id/pause", mark("sourcePause"))
	sources.POST("/:id/resume", mark("sourceResume"))
	sources.POST("/:id/revoke", mark("sourceRevoke"))

	verificationRoutes := r.Group("/api/v1").Group("/verification")
	verificationRoutes.POST("/answer", mark("verificationAnswer"))
	verificationRoutes.GET("/runs", mark("verificationRuns"))
	verificationRoutes.GET("/runs/:id", mark("verificationRunDetails"))

	osRoutes := r.Group("/api/v1").Group("/os")
	osRoutes.GET("/overview", mark("osOverview"))

	workflowRoutes := r.Group("/api/v1").Group("/workflow")
	workflowRoutes.GET("/overview", mark("workflowOverview"))
	workflowRoutes.GET("/approvals", mark("workflowApprovals"))
	workflowRoutes.GET("/dashboard", mark("workflowDashboard"))
	workflowRoutes.GET("/", mark("workflowItems"))
	workflowRoutes.POST("/intake", mark("workflowIntake"))
	workflowRoutes.POST("/recover-stale", mark("workflowRecoverStale"))
	workflowRoutes.POST("/run-due", mark("workflowRunDue"))
	workflowRoutes.POST("/open-loops/run-due", mark("workflowOpenLoopRunDue"))
	workflowRoutes.GET("/:id", mark("workflowGet"))
	workflowRoutes.POST("/:id/transition", mark("workflowTransition"))
	workflowRoutes.POST("/:id/approval", mark("workflowApprovalResolve"))
	workflowRoutes.POST("/:id/interruption/resolve", mark("workflowInterruptionResolve"))
	workflowRoutes.POST("/:id/proposals/:proposalId/resolve", mark("workflowProposalResolve"))
	workflowRoutes.PATCH("/:id/checklist/:itemId", mark("workflowChecklist"))

	pursuits := r.Group("/api/v1").Group("/pursuits")
	pursuits.GET("/", mark("pursuitList"))
	pursuits.POST("/", mark("pursuitCreate"))
	pursuits.GET("/dashboard", mark("pursuitDashboard"))
	pursuits.GET("/brief", mark("pursuitBrief"))
	pursuits.GET("/decisions", mark("pursuitDecisions"))
	pursuits.POST("/match", mark("pursuitMatch"))
	pursuits.POST("/intake", mark("pursuitRouteIntake"))
	pursuits.GET("/:id/evidence", mark("pursuitEvidence"))
	pursuits.GET("/:id", mark("pursuitGet"))
	pursuits.PATCH("/:id", mark("pursuitUpdate"))
	pursuits.POST("/:id/archive", mark("pursuitArchive"))
	pursuits.POST("/:id/summary", mark("pursuitSummary"))
	pursuits.POST("/:id/review", mark("pursuitReview"))
	pursuits.POST("/:id/decisions/resolve", mark("pursuitDecisionResolve"))
	pursuits.GET("/:id/activity", mark("pursuitActivity"))
	pursuits.GET("/:id/next-actions", mark("pursuitNextActions"))
	pursuits.GET("/:id/blockers", mark("pursuitBlockers"))
	pursuits.GET("/:id/approvals", mark("pursuitApprovals"))
	pursuits.POST("/:id/intake", mark("pursuitIntake"))
	pursuits.POST("/:id/plan", mark("pursuitPlan"))
	pursuits.POST("/:id/links", mark("pursuitLink"))
	pursuits.DELETE("/:id/links/:linkId", mark("pursuitDeleteLink"))

	cases := []struct {
		method, path, want string
	}{
		{"GET", "/api/v1/automation/health/summary", "summary"},
		{"GET", "/api/v1/automation/health-summary", "summary"},
		{"POST", "/api/v1/automation/abc/launch", "launch"},
		{"POST", "/api/v1/automation/abc/stop-runtime", "stopRuntime"},
		{"POST", "/api/v1/automation/abc/health-check", "healthCheck"},
		{"GET", "/api/v1/automation/abc/diagnostics", "diagnostics"},
		{"GET", "/api/v1/automation/abc", "getByID"},
		{"GET", "/api/v1/automation/images/logo.png", "image"},
		{"PATCH", "/api/v1/automation/swap/1/2", "swap"},
		{"GET", "/api/v1/agent-runtimes/", "agentRuntimeRegistry"},
		{"GET", "/api/v1/agent-runtimes/health", "agentRuntimeHealth"},
		{"GET", "/api/v1/agent-runtimes/openclaw/skills", "agentRuntimeSkills"},
		{"POST", "/api/v1/agent-runtimes/openclaw/tasks/task-1/stop", "agentRuntimeStopTask"},
		{"GET", "/api/v1/agent-runtimes/openclaw/ecosystem", "openclawEcosystem"},
		{"PATCH", "/api/v1/agent-runtimes/openclaw/ecosystem", "openclawEcosystemSet"},
		{"POST", "/api/v1/agent-runtimes/openclaw/ecosystem/refresh", "openclawEcosystemRefresh"},
		{"POST", "/api/v1/agent-runtimes/openclaw/ecosystem/upload", "openclawEcosystemUpload"},
		{"GET", "/api/v1/pursuits/decisions", "pursuitDecisions"},
		{"POST", "/api/v1/agent-cycle/run", "agentCycleRun"},
		{"POST", "/api/v1/assistant/command", "assistantCommand"},
		{"GET", "/api/v1/assistant/logs", "assistantLogs"},
		{"POST", "/api/v1/memory/retrieve", "memoryRetrieve"},
		{"GET", "/api/v1/memory/export", "memoryExport"},
		{"GET", "/api/v1/memory/abc", "memoryGet"},
		{"POST", "/api/v1/memory-engine/import", "memoryEngineImport"},
		{"GET", "/api/v1/memory-engine/dashboard", "memoryEngineDashboard"},
		{"POST", "/api/v1/memory-engine/search", "memoryEngineSearch"},
		{"GET", "/api/v1/memory-engine/conversations", "memoryEngineConversations"},
		{"GET", "/api/v1/memory-engine/conversations/abc", "memoryEngineConversation"},
		{"DELETE", "/api/v1/memory-engine/conversations/abc", "memoryEngineConversationDelete"},
		{"GET", "/api/v1/memory-engine/insights", "memoryEngineInsights"},
		{"GET", "/api/v1/llm/policy", "llmPolicy"},
		{"GET", "/api/v1/llm/probes", "llmProbes"},
		{"POST", "/api/v1/llm/route", "llmRoute"},
		{"POST", "/api/v1/llm/generate", "llmGenerate"},
		{"GET", "/api/v1/llm/logs", "llmLogs"},
		{"POST", "/api/v1/task/plan", "taskPlan"},
		{"POST", "/api/v1/task/run", "taskRun"},
		{"POST", "/api/v1/task/success", "taskRun"},
		{"GET", "/api/v1/task/logs", "taskLogs"},
		{"GET", "/api/v1/task/review-queue", "taskReviewQueue"},
		{"POST", "/api/v1/task/review-queue/abc/resolve", "taskReviewResolve"},
		{"GET", "/api/v1/sources/connectors", "sourceConnectors"},
		{"GET", "/api/v1/sources/", "sourceList"},
		{"POST", "/api/v1/sources/", "sourceCreate"},
		{"POST", "/api/v1/sources/search", "sourceSearch"},
		{"POST", "/api/v1/sources/sync-due", "sourceSyncDue"},
		{"GET", "/api/v1/sources/extractions", "sourceExtractions"},
		{"GET", "/api/v1/sources/audit-logs", "sourceAuditLogs"},
		{"PATCH", "/api/v1/sources/extractions/abc", "sourceExtractionUpdate"},
		{"POST", "/api/v1/sources/extractions/abc/archive", "sourceExtractionArchive"},
		{"DELETE", "/api/v1/sources/extractions/abc", "sourceExtractionDelete"},
		{"PATCH", "/api/v1/sources/abc", "sourceUpdate"},
		{"POST", "/api/v1/sources/abc/sync", "sourceSync"},
		{"POST", "/api/v1/sources/abc/reindex", "sourceReindex"},
		{"POST", "/api/v1/sources/abc/pause", "sourcePause"},
		{"POST", "/api/v1/sources/abc/resume", "sourceResume"},
		{"POST", "/api/v1/sources/abc/revoke", "sourceRevoke"},
		{"POST", "/api/v1/verification/answer", "verificationAnswer"},
		{"GET", "/api/v1/verification/runs", "verificationRuns"},
		{"GET", "/api/v1/verification/runs/abc", "verificationRunDetails"},
		{"GET", "/api/v1/os/overview", "osOverview"},
		{"GET", "/api/v1/workflow/overview", "workflowOverview"},
		{"GET", "/api/v1/workflow/approvals", "workflowApprovals"},
		{"GET", "/api/v1/workflow/dashboard", "workflowDashboard"},
		{"GET", "/api/v1/workflow/", "workflowItems"},
		{"POST", "/api/v1/workflow/intake", "workflowIntake"},
		{"POST", "/api/v1/workflow/recover-stale", "workflowRecoverStale"},
		{"POST", "/api/v1/workflow/run-due", "workflowRunDue"},
		{"POST", "/api/v1/workflow/open-loops/run-due", "workflowOpenLoopRunDue"},
		{"GET", "/api/v1/workflow/abc", "workflowGet"},
		{"POST", "/api/v1/workflow/abc/transition", "workflowTransition"},
		{"POST", "/api/v1/workflow/abc/approval", "workflowApprovalResolve"},
		{"POST", "/api/v1/workflow/abc/interruption/resolve", "workflowInterruptionResolve"},
		{"POST", "/api/v1/workflow/abc/proposals/def/resolve", "workflowProposalResolve"},
		{"PATCH", "/api/v1/workflow/abc/checklist/def", "workflowChecklist"},
		{"GET", "/api/v1/pursuits/", "pursuitList"},
		{"POST", "/api/v1/pursuits/", "pursuitCreate"},
		{"GET", "/api/v1/pursuits/dashboard", "pursuitDashboard"},
		{"GET", "/api/v1/pursuits/brief", "pursuitBrief"},
		{"POST", "/api/v1/pursuits/match", "pursuitMatch"},
		{"POST", "/api/v1/pursuits/intake", "pursuitRouteIntake"},
		{"GET", "/api/v1/pursuits/abc/evidence?uri=automation-launch://example", "pursuitEvidence"},
		{"GET", "/api/v1/pursuits/abc", "pursuitGet"},
		{"PATCH", "/api/v1/pursuits/abc", "pursuitUpdate"},
		{"POST", "/api/v1/pursuits/abc/archive", "pursuitArchive"},
		{"POST", "/api/v1/pursuits/abc/summary", "pursuitSummary"},
		{"POST", "/api/v1/pursuits/abc/review", "pursuitReview"},
		{"POST", "/api/v1/pursuits/abc/decisions/resolve", "pursuitDecisionResolve"},
		{"GET", "/api/v1/pursuits/abc/activity", "pursuitActivity"},
		{"GET", "/api/v1/pursuits/abc/next-actions", "pursuitNextActions"},
		{"GET", "/api/v1/pursuits/abc/blockers", "pursuitBlockers"},
		{"GET", "/api/v1/pursuits/abc/approvals", "pursuitApprovals"},
		{"POST", "/api/v1/pursuits/abc/intake", "pursuitIntake"},
		{"POST", "/api/v1/pursuits/abc/plan", "pursuitPlan"},
		{"POST", "/api/v1/pursuits/abc/links", "pursuitLink"},
		{"DELETE", "/api/v1/pursuits/abc/links/def", "pursuitDeleteLink"},
	}
	for _, tc := range cases {
		hit = ""
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s -> code %d, want 200", tc.method, tc.path, w.Code)
		}
		if hit != tc.want {
			t.Fatalf("%s %s -> handler %q, want %q", tc.method, tc.path, hit, tc.want)
		}
	}
}

func TestBackendAPIKeyMiddlewareDisabledWithoutKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.AppConfig.BackendAPIKey
	t.Cleanup(func() { config.AppConfig.BackendAPIKey = previous })
	config.AppConfig.BackendAPIKey = ""

	r := gin.New()
	r.Use(backendAPIKeyMiddleware())
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestLocalCaptureCORSAllowsExtensionPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(localCaptureCORSMiddleware())
	r.POST("/api/v1/memory-engine/import", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/memory-engine/import", nil)
	req.Header.Set("Origin", "chrome-extension://example-extension")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "chrome-extension://example-extension" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestLocalCaptureCORSRejectsUntrustedPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(localCaptureCORSMiddleware())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/memory-engine/import", nil)
	req.Header.Set("Origin", "https://attacker.example")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBackendAPIKeyMiddlewareBlocksMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.AppConfig.BackendAPIKey
	t.Cleanup(func() { config.AppConfig.BackendAPIKey = previous })
	config.AppConfig.BackendAPIKey = "secret"

	r := gin.New()
	r.Use(backendAPIKeyMiddleware())
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBackendAPIKeyMiddlewareAllowsMatchingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.AppConfig.BackendAPIKey
	t.Cleanup(func() { config.AppConfig.BackendAPIKey = previous })
	config.AppConfig.BackendAPIKey = "secret"

	r := gin.New()
	r.Use(backendAPIKeyMiddleware())
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(backendAPIKeyHeader, "secret")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
