package router

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"automation-hub-backend/docs"
	"automation-hub-backend/internal/accountfeed"
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
	"automation-hub-backend/internal/hardwareprofile"
	"automation-hub-backend/internal/i18n"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/opscontrol"
	"automation-hub-backend/internal/phase2"
	"automation-hub-backend/internal/privacyfilter"
	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/runtimelab"
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
		llmService, err := llm.NewServiceFromEnv()
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
		verificationService := verification.NewService(verification.DefaultRepository(), sourceService, memoryService)
		initializeVerificationRoutes(v1, verification.NewHandler(verificationService))
		taskService := task.NewServiceWithEngines(
			memoryService,
			llmService,
			sourceService,
			verificationService,
			task.NewAutomationToolExecutor(automationService),
		)
		workflowRunner.Set(workflowtask.NewRunner(taskService))
		source.StartScheduler(context.Background(), sourceService)
		workflow.StartScheduler(context.Background(), workflowService)
		initializeSourceRoutes(v1, source.NewHandler(sourceService))
		initializeWorkflowRoutes(v1, workflow.NewHandler(workflowService))
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
		initializeAssistantRoutes(v1, assistant.NewHandler(assistant.NewService(taskService, agentCycleService)))
		initializeAutonomyRoutes(v1, autonomy.NewHandler(autonomy.DefaultService()))
		osHandler, err := haios.DefaultHandlerWithPursuits(pursuitService)
		if err != nil {
			return err
		}
		initializeHAIOSRoutes(v1, osHandler)
		initializeTaskRoutes(v1, task.NewHandler(taskService))
		modelIntelService := modelintelligence.DefaultService()
		initializeModelIntelligenceRoutes(v1, modelintelligence.NewHandler(modelIntelService))
		initializeHardwareRoutes(v1, hardwareprofile.NewHandler(hardwareprofile.DefaultService()))
		privacyService := privacyfilter.DefaultService()
		initializePrivacyRoutes(v1, privacyfilter.NewHandler(privacyService))
		phase2Module := phase2.DefaultModuleWithModelIntel(modelIntelService)
		initializePhase2Routes(v1, phase2Module.Handler())
		runtimeLabService := runtimelab.NewService(phase2Module.Broker(), phase2Module.Service(), phase2Module.OwnerUserID(), phase2Module.WorkspaceID())
		initializeRuntimeLabRoutes(v1, runtimelab.NewHandler(runtimeLabService))
		feedRegistry := accountfeed.NewRegistry(phase2Module.Service(), privacyService, accountfeed.FetchOptions{
			FeedsRoot: phase2Module.FeedsDir(),
			AllowHTTP: strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_PHASE2_ALLOW_HTTP_FEEDS")), "true"),
		})
		seedAccountFeeds(feedRegistry, phase2Module)
		initializeAccountFeedRoutes(v1, accountfeed.NewHandler(feedRegistry, phase2Module.OwnerUserID(), phase2Module.WorkspaceID()))
		initializeOpsControlRoutes(v1, opscontrol.NewHandler(phase2Module.OpsControl()))
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
	{
		routes.POST("/command", handler.Command)
		routes.GET("/logs", handler.Logs)
	}
}

func initializeAgentCycleRoutes(apiVersion *gin.RouterGroup, handler *agentcycle.Handler) {
	routes := apiVersion.Group("/agent-cycle")
	{
		routes.POST("/run", handler.Run)
	}
}

func initializeAutonomyRoutes(apiVersion *gin.RouterGroup, handler *autonomy.Handler) {
	routes := apiVersion.Group("/autonomy")
	{
		routes.GET("/overview", handler.Overview)
		routes.POST("/stress", handler.Stress)
	}
}

func initializeAmbientRoutes(apiVersion *gin.RouterGroup, handler *ambient.Handler) {
	routes := apiVersion.Group("/ambient")
	{
		routes.GET("/overview", handler.Overview)
		routes.POST("/scan", handler.Scan)
		routes.PATCH("/needs/:key", handler.UpdateNeed)
		routes.POST("/opportunities/:id/accept", handler.Accept)
		routes.POST("/opportunities/:id/dismiss", handler.Dismiss)
	}
}

func initializeAgentRuntimeRoutes(apiVersion *gin.RouterGroup, handler *agentruntime.Handler) {
	routes := apiVersion.Group("/agent-runtimes")
	{
		routes.GET("/", handler.Registry)
		routes.GET("/health", handler.Health)
		routes.GET("/:id/skills", handler.Skills)
		routes.POST("/:id/tasks/:taskId/stop", handler.StopTask)
		routes.GET("/openclaw/ecosystem", handler.OpenClawEcosystem)
		routes.PATCH("/openclaw/ecosystem", handler.SetOpenClawEcosystem)
		routes.POST("/openclaw/ecosystem/refresh", handler.RefreshOpenClawEcosystem)
		routes.POST("/openclaw/ecosystem/upload", handler.UploadOpenClawEcosystem)
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
	{
		automations.GET("/swap/:id1/:id2", autoHandler.SwapPosition)
		automations.GET("/", autoHandler.GetAll)
		automations.GET("/health/summary", autoHandler.HealthSummary)
		automations.GET("/health-summary", autoHandler.HealthSummary)
		automations.GET("/images/:imageName", autoHandler.ImageHandler)
		automations.GET("/:id", autoHandler.GetByID)
		automations.POST("/:id/launch", autoHandler.Launch)
		automations.POST("/:id/stop-runtime", autoHandler.StopRuntimeTask)
		automations.POST("/:id/health-check", autoHandler.RunHealthCheck)
		automations.GET("/:id/diagnostics", autoHandler.Diagnostics)
		automations.POST("/", autoHandler.Create)
		automations.PATCH("/", autoHandler.Update)
		automations.DELETE("/:id", autoHandler.DeleteByID)
	}

	return nil
}

func initializeLLMRoutes(apiVersion *gin.RouterGroup, llmHandler *llm.Handler) {
	llmRoutes := apiVersion.Group("/llm")
	{
		llmRoutes.GET("/policy", llmHandler.Policy)
		llmRoutes.GET("/probes", llmHandler.ProviderProbes)
		llmRoutes.POST("/route", llmHandler.Route)
		llmRoutes.POST("/generate", llmHandler.Generate)
		llmRoutes.GET("/logs", llmHandler.Logs)
	}
}

func initializeMemoryRoutes(apiVersion *gin.RouterGroup, memoryHandler *memory.Handler) {
	memoryRoutes := apiVersion.Group("/memory")
	{
		memoryRoutes.GET("/", memoryHandler.List)
		memoryRoutes.GET("/query", memoryHandler.Query)
		memoryRoutes.POST("/", memoryHandler.Create)
		memoryRoutes.POST("/retrieve", memoryHandler.Retrieve)
		memoryRoutes.GET("/export", memoryHandler.Export)
		memoryRoutes.GET("/:id", memoryHandler.Get)
		memoryRoutes.PATCH("/:id", memoryHandler.Update)
		memoryRoutes.POST("/:id/archive", memoryHandler.Archive)
		memoryRoutes.POST("/:id/restore", memoryHandler.Restore)
		memoryRoutes.DELETE("/:id", memoryHandler.Delete)
	}
}

func initializeMemoryEngineRoutes(apiVersion *gin.RouterGroup, handler *memoryengine.Handler) {
	routes := apiVersion.Group("/memory-engine")
	{
		routes.POST("/import", handler.Import)
		routes.GET("/dashboard", handler.Dashboard)
		routes.POST("/search", handler.Search)
		routes.GET("/conversations", handler.Conversations)
		routes.GET("/conversations/:id", handler.Conversation)
		routes.DELETE("/conversations/:id", handler.DeleteConversation)
		routes.GET("/insights", handler.Insights)
	}
}

func initializeSourceRoutes(apiVersion *gin.RouterGroup, sourceHandler *source.Handler) {
	sourceRoutes := apiVersion.Group("/sources")
	{
		sourceRoutes.GET("/connectors", sourceHandler.Connectors)
		sourceRoutes.GET("/", sourceHandler.Sources)
		sourceRoutes.POST("/", sourceHandler.CreateSource)
		sourceRoutes.POST("/search", sourceHandler.Search)
		sourceRoutes.POST("/sync-due", sourceHandler.RunDueScheduledSyncs)
		sourceRoutes.GET("/sync-jobs", sourceHandler.SyncJobs)
		sourceRoutes.GET("/extractions", sourceHandler.Extractions)
		sourceRoutes.GET("/audit-logs", sourceHandler.AuditLogs)
		sourceRoutes.PATCH("/extractions/:id", sourceHandler.UpdateExtraction)
		sourceRoutes.POST("/extractions/:id/archive", sourceHandler.ArchiveExtraction)
		sourceRoutes.DELETE("/extractions/:id", sourceHandler.DeleteExtraction)
		sourceRoutes.PATCH("/:id", sourceHandler.UpdateSource)
		sourceRoutes.POST("/:id/sync", sourceHandler.Sync)
		sourceRoutes.POST("/:id/reindex", sourceHandler.Reindex)
		sourceRoutes.POST("/:id/pause", sourceHandler.Pause)
		sourceRoutes.POST("/:id/resume", sourceHandler.Resume)
		sourceRoutes.POST("/:id/revoke", sourceHandler.Revoke)
	}
}

func initializeTaskRoutes(apiVersion *gin.RouterGroup, taskHandler *task.Handler) {
	taskRoutes := apiVersion.Group("/task")
	{
		taskRoutes.POST("/plan", taskHandler.Plan)
		taskRoutes.POST("/run", taskHandler.Run)
		taskRoutes.POST("/success", taskHandler.Run)
		taskRoutes.GET("/logs", taskHandler.Logs)
		taskRoutes.GET("/review-queue", taskHandler.ReviewQueue)
		taskRoutes.POST("/review-queue/:id/resolve", taskHandler.ResolveReviewItem)
	}
}

func initializeVerificationRoutes(apiVersion *gin.RouterGroup, verificationHandler *verification.Handler) {
	verificationRoutes := apiVersion.Group("/verification")
	{
		verificationRoutes.POST("/answer", verificationHandler.Answer)
		verificationRoutes.GET("/runs", verificationHandler.Runs)
		verificationRoutes.GET("/runs/:id", verificationHandler.RunDetails)
	}
}

func initializePhase2Routes(apiVersion *gin.RouterGroup, handler *phase2.Handler) {
	ops := apiVersion.Group("/operations")
	{
		ops.GET("", handler.ListOperations)
		ops.GET("/dashboard", handler.Dashboard)
		ops.GET("/:id", handler.GetOperation)
		ops.GET("/:id/events", handler.OperationEvents)
		ops.GET("/:id/approvals", handler.Approvals)
		ops.POST("/:id/approve", handler.Approve)
		ops.POST("/:id/reject", handler.Reject)
		ops.POST("/:id/later", handler.Later)
		ops.POST("/:id/block-similar", handler.BlockSimilar)
		ops.POST("/:id/run", handler.RunOperation)
		ops.POST("/:id/evidence-pack", handler.GenerateEvidencePack)
	}
	apiVersion.GET("/evidence-packs/:id", handler.GetEvidencePack)
	apiVersion.POST("/background/run", handler.RunBackground)
}

func initializeAccountFeedRoutes(apiVersion *gin.RouterGroup, handler *accountfeed.Handler) {
	af := apiVersion.Group("/account-feeds")
	{
		af.GET("", handler.List)
		af.POST("", handler.Create)
		af.GET("/bridges", handler.Bridges)
		af.GET("/permissions", handler.Permissions)
		af.POST("/sync-due", handler.SyncDue)
		af.GET("/:id", handler.Get)
		af.PATCH("/:id", handler.Patch)
		af.POST("/:id/sync", handler.Sync)
		af.GET("/:id/audit", handler.Audit)
	}
}

// seedAccountFeeds registers the module's configured local feed files so they
// appear in the Account Feeds API and can be synced on demand.
func seedAccountFeeds(reg *accountfeed.Registry, m *phase2.Module) {
	for _, name := range m.FeedFiles() {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		_, _ = reg.Register(accountfeed.Feed{
			Name:         strings.TrimSuffix(name, ".json"),
			Provider:     string(accountfeed.ProviderGenericJSONFeed),
			AccountLabel: name,
			SourceType:   accountfeed.SourceLocalJSONFile,
			Path:         name,
			OwnerUserID:  m.OwnerUserID(),
			WorkspaceID:  m.WorkspaceID(),
			Enabled:      true,
		})
	}
}

func initializeModelIntelligenceRoutes(apiVersion *gin.RouterGroup, handler *modelintelligence.Handler) {
	mi := apiVersion.Group("/model-intelligence")
	{
		mi.GET("/overview", handler.Overview)
		mi.GET("/profiles", handler.Profiles)
		mi.GET("/profiles/:providerId/:modelId", handler.Profile)
		mi.POST("/profiles/:providerId/:modelId/benchmark", handler.Benchmark)
		mi.GET("/benchmarks", handler.Benchmarks)
		mi.GET("/telemetry", handler.Telemetry)
		mi.GET("/lane-winners", handler.LaneWinners)
		mi.GET("/cache", handler.Cache)
		mi.DELETE("/cache/:id", handler.DeleteCache)
		mi.GET("/token-budgets", handler.TokenBudgets)
		mi.PATCH("/token-budgets", handler.UpdateTokenBudgets)
	}
}

func initializeHardwareRoutes(apiVersion *gin.RouterGroup, handler *hardwareprofile.Handler) {
	hw := apiVersion.Group("/hardware")
	{
		hw.GET("/profile", handler.Profile)
		hw.POST("/detect", handler.Detect)
		hw.PATCH("/profile", handler.Patch)
	}
	power := apiVersion.Group("/power")
	{
		power.GET("/policy", handler.PowerPolicy)
		power.PATCH("/policy", handler.UpdatePowerPolicy)
	}
}

func initializePrivacyRoutes(apiVersion *gin.RouterGroup, handler *privacyfilter.Handler) {
	privacy := apiVersion.Group("/privacy")
	{
		privacy.POST("/scan", handler.ScanContent)
		privacy.GET("/scans", handler.Scans)
		privacy.GET("/scans/:id", handler.ScanByID)
	}
}

func initializeOpsControlRoutes(apiVersion *gin.RouterGroup, handler *opscontrol.Handler) {
	bg := apiVersion.Group("/background")
	{
		bg.GET("/status", handler.Status)
		bg.POST("/pause", handler.Pause)
		bg.POST("/resume", handler.Resume)
		bg.PATCH("/mode", handler.SetMode)
	}
	wr := apiVersion.Group("/windows-runtime")
	{
		wr.GET("/readiness", handler.Readiness)
		wr.POST("/recovery", handler.Recovery)
		wr.POST("/emergency-stop/verify", handler.VerifyEmergencyStop)
	}
}

func initializeRuntimeLabRoutes(apiVersion *gin.RouterGroup, handler *runtimelab.Handler) {
	rl := apiVersion.Group("/runtime-lab")
	{
		rl.GET("/overview", handler.Overview)
		rl.POST("/:runtimeId/probe", handler.Probe)
		rl.POST("/:runtimeId/self-test", handler.SelfTest)
		rl.GET("/:runtimeId/attempts", handler.Attempts)
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
	{
		osRoutes.GET("/overview", osHandler.Overview)
	}
}

func initializeWorkflowRoutes(apiVersion *gin.RouterGroup, workflowHandler *workflow.Handler) {
	workflowRoutes := apiVersion.Group("/workflow")
	{
		workflowRoutes.GET("/overview", workflowHandler.Overview)
		workflowRoutes.GET("/approvals", workflowHandler.ApprovalItems)
		workflowRoutes.GET("/dashboard", workflowHandler.Dashboard)
		workflowRoutes.GET("/", workflowHandler.Items)
		workflowRoutes.POST("/intake", workflowHandler.Intake)
		workflowRoutes.POST("/recover-stale", workflowHandler.RecoverStaleClaims)
		workflowRoutes.POST("/run-due", workflowHandler.RunDue)
		workflowRoutes.POST("/open-loops/run-due", workflowHandler.RunDueOpenLoops)
		workflowRoutes.GET("/:id", workflowHandler.Get)
		workflowRoutes.POST("/:id/transition", workflowHandler.Transition)
		workflowRoutes.POST("/:id/approval", workflowHandler.ResolveApproval)
		workflowRoutes.POST("/:id/interruption/resolve", workflowHandler.ResolveInterruptedExecution)
		workflowRoutes.POST("/:id/proposals/:proposalId/resolve", workflowHandler.ResolveProposal)
		workflowRoutes.PATCH("/:id/checklist/:itemId", workflowHandler.UpdateChecklistItem)
	}
}

func initializePursuitRoutes(apiVersion *gin.RouterGroup, pursuitHandler *pursuit.Handler) {
	pursuitRoutes := apiVersion.Group("/pursuits")
	{
		pursuitRoutes.GET("/", pursuitHandler.List)
		pursuitRoutes.POST("/", pursuitHandler.Create)
		pursuitRoutes.GET("/dashboard", pursuitHandler.Dashboard)
		pursuitRoutes.GET("/brief", pursuitHandler.Brief)
		pursuitRoutes.GET("/decisions", pursuitHandler.Decisions)
		pursuitRoutes.POST("/match", pursuitHandler.Match)
		pursuitRoutes.POST("/intake", pursuitHandler.RouteIntake)
		pursuitRoutes.GET("/:id/evidence", pursuitHandler.ResolveEvidence)
		pursuitRoutes.GET("/:id", pursuitHandler.Get)
		pursuitRoutes.PATCH("/:id", pursuitHandler.Update)
		pursuitRoutes.POST("/:id/archive", pursuitHandler.Archive)
		pursuitRoutes.POST("/:id/summary", pursuitHandler.RefreshSummary)
		pursuitRoutes.POST("/:id/review", pursuitHandler.Review)
		pursuitRoutes.POST("/:id/decisions/resolve", pursuitHandler.ResolveDecision)
		pursuitRoutes.GET("/:id/activity", pursuitHandler.Activity)
		pursuitRoutes.GET("/:id/next-actions", pursuitHandler.NextActions)
		pursuitRoutes.GET("/:id/blockers", pursuitHandler.Blockers)
		pursuitRoutes.GET("/:id/approvals", pursuitHandler.Approvals)
		pursuitRoutes.POST("/:id/intake", pursuitHandler.Intake)
		pursuitRoutes.POST("/:id/plan", pursuitHandler.Plan)
		pursuitRoutes.POST("/:id/links", pursuitHandler.Link)
		pursuitRoutes.DELETE("/:id/links/:linkId", pursuitHandler.DeleteLink)
	}
}
