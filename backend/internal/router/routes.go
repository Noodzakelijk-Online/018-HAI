package router

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"automation-hub-backend/docs"
	"automation-hub-backend/internal/agentcycle"
	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/ambient"
	"automation-hub-backend/internal/assistant"
	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/autonomy"
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/featureflags"
	"automation-hub-backend/internal/haios"
	"automation-hub-backend/internal/i18n"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/internal/source"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/verification"
	"automation-hub-backend/internal/workflow"
	"automation-hub-backend/internal/workflowtask"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func initializeRoutes(router *gin.Engine) error {
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "backend"})
	})
	router.GET("/readyz", readinessHandler(func() doctor.Report {
		return doctor.Diagnose(config.AppConfig)
	}))

	relativePathV1 := config.AppConfig.BaseUrl + "/v1"
	docs.SwaggerInfo.BasePath = relativePathV1
	v1 := router.Group(relativePathV1)
	v1.Use(backendAPIKeyMiddleware())
	v1.Use(identityMiddleware())
	{
		automationService := automation.DefaultService()
		runtimeRegistry := agentruntime.DefaultRegistry()
		runtimeHandler := agentruntime.NewHandler(runtimeRegistry)
		initializeAgentRuntimeRoutes(v1, runtimeHandler)
		autoHandler := automation.NewHandler(automationService)
		err := initializeAutomationsRoutes(v1, autoHandler)
		if err != nil {
			return err
		}
		probeHistory, err := llm.DefaultProbeHistoryRepository()
		if err != nil {
			return err
		}
		llmService, err := llm.NewServiceFromEnvWithProbeHistory(probeHistory)
		if err != nil {
			return err
		}
		llmHandler := llm.NewHandler(llmService)
		initializeLLMRoutes(v1, llmHandler)
		memoryService := memory.DefaultService()
		initializeMemoryRoutes(v1, memory.NewHandler(memoryService))
		workflowRunner := workflowtask.NewDeferredRunner()
		workflowService := workflow.NewServiceWithTaskRunner(workflow.DefaultRepository(), workflowRunner, memoryService)
		pursuitService := pursuit.NewService(pursuit.DefaultRepository(), workflowService)
		sourceService := source.NewServiceWithWorkflowAndPursuitLinker(source.DefaultRepository(), memoryService, workflowService, pursuitService)
		verificationService := verification.NewService(verification.DefaultRepository(), sourceService, memoryService, pursuitService)
		initializeVerificationRoutes(v1, verification.NewHandler(verificationService))
		taskService := task.NewServiceWithEnginesAndPursuitAttempts(
			memoryService,
			llmService,
			sourceService,
			verificationService,
			task.NewAutomationToolExecutor(automationService),
			pursuitService,
		)
		workflowRunner.Set(workflowtask.NewRunner(taskService))
		source.StartScheduler(context.Background(), sourceService)
		workflow.StartScheduler(context.Background(), workflowService)
		initializeSourceRoutes(v1, source.NewHandler(sourceService))
		initializeWorkflowRoutes(v1, workflow.NewHandlerWithPursuitIntakeRouter(workflowService, pursuitService))
		initializePursuitRoutes(v1, pursuit.NewHandler(pursuitService))
		memoryEngineSecret := config.AppConfig.MemoryEngineKey
		if strings.TrimSpace(memoryEngineSecret) == "" {
			memoryEngineSecret = config.AppConfig.BackendAPIKey
		}
		memoryEngineService := memoryengine.NewServiceWithPursuitLinker(
			memoryengine.DefaultRepository(),
			memoryService,
			workflowService,
			memoryEngineSecret,
			pursuitService,
		)
		initializeMemoryEngineRoutes(v1, memoryengine.NewHandler(memoryEngineService))
		ambientService := ambient.NewServiceWithPursuits(ambient.DefaultRepository(), workflowService, memoryEngineService, pursuitService, memoryService)
		ambient.StartScheduler(context.Background(), ambientService)
		initializeAmbientRoutes(v1, ambient.NewHandler(ambientService))
		agentCycleService := agentcycle.NewServiceWithPursuits(sourceService, workflowService, ambientService, pursuitService, memoryService)
		initializeAgentCycleRoutes(v1, agentcycle.NewHandler(agentCycleService))
		initializeAssistantRoutes(v1, assistant.NewHandler(assistant.NewService(taskService, agentCycleService, pursuitService)))
		initializeAutonomyRoutes(v1, autonomy.NewHandler(autonomy.DefaultService()))
		osHandler, err := haios.DefaultHandlerWithPursuits(pursuitService)
		if err != nil {
			return err
		}
		initializeHAIOSRoutes(v1, osHandler)
		initializeTaskRoutes(v1, task.NewHandler(taskService))
		flagStore := defaultFeatureFlags()
		initializeFeatureFlagRoutes(v1, flagStore)
		diagnose := func() doctor.Report { return doctor.Diagnose(config.AppConfig) }
		initializeSystemRoutes(v1, diagnose, func() map[string]int {
			return map[string]int{
				"featureFlags": len(flagStore.List()),
				"languages":    len(i18n.Supported()),
			}
		})
	}
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	return nil
}

func initializeAssistantRoutes(apiVersion *gin.RouterGroup, handler *assistant.Handler) {
	routes := apiVersion.Group("/assistant")
	routes.Use(assistant.RequireAuthenticatedOwner())
	{
		// A chat command may request execution, so it needs the same approval
		// capability as the direct task-run endpoint.
		routes.POST("/command", requirePermission(rbac.PermApprove), handler.Command)
		routes.GET("/logs", requirePermission(rbac.PermRead), handler.Logs)
	}
}

func initializeAgentCycleRoutes(apiVersion *gin.RouterGroup, handler *agentcycle.Handler) {
	routes := apiVersion.Group("/agent-cycle")
	routes.Use(requireAuthenticatedOwner())
	{
		// HTTP calls run the owner-scoped read/brief path only; system work stays
		// with the background worker.
		routes.POST("/run", requirePermission(rbac.PermRead), handler.Run)
	}
}

func initializeAutonomyRoutes(apiVersion *gin.RouterGroup, handler *autonomy.Handler) {
	routes := apiVersion.Group("/autonomy")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/overview", requirePermission(rbac.PermRead), handler.Overview)
		routes.POST("/stress", requirePermission(rbac.PermWrite), handler.Stress)
	}
}

func initializeAmbientRoutes(apiVersion *gin.RouterGroup, handler *ambient.Handler) {
	routes := apiVersion.Group("/ambient")
	routes.Use(ambient.RequireAuthenticatedOwner())
	{
		routes.GET("/overview", requirePermission(rbac.PermRead), handler.Overview)
		routes.POST("/scan", requirePermission(rbac.PermWrite), handler.Scan)
		routes.PATCH("/needs/:key", requirePermission(rbac.PermWrite), handler.UpdateNeed)
		routes.POST("/opportunities/:id/accept", requirePermission(rbac.PermWrite), handler.Accept)
		routes.POST("/opportunities/:id/dismiss", requirePermission(rbac.PermWrite), handler.Dismiss)
	}
}

func initializeAgentRuntimeRoutes(apiVersion *gin.RouterGroup, handler *agentruntime.Handler) {
	routes := apiVersion.Group("/agent-runtimes")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/", requirePermission(rbac.PermRead), handler.Registry)
		routes.GET("/health", requirePermission(rbac.PermRead), handler.Health)
		routes.GET("/:id/skills", requirePermission(rbac.PermRead), handler.Skills)
		routes.POST("/:id/tasks/:taskId/stop", requirePermission(rbac.PermApprove), handler.StopTask)
		routes.GET("/openclaw/ecosystem", requirePermission(rbac.PermRead), handler.OpenClawEcosystem)
		routes.PATCH("/openclaw/ecosystem", requirePermission(rbac.PermAdmin), handler.SetOpenClawEcosystem)
		routes.POST("/openclaw/ecosystem/refresh", requirePermission(rbac.PermAdmin), handler.RefreshOpenClawEcosystem)
		routes.POST("/openclaw/ecosystem/upload", requirePermission(rbac.PermAdmin), handler.UploadOpenClawEcosystem)
	}
}

func localCaptureCORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		allowed := strings.HasPrefix(origin, "chrome-extension://") ||
			strings.HasPrefix(origin, "moz-extension://") ||
			strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:")
		if origin != "" && allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-HAI-Backend-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if !allowed {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

const backendAPIKeyHeader = "X-HAI-Backend-Key"

func backendAPIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(config.AppConfig.BackendAPIKey)
		if expected == "" || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		provided := strings.TrimSpace(c.GetHeader(backendAPIKeyHeader))
		providedHash := sha256.Sum256([]byte(provided))
		expectedHash := sha256.Sum256([]byte(expected))
		if subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "backend API key required"})
			return
		}

		c.Next()
	}
}

func initializeAutomationsRoutes(apiVersion *gin.RouterGroup, autoHandler *automation.Handler) error {
	automations := apiVersion.Group("/automation")
	automations.Use(automation.RequireAuthenticatedOperator())
	{
		automations.PATCH("/swap/:id1/:id2", requirePermission(rbac.PermAdmin), autoHandler.SwapPosition)
		automations.GET("/", requirePermission(rbac.PermRead), autoHandler.GetAll)
		automations.GET("/health/summary", requirePermission(rbac.PermRead), autoHandler.HealthSummary)
		automations.GET("/health-summary", requirePermission(rbac.PermRead), autoHandler.HealthSummary)
		automations.GET("/images/:imageName", requirePermission(rbac.PermRead), autoHandler.ImageHandler)
		automations.GET("/:id", requirePermission(rbac.PermRead), autoHandler.GetByID)
		automations.POST("/:id/launch", requirePermission(rbac.PermApprove), autoHandler.Launch)
		automations.POST("/:id/stop-runtime", requirePermission(rbac.PermApprove), autoHandler.StopRuntimeTask)
		automations.POST("/:id/health-check", requirePermission(rbac.PermWrite), autoHandler.RunHealthCheck)
		automations.GET("/:id/diagnostics", requirePermission(rbac.PermRead), autoHandler.Diagnostics)
		automations.POST("/", requirePermission(rbac.PermAdmin), autoHandler.Create)
		automations.PATCH("/", requirePermission(rbac.PermAdmin), autoHandler.Update)
		automations.DELETE("/:id", requirePermission(rbac.PermAdmin), autoHandler.DeleteByID)
	}

	return nil
}

func initializeLLMRoutes(apiVersion *gin.RouterGroup, llmHandler *llm.Handler) {
	llmRoutes := apiVersion.Group("/llm")
	llmRoutes.Use(requireAuthenticatedOwner())
	{
		llmRoutes.GET("/policy", requirePermission(rbac.PermRead), llmHandler.Policy)
		llmRoutes.GET("/probes", requirePermission(rbac.PermRead), llmHandler.ProviderProbes)
		llmRoutes.GET("/probes/history", requirePermission(rbac.PermRead), llmHandler.ProviderProbeHistory)
		llmRoutes.POST("/route", requirePermission(rbac.PermWrite), llmHandler.Route)
		llmRoutes.POST("/generate", requirePermission(rbac.PermApprove), llmHandler.Generate)
		llmRoutes.GET("/logs", requirePermission(rbac.PermRead), llmHandler.Logs)
	}
}

func initializeMemoryRoutes(apiVersion *gin.RouterGroup, memoryHandler *memory.Handler) {
	memoryRoutes := apiVersion.Group("/memory")
	memoryRoutes.Use(requireAuthenticatedOwner())
	{
		memoryRoutes.GET("/", requirePermission(rbac.PermRead), memoryHandler.List)
		memoryRoutes.GET("/query", requirePermission(rbac.PermRead), memoryHandler.Query)
		memoryRoutes.POST("/", requirePermission(rbac.PermWrite), memoryHandler.Create)
		memoryRoutes.POST("/retrieve", requirePermission(rbac.PermRead), memoryHandler.Retrieve)
		memoryRoutes.GET("/export", requirePermission(rbac.PermRead), memoryHandler.Export)
		memoryRoutes.GET("/:id", requirePermission(rbac.PermRead), memoryHandler.Get)
		memoryRoutes.PATCH("/:id", requirePermission(rbac.PermWrite), memoryHandler.Update)
		memoryRoutes.POST("/:id/archive", requirePermission(rbac.PermWrite), memoryHandler.Archive)
		memoryRoutes.POST("/:id/restore", requirePermission(rbac.PermWrite), memoryHandler.Restore)
		memoryRoutes.DELETE("/:id", requirePermission(rbac.PermWrite), memoryHandler.Delete)
	}
}

func initializeMemoryEngineRoutes(apiVersion *gin.RouterGroup, handler *memoryengine.Handler) {
	routes := apiVersion.Group("/memory-engine")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.POST("/import", requirePermission(rbac.PermWrite), handler.Import)
		routes.GET("/dashboard", requirePermission(rbac.PermRead), handler.Dashboard)
		routes.POST("/search", requirePermission(rbac.PermRead), handler.Search)
		routes.GET("/conversations", requirePermission(rbac.PermRead), handler.Conversations)
		routes.GET("/conversations/:id", requirePermission(rbac.PermRead), handler.Conversation)
		routes.DELETE("/conversations/:id", requirePermission(rbac.PermWrite), handler.DeleteConversation)
		routes.GET("/insights", requirePermission(rbac.PermRead), handler.Insights)
	}
}

func initializeSourceRoutes(apiVersion *gin.RouterGroup, sourceHandler *source.Handler) {
	sourceRoutes := apiVersion.Group("/sources")
	sourceRoutes.Use(requireAuthenticatedOwner())
	{
		sourceRoutes.GET("/connectors", requirePermission(rbac.PermRead), sourceHandler.Connectors)
		sourceRoutes.GET("/", requirePermission(rbac.PermRead), sourceHandler.Sources)
		sourceRoutes.POST("/", requirePermission(rbac.PermWrite), sourceHandler.CreateSource)
		sourceRoutes.POST("/search", requirePermission(rbac.PermRead), sourceHandler.Search)
		sourceRoutes.POST("/sync-due", requirePermission(rbac.PermAdmin), sourceHandler.RunDueScheduledSyncs)
		sourceRoutes.GET("/sync-jobs", requirePermission(rbac.PermRead), sourceHandler.SyncJobs)
		sourceRoutes.GET("/extractions", requirePermission(rbac.PermRead), sourceHandler.Extractions)
		sourceRoutes.GET("/audit-logs", requirePermission(rbac.PermRead), sourceHandler.AuditLogs)
		sourceRoutes.PATCH("/extractions/:id", requirePermission(rbac.PermWrite), sourceHandler.UpdateExtraction)
		sourceRoutes.POST("/extractions/:id/archive", requirePermission(rbac.PermWrite), sourceHandler.ArchiveExtraction)
		sourceRoutes.DELETE("/extractions/:id", requirePermission(rbac.PermWrite), sourceHandler.DeleteExtraction)
		sourceRoutes.PATCH("/:id", requirePermission(rbac.PermWrite), sourceHandler.UpdateSource)
		sourceRoutes.POST("/:id/sync", requirePermission(rbac.PermWrite), sourceHandler.Sync)
		sourceRoutes.POST("/:id/reindex", requirePermission(rbac.PermWrite), sourceHandler.Reindex)
		sourceRoutes.POST("/:id/pause", requirePermission(rbac.PermWrite), sourceHandler.Pause)
		sourceRoutes.POST("/:id/resume", requirePermission(rbac.PermWrite), sourceHandler.Resume)
		sourceRoutes.POST("/:id/revoke", requirePermission(rbac.PermWrite), sourceHandler.Revoke)
	}
}

func initializeTaskRoutes(apiVersion *gin.RouterGroup, taskHandler *task.Handler) {
	taskRoutes := apiVersion.Group("/task")
	taskRoutes.Use(requireAuthenticatedOwner())
	{
		taskRoutes.POST("/plan", requirePermission(rbac.PermWrite), taskHandler.Plan)
		taskRoutes.POST("/run", requirePermission(rbac.PermApprove), taskHandler.Run)
		taskRoutes.POST("/success", requirePermission(rbac.PermApprove), taskHandler.Run)
		taskRoutes.GET("/logs", requirePermission(rbac.PermRead), taskHandler.Logs)
		taskRoutes.GET("/review-queue", requirePermission(rbac.PermRead), taskHandler.ReviewQueue)
		taskRoutes.POST("/review-queue/:id/resolve", requirePermission(rbac.PermApprove), taskHandler.ResolveReviewItem)
	}
}

func initializeVerificationRoutes(apiVersion *gin.RouterGroup, verificationHandler *verification.Handler) {
	verificationRoutes := apiVersion.Group("/verification")
	verificationRoutes.Use(requireAuthenticatedOwner())
	{
		verificationRoutes.POST("/answer", requirePermission(rbac.PermWrite), verificationHandler.Answer)
		verificationRoutes.GET("/runs", requirePermission(rbac.PermRead), verificationHandler.Runs)
		verificationRoutes.GET("/runs/:id", requirePermission(rbac.PermRead), verificationHandler.RunDetails)
	}
}

func defaultFeatureFlags() *featureflags.Store {
	store := featureflags.New()
	store.Set(featureflags.Flag{Key: "memory_query_search", Enabled: true, RolloutPercent: 100, Description: "Search/filter/sort/pagination on the memory list"})
	store.Set(featureflags.Flag{Key: "readiness_probe", Enabled: true, RolloutPercent: 100, Description: "Expose /readyz readiness endpoint"})
	return store
}

func initializeFeatureFlagRoutes(apiVersion *gin.RouterGroup, store *featureflags.Store) {
	apiVersion.GET("/flags", func(c *gin.Context) {
		c.JSON(200, gin.H{"flags": store.List()})
	})
}

func initializeHAIOSRoutes(apiVersion *gin.RouterGroup, osHandler *haios.Handler) {
	osRoutes := apiVersion.Group("/os")
	osRoutes.Use(haios.RequireAuthenticatedOwner())
	{
		osRoutes.GET("/overview", requirePermission(rbac.PermRead), osHandler.Overview)
	}
}

func initializeWorkflowRoutes(apiVersion *gin.RouterGroup, workflowHandler *workflow.Handler) {
	workflowRoutes := apiVersion.Group("/workflow")
	workflowRoutes.Use(workflow.RequireAuthenticatedOwner())
	{
		workflowRoutes.GET("/overview", requirePermission(rbac.PermRead), workflowHandler.Overview)
		workflowRoutes.GET("/approvals", requirePermission(rbac.PermRead), workflowHandler.ApprovalItems)
		workflowRoutes.GET("/dashboard", requirePermission(rbac.PermRead), workflowHandler.Dashboard)
		workflowRoutes.GET("/", requirePermission(rbac.PermRead), workflowHandler.Items)
		workflowRoutes.POST("/intake", requirePermission(rbac.PermWrite), workflowHandler.Intake)
		workflowRoutes.POST("/recover-stale", requirePermission(rbac.PermAdmin), workflowHandler.RecoverStaleClaims)
		workflowRoutes.POST("/run-due", requirePermission(rbac.PermAdmin), workflowHandler.RunDue)
		workflowRoutes.POST("/open-loops/run-due", requirePermission(rbac.PermAdmin), workflowHandler.RunDueOpenLoops)
		workflowRoutes.GET("/:id", requirePermission(rbac.PermRead), workflowHandler.Get)
		workflowRoutes.POST("/:id/transition", requirePermission(rbac.PermWrite), workflowHandler.Transition)
		workflowRoutes.POST("/:id/approval", requirePermission(rbac.PermApprove), workflowHandler.ResolveApproval)
		workflowRoutes.POST("/:id/interruption/resolve", requirePermission(rbac.PermApprove), workflowHandler.ResolveInterruptedExecution)
		workflowRoutes.POST("/:id/proposals/:proposalId/resolve", requirePermission(rbac.PermApprove), workflowHandler.ResolveProposal)
		workflowRoutes.PATCH("/:id/checklist/:itemId", requirePermission(rbac.PermWrite), workflowHandler.UpdateChecklistItem)
	}
}

func initializePursuitRoutes(apiVersion *gin.RouterGroup, pursuitHandler *pursuit.Handler) {
	pursuitRoutes := apiVersion.Group("/pursuits")
	pursuitRoutes.Use(pursuit.RequireAuthenticatedOwner())
	{
		pursuitRoutes.GET("/", requirePermission(rbac.PermRead), pursuitHandler.List)
		pursuitRoutes.POST("/", requirePermission(rbac.PermWrite), pursuitHandler.Create)
		pursuitRoutes.GET("/dashboard", requirePermission(rbac.PermRead), pursuitHandler.Dashboard)
		pursuitRoutes.GET("/brief", requirePermission(rbac.PermRead), pursuitHandler.Brief)
		pursuitRoutes.GET("/decisions", requirePermission(rbac.PermRead), pursuitHandler.Decisions)
		pursuitRoutes.POST("/match", requirePermission(rbac.PermRead), pursuitHandler.Match)
		pursuitRoutes.POST("/intake", requirePermission(rbac.PermWrite), pursuitHandler.RouteIntake)
		pursuitRoutes.GET("/:id/evidence", requirePermission(rbac.PermRead), pursuitHandler.ResolveEvidence)
		pursuitRoutes.GET("/:id", requirePermission(rbac.PermRead), pursuitHandler.Get)
		pursuitRoutes.PATCH("/:id", requirePermission(rbac.PermWrite), pursuitHandler.Update)
		pursuitRoutes.POST("/:id/archive", requirePermission(rbac.PermWrite), pursuitHandler.Archive)
		pursuitRoutes.POST("/:id/reopen", requirePermission(rbac.PermWrite), pursuitHandler.Reopen)
		pursuitRoutes.POST("/:id/summary", requirePermission(rbac.PermWrite), pursuitHandler.RefreshSummary)
		pursuitRoutes.POST("/:id/review", requirePermission(rbac.PermWrite), pursuitHandler.Review)
		pursuitRoutes.POST("/:id/decisions/resolve", requirePermission(rbac.PermApprove), pursuitHandler.ResolveDecision)
		pursuitRoutes.GET("/:id/activity", requirePermission(rbac.PermRead), pursuitHandler.Activity)
		pursuitRoutes.GET("/:id/next-actions", requirePermission(rbac.PermRead), pursuitHandler.NextActions)
		pursuitRoutes.GET("/:id/blockers", requirePermission(rbac.PermRead), pursuitHandler.Blockers)
		pursuitRoutes.GET("/:id/approvals", requirePermission(rbac.PermRead), pursuitHandler.Approvals)
		pursuitRoutes.GET("/:id/delegation", requirePermission(rbac.PermRead), pursuitHandler.DelegationPackage)
		pursuitRoutes.POST("/:id/intake", requirePermission(rbac.PermWrite), pursuitHandler.Intake)
		pursuitRoutes.POST("/:id/plan", requirePermission(rbac.PermWrite), pursuitHandler.Plan)
		pursuitRoutes.POST("/:id/links", requirePermission(rbac.PermWrite), pursuitHandler.Link)
		pursuitRoutes.DELETE("/:id/links/:linkId", requirePermission(rbac.PermWrite), pursuitHandler.DeleteLink)
	}
}
