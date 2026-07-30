package router

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"automation-hub-backend/docs"
	"automation-hub-backend/internal/a2abridge"
	"automation-hub-backend/internal/accountfeed"
	"automation-hub-backend/internal/agentcycle"
	"automation-hub-backend/internal/agentframework"
	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/ambient"
	"automation-hub-backend/internal/anythingllm"
	"automation-hub-backend/internal/assistant"
	"automation-hub-backend/internal/autogencompat"
	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/autonomy"
	"automation-hub-backend/internal/braincatalog"
	"automation-hub-backend/internal/browserverify"
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/crewai"
	"automation-hub-backend/internal/deepeval"
	"automation-hub-backend/internal/deepteam"
	"automation-hub-backend/internal/docling"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/evidently"
	"automation-hub-backend/internal/featureflags"
	"automation-hub-backend/internal/garak"
	"automation-hub-backend/internal/gitleaks"
	"automation-hub-backend/internal/gosec"
	"automation-hub-backend/internal/grype"
	"automation-hub-backend/internal/guardrails"
	"automation-hub-backend/internal/haios"
	"automation-hub-backend/internal/hardwareprofile"
	"automation-hub-backend/internal/health"
	"automation-hub-backend/internal/i18n"
	"automation-hub-backend/internal/langfuse"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/lmeval"
	"automation-hub-backend/internal/mcpbridge"
	"automation-hub-backend/internal/mcppreflight"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/miniswe"
	"automation-hub-backend/internal/mlflow"
	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/openlit"
	"automation-hub-backend/internal/opscontrol"
	"automation-hub-backend/internal/phase2"
	"automation-hub-backend/internal/planningoptimizer"
	"automation-hub-backend/internal/presidio"
	"automation-hub-backend/internal/privacyfilter"
	"automation-hub-backend/internal/promptfoo"
	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/pydanticai"
	"automation-hub-backend/internal/ragflow"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/internal/research"
	"automation-hub-backend/internal/runtimelab"
	"automation-hub-backend/internal/semantic"
	"automation-hub-backend/internal/serena"
	"automation-hub-backend/internal/source"
	"automation-hub-backend/internal/syft"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/temporalbridge"
	"automation-hub-backend/internal/trivy"
	"automation-hub-backend/internal/verification"
	"automation-hub-backend/internal/wasiexec"
	"automation-hub-backend/internal/whispercpp"
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
	router.GET("/readyz", readinessHandler(func(ctx context.Context) doctor.Report {
		// Static configuration diagnosis, then live dependency probes. The
		// second half is what makes the answer trustworthy: without it a
		// process with an unreachable database still reports itself ready.
		configured := doctor.Diagnose(config.AppConfig)
		live := doctor.RunProbes(ctx, health.DefaultTimeout, health.Probes(config.AppConfig))
		return configured.Merge(live...)
	}))

	relativePathV1 := config.AppConfig.BaseUrl + "/v1"
	docs.SwaggerInfo.BasePath = relativePathV1
	v1 := router.Group(relativePathV1)
	v1.Use(backendAPIKeyMiddleware())
	v1.Use(identityMiddleware())
	{
		runtimeRegistry := agentruntime.DefaultRegistry()
		// The automation executor and runtime-control routes must share one
		// registry so an approved task can be cancelled by its actual owner.
		automationService := automation.NewServiceWithRuntimeRegistry(
			automation.DefaultRepository(),
			*events.DefaultPublisher(),
			runtimeRegistry,
		)
		runtimeHandler := agentruntime.NewHandler(runtimeRegistry)
		initializeAgentRuntimeRoutes(v1, runtimeHandler)
		catalogReviewHistory, err := braincatalog.DefaultUpstreamReviewHistoryRepository()
		if err != nil {
			return err
		}
		catalogCollectionReviewHistory, err := braincatalog.DefaultCollectionReviewHistoryRepository()
		if err != nil {
			return err
		}
		catalogRepositoryDiscoveryReviewHistory, err := braincatalog.DefaultRepositoryDiscoveryReviewHistoryRepository()
		if err != nil {
			return err
		}
		catalogRepositoryScout := braincatalog.NewOSSInsightRepositoryScout(nil)
		catalogMaintenance := braincatalog.NewCatalogMaintenanceService(braincatalog.NewUpstreamReviewer(nil), catalogReviewHistory).
			WithCollectionMaintenance(braincatalog.NewOSSInsightCollectionReviewer(nil), catalogCollectionReviewHistory).
			WithRepositoryDiscoveryMaintenance(catalogRepositoryScout, catalogRepositoryDiscoveryReviewHistory)
		initializeBrainCatalogRoutes(v1, braincatalog.NewHandlerWithReviewersAndScout(braincatalog.NewUpstreamReviewer(nil), braincatalog.NewOSSInsightCollectionReviewer(nil), catalogRepositoryScout).WithMaintenance(catalogMaintenance))
		initializeAutoGenCompatibilityRoutes(v1, autogencompat.NewHandler(autogencompat.DefaultService()))
		initializeMCPPreflightRoutes(v1, mcppreflight.NewHandler(mcppreflight.NewServiceFromEnv()))
		initializePlanningOptimizerRoutes(v1, planningoptimizer.NewHandler(planningoptimizer.DefaultService()))
		researchService := research.DefaultService()
		initializeResearchRoutes(v1, research.NewHandler(researchService))
		ragflowService := ragflow.DefaultService()
		initializeRAGFlowRoutes(v1, ragflow.NewHandler(ragflowService))
		initializeAnythingLLMRoutes(v1, anythingllm.NewHandler(anythingllm.DefaultService()))
		initializeSerenaRoutes(v1, serena.NewHandler(serena.DefaultService()))
		initializePresidioRoutes(v1, presidio.NewHandler(presidio.DefaultService()))
		initializeEvidentlyRoutes(v1, evidently.NewHandler(evidently.DefaultService()))
		initializeGuardrailsRoutes(v1, guardrails.NewHandler(guardrails.DefaultService()))
		initializeLangfuseRoutes(v1, langfuse.NewHandler(langfuse.DefaultService()))
		initializeOpenLITRoutes(v1, openlit.NewHandler(openlit.DefaultService()))
		initializeMLflowRoutes(v1, mlflow.NewHandler(mlflow.DefaultService()))
		whisperService := whispercpp.DefaultService()
		initializeWhisperCPPRoutes(v1, whispercpp.NewHandler(whisperService))
		doclingService := docling.DefaultService()
		initializeDoclingRoutes(v1, docling.NewHandler(doclingService))
		initializeWASIRoutes(v1, wasiexec.NewHandler(wasiexec.DefaultService()))
		autoHandler := automation.NewHandler(automationService)
		err = initializeAutomationsRoutes(v1, autoHandler)
		if err != nil {
			return err
		}
		probeHistory, err := llm.DefaultProbeHistoryRepository()
		if err != nil {
			return err
		}
		maintenanceHistory, err := llm.DefaultModelMaintenanceRepository()
		if err != nil {
			return err
		}
		generationHistory, err := llm.DefaultGenerationHistoryRepository()
		if err != nil {
			return err
		}
		llmService, err := llm.NewServiceFromEnvWithOperationalHistories(probeHistory, maintenanceHistory, generationHistory)
		if err != nil {
			return err
		}
		llmHandler := llm.NewHandler(llmService)
		initializeLLMRoutes(v1, llmHandler)
		initializeLMEvalRoutes(v1, lmeval.NewHandler(lmeval.WithModelMaintenance(lmeval.DefaultService(), llmService)))
		initializePromptfooRoutes(v1, promptfoo.NewHandler(promptfoo.WithModelMaintenance(promptfoo.DefaultService(), llmService)))
		initializeDeepEvalRoutes(v1, deepeval.NewHandler(deepeval.WithModelMaintenance(deepeval.DefaultService(), llmService)))
		initializeDeepTeamRoutes(v1, deepteam.NewHandler(deepteam.WithModelMaintenance(deepteam.DefaultService(), llmService)))
		initializeGarakRoutes(v1, garak.NewHandler(garak.WithModelMaintenance(garak.DefaultService(), llmService)))
		initializePydanticAIRoutes(v1, pydanticai.NewHandler(pydanticai.WithModelMaintenance(pydanticai.DefaultService(), llmService)))
		initializeCrewAIRoutes(v1, crewai.NewHandler(crewai.WithModelMaintenance(crewai.DefaultService(), llmService)))
		initializeAgentFrameworkRoutes(v1, agentframework.NewHandler(agentframework.WithModelMaintenance(agentframework.DefaultService(), llmService)))
		semanticService := semantic.NewServiceFromEnv()
		memoryService := memory.NewServiceWithSemantic(memory.DefaultRepository(), semanticService)
		initializeMemoryRoutes(v1, memory.NewHandler(memoryService))
		workflowRunner := workflowtask.NewDeferredRunner()
		workflowService := workflow.NewServiceWithTaskRunner(workflow.DefaultRepository(), workflowRunner, memoryService)
		browserVerificationService := browserverify.DefaultService()
		if workflowLinker, ok := workflowService.(browserverify.WorkflowLinker); ok {
			browserVerificationService = browserverify.DefaultService(workflowLinker)
		}
		initializeBrowserVerificationRoutes(v1, browserverify.NewHandler(browserVerificationService))
		gitleaksService := gitleaks.DefaultService()
		if workflowLinker, ok := workflowService.(gitleaks.WorkflowLinker); ok {
			gitleaksService = gitleaks.DefaultService(workflowLinker)
		}
		initializeGitleaksRoutes(v1, gitleaks.NewHandler(gitleaksService))
		initializeGosecRoutes(v1, gosec.NewHandler(gosec.DefaultService()))
		initializeTrivyRoutes(v1, trivy.NewHandler(trivy.DefaultService()))
		initializeGrypeRoutes(v1, grype.NewHandler(grype.DefaultService()))
		syftService := syft.DefaultService()
		if workflowLinker, ok := workflowService.(syft.WorkflowLinker); ok {
			syftService = syft.DefaultService(workflowLinker)
		}
		initializeSyftRoutes(v1, syft.NewHandler(syftService))
		initializeMiniSWERoutes(v1, miniswe.NewHandler(miniswe.WithModelMaintenance(miniswe.DefaultService(workflowService), llmService)))
		temporalService := temporalbridge.NewServiceFromEnv(workflowService)
		temporalService.StartWorkerEventually(context.Background())
		initializeTemporalRoutes(v1, temporalbridge.NewHandler(temporalService))
		pursuitService := pursuit.NewService(pursuit.DefaultRepository(), workflowService)
		sourceService := source.NewServiceWithWorkflowPursuitAndSemantic(source.DefaultRepository(), memoryService, workflowService, pursuitService, semanticService)
		verificationService := verification.NewServiceWithCandidateRetrieval(verification.DefaultRepository(), sourceService, memoryService, ragflowService, researchService, pursuitService)
		initializeVerificationRoutes(v1, verification.NewHandler(verificationService))
		taskService := task.NewServiceWithEnginesAndPursuitAttemptsAndRAGFlow(
			memoryService,
			llmService,
			sourceService,
			verificationService,
			task.NewAutomationToolExecutor(automationService),
			pursuitService,
			ragflowService,
		)
		planningPreview, _ := taskService.(task.PreviewService)
		a2aBridgeHandler := a2abridge.NewHandler(a2abridge.NewServiceFromEnv(planningPreview))
		initializeA2ABridgeStatusRoutes(v1, a2aBridgeHandler)
		initializeA2ABridgeRoutes(router, relativePathV1, a2aBridgeHandler)
		workflowRunner.Set(workflowtask.NewRunner(taskService))
		source.StartScheduler(context.Background(), sourceService)
		workflow.StartScheduler(context.Background(), workflowService)
		mcpBridgeService := mcpbridge.NewServiceFromEnv(workflowService, sourceService).WithModelMaintenance(llmService)
		mcpBridgeHandler := mcpbridge.NewHandler(mcpBridgeService)
		initializeMCPBridgeStatusRoutes(v1, mcpBridgeHandler)
		initializeMCPAgentRoutes(router, relativePathV1, mcpBridgeHandler)
		initializeSourceRoutes(v1, source.NewHandlerWithDocumentExtractor(sourceService, whisperService, doclingService))
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
		modelIntelService := modelintelligence.DefaultService().WithModelMaintenance(llmService)
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
		opsControlService := phase2Module.OpsControl()
		// Model maintenance may download an operator-configured local Ollama
		// model. It therefore observes the same persisted emergency stop as
		// every other background operation.
		llm.StartModelMaintenanceScheduler(context.Background(), llmService, func() bool {
			return !opsControlService.Control().EmergencyStop()
		})
		braincatalog.StartCatalogRevalidationScheduler(context.Background(), catalogMaintenance, func() bool {
			return !opsControlService.Control().EmergencyStop()
		})
		initializeOpsControlRoutes(v1, opscontrol.NewHandler(opsControlService))
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

func initializeBrainCatalogRoutes(apiVersion *gin.RouterGroup, handler *braincatalog.Handler) {
	routes := apiVersion.Group("/brain-catalog")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/", requirePermission(rbac.PermRead), handler.List)
		routes.GET("/adoption-plan", requirePermission(rbac.PermRead), handler.AdoptionPlan)
		routes.GET("/revalidation-history", requirePermission(rbac.PermRead), handler.RevalidationHistory)
		routes.GET("/collection-revalidation-history", requirePermission(rbac.PermRead), handler.CollectionRevalidationHistory)
		routes.GET("/repository-discovery-revalidation-history", requirePermission(rbac.PermRead), handler.RepositoryDiscoveryRevalidationHistory)
		routes.POST("/revalidation/run", requirePermission(rbac.PermAdmin), handler.RunDueRevalidations)
		routes.POST("/collection-revalidation/run", requirePermission(rbac.PermAdmin), handler.RunDueCollectionRevalidation)
		routes.POST("/repository-discovery-revalidation/run", requirePermission(rbac.PermAdmin), handler.RunDueRepositoryDiscoveryRevalidation)
		routes.POST("/ossinsight/revalidate", requirePermission(rbac.PermAdmin), handler.RevalidateCollections)
		routes.POST("/ossinsight/discover", requirePermission(rbac.PermAdmin), handler.DiscoverRepositories)
		routes.POST("/ossinsight/discover/reviewable", requirePermission(rbac.PermAdmin), handler.DiscoverReviewableRepositories)
		routes.POST("/ossinsight/discoveries/revalidate", requirePermission(rbac.PermAdmin), handler.RevalidateDiscovery)
		routes.POST("/recommend", requirePermission(rbac.PermRead), handler.RecommendCapabilities)
		routes.GET("/:id", requirePermission(rbac.PermRead), handler.Get)
		routes.POST("/:id/revalidate", requirePermission(rbac.PermAdmin), handler.Revalidate)
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
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-HAI-Backend-Key")
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
		llmRoutes.GET("/model-maintenance", requirePermission(rbac.PermRead), llmHandler.ModelMaintenanceHistory)
		llmRoutes.POST("/model-maintenance/run", requirePermission(rbac.PermAdmin), llmHandler.RunDueModelMaintenance)
		llmRoutes.GET("/generations", requirePermission(rbac.PermRead), llmHandler.GenerationHistory)
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
		memoryRoutes.GET("/health", requirePermission(rbac.PermRead), memoryHandler.Health)
		memoryRoutes.POST("/", requirePermission(rbac.PermWrite), memoryHandler.Create)
		memoryRoutes.POST("/retrieve", requirePermission(rbac.PermRead), memoryHandler.Retrieve)
		memoryRoutes.POST("/semantic/reindex", requirePermission(rbac.PermWrite), memoryHandler.ReindexSemantic)
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
		// The HTTP handler scopes this batch to the authenticated owner. The
		// separate in-process scheduler is the only global source worker.
		sourceRoutes.POST("/sync-due", requirePermission(rbac.PermWrite), sourceHandler.RunDueScheduledSyncs)
		sourceRoutes.GET("/sync-jobs", requirePermission(rbac.PermRead), sourceHandler.SyncJobs)
		sourceRoutes.GET("/extractions", requirePermission(rbac.PermRead), sourceHandler.Extractions)
		sourceRoutes.GET("/knowledge-graph", requirePermission(rbac.PermRead), sourceHandler.KnowledgeGraph)
		sourceRoutes.GET("/audit-logs", requirePermission(rbac.PermRead), sourceHandler.AuditLogs)
		sourceRoutes.PATCH("/extractions/:id", requirePermission(rbac.PermWrite), sourceHandler.UpdateExtraction)
		sourceRoutes.POST("/extractions/:id/archive", requirePermission(rbac.PermWrite), sourceHandler.ArchiveExtraction)
		sourceRoutes.DELETE("/extractions/:id", requirePermission(rbac.PermWrite), sourceHandler.DeleteExtraction)
		sourceRoutes.PATCH("/:id", requirePermission(rbac.PermWrite), sourceHandler.UpdateSource)
		sourceRoutes.POST("/:id/sync", requirePermission(rbac.PermWrite), sourceHandler.Sync)
		sourceRoutes.POST("/:id/transcribe", requirePermission(rbac.PermWrite), sourceHandler.Transcribe)
		sourceRoutes.POST("/:id/extract-documents", requirePermission(rbac.PermWrite), sourceHandler.ExtractDocuments)
		sourceRoutes.POST("/:id/reindex", requirePermission(rbac.PermWrite), sourceHandler.Reindex)
		sourceRoutes.POST("/:id/pause", requirePermission(rbac.PermWrite), sourceHandler.Pause)
		sourceRoutes.POST("/:id/resume", requirePermission(rbac.PermWrite), sourceHandler.Resume)
		sourceRoutes.POST("/:id/revoke", requirePermission(rbac.PermWrite), sourceHandler.Revoke)
	}

	// Google OAuth for the Gmail connector. Not under requireAuthenticatedOwner:
	// the callback is invoked directly by Google with no HAI session, and is
	// protected by the HMAC-signed OAuth state instead. start needs write; the
	// public callback resolves to viewer, so it is gated read.
	sourceOAuth := apiVersion.Group("/sources")
	{
		sourceOAuth.GET("/oauth/google/start", requirePermission(rbac.PermWrite), sourceHandler.StartGoogleOAuth)
		sourceOAuth.GET("/oauth/google/callback", requirePermission(rbac.PermRead), sourceHandler.GoogleOAuthCallback)
	}
}

func initializeWhisperCPPRoutes(apiVersion *gin.RouterGroup, handler *whispercpp.Handler) {
	routes := apiVersion.Group("/whispercpp")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermWrite), handler.Probe)
	}
}

func initializeDoclingRoutes(apiVersion *gin.RouterGroup, handler *docling.Handler) {
	routes := apiVersion.Group("/docling")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
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

func initializeResearchRoutes(apiVersion *gin.RouterGroup, handler *research.Handler) {
	routes := apiVersion.Group("/research")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/search", requirePermission(rbac.PermWrite), handler.Search)
	}
}

func initializeRAGFlowRoutes(apiVersion *gin.RouterGroup, handler *ragflow.Handler) {
	routes := apiVersion.Group("/ragflow")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/retrieve", requirePermission(rbac.PermWrite), handler.Retrieve)
	}
}

// initializeMiniSWERoutes exposes only a diff-only patch proposal path. A
// request must be owner-scoped and requires the same explicit approval role as
// other consequential workflow actions.
func initializeMiniSWERoutes(apiVersion *gin.RouterGroup, handler *miniswe.Handler) {
	routes := apiVersion.Group("/mini-swe")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.GET("/jobs", requirePermission(rbac.PermRead), handler.Jobs)
		routes.POST("/workflows/:id/propose-patch", requirePermission(rbac.PermApprove), handler.ProposePatch)
	}
}

func initializeAnythingLLMRoutes(apiVersion *gin.RouterGroup, handler *anythingllm.Handler) {
	routes := apiVersion.Group("/anythingllm")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/retrieve", requirePermission(rbac.PermWrite), handler.Retrieve)
	}
}

// initializeSerenaRoutes exposes one bounded read-only semantic lookup. The
// service never launches Serena or forwards a generic MCP call surface.
func initializeSerenaRoutes(apiVersion *gin.RouterGroup, handler *serena.Handler) {
	routes := apiVersion.Group("/serena")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/symbols", requirePermission(rbac.PermWrite), handler.FindSymbols)
	}
}

func initializePresidioRoutes(apiVersion *gin.RouterGroup, handler *presidio.Handler) {
	routes := apiVersion.Group("/presidio")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		// Text leaves the request process only for the explicitly configured local
		// analyzer; analysis has no persistence or external-action capability.
		routes.POST("/analyze", requirePermission(rbac.PermWrite), handler.Analyze)
	}
}

func initializeEvidentlyRoutes(apiVersion *gin.RouterGroup, handler *evidently.Handler) {
	routes := apiVersion.Group("/evidently")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/evaluate", requirePermission(rbac.PermWrite), handler.Evaluate)
	}
}

func initializeGuardrailsRoutes(apiVersion *gin.RouterGroup, handler *guardrails.Handler) {
	routes := apiVersion.Group("/guardrails")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/validate", requirePermission(rbac.PermWrite), handler.Validate)
	}
}

func initializeLMEvalRoutes(apiVersion *gin.RouterGroup, handler *lmeval.Handler) {
	routes := apiVersion.Group("/lm-eval")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

func initializePromptfooRoutes(apiVersion *gin.RouterGroup, handler *promptfoo.Handler) {
	routes := apiVersion.Group("/promptfoo")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

func initializeDeepEvalRoutes(apiVersion *gin.RouterGroup, handler *deepeval.Handler) {
	routes := apiVersion.Group("/deepeval")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

func initializeDeepTeamRoutes(apiVersion *gin.RouterGroup, handler *deepteam.Handler) {
	routes := apiVersion.Group("/deepteam")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

func initializeGarakRoutes(apiVersion *gin.RouterGroup, handler *garak.Handler) {
	routes := apiVersion.Group("/garak")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

// initializeGitleaksRoutes exposes a fixed snapshot identifier, never a
// filesystem path, report, secret, or scanner setting. Scans are owner-
// triggered and use the same administrator permission as local runtime probes.
func initializeGitleaksRoutes(apiVersion *gin.RouterGroup, handler *gitleaks.Handler) {
	routes := apiVersion.Group("/gitleaks")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/scan", requirePermission(rbac.PermAdmin), handler.Scan)
	}
}

// initializeGosecRoutes exposes a fixed Go snapshot identifier, never a
// filesystem path, source code, finding, rule, CWE, report, command, or
// scanner setting. Scans are owner-triggered aggregate evidence only.
func initializeGosecRoutes(apiVersion *gin.RouterGroup, handler *gosec.Handler) {
	routes := apiVersion.Group("/gosec")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/scan", requirePermission(rbac.PermAdmin), handler.Scan)
	}
}

// initializeTrivyRoutes exposes a fixed configuration snapshot identifier,
// never a filesystem path, image, vulnerability, secret, policy, report,
// command, or scanner setting. Scans are owner-triggered aggregate evidence
// only and do not authorize remediation.
func initializeTrivyRoutes(apiVersion *gin.RouterGroup, handler *trivy.Handler) {
	routes := apiVersion.Group("/trivy")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/scan", requirePermission(rbac.PermAdmin), handler.Scan)
	}
}

// initializeGrypeRoutes exposes a fixed snapshot identifier, never a
// filesystem path, vulnerability record, advisory database, command, or
// remediation. Scans are owner-triggered aggregate evidence only.
func initializeGrypeRoutes(apiVersion *gin.RouterGroup, handler *grype.Handler) {
	routes := apiVersion.Group("/grype")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/scan", requirePermission(rbac.PermAdmin), handler.Scan)
	}
}

// initializeSyftRoutes exposes a fixed snapshot identifier, never a
// filesystem path, SBOM, package-level metadata, or scanner setting. Inventory
// is owner-triggered and uses the same administrator permission as local probes.
func initializeSyftRoutes(apiVersion *gin.RouterGroup, handler *syft.Handler) {
	routes := apiVersion.Group("/syft")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/inventory", requirePermission(rbac.PermAdmin), handler.Inventory)
	}
}

func initializeLangfuseRoutes(apiVersion *gin.RouterGroup, handler *langfuse.Handler) {
	routes := apiVersion.Group("/langfuse")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/export/operational-snapshot", requirePermission(rbac.PermAdmin), handler.ExportOperationalSnapshot)
	}
}

// initializeOpenLITRoutes exposes only a fixed, owner-triggered aggregate
// OTLP snapshot. It accepts no caller-selected telemetry payload or settings.
func initializeOpenLITRoutes(apiVersion *gin.RouterGroup, handler *openlit.Handler) {
	routes := apiVersion.Group("/openlit")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/export/operational-snapshot", requirePermission(rbac.PermAdmin), handler.ExportOperationalSnapshot)
	}
}

// initializeMLflowRoutes exposes a fixed local evaluation-evidence view. The
// client cannot select experiments, metrics, filters, or any MLflow mutation.
func initializeMLflowRoutes(apiVersion *gin.RouterGroup, handler *mlflow.Handler) {
	routes := apiVersion.Group("/mlflow")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.GET("/runs", requirePermission(rbac.PermRead), handler.RecentRuns)
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
	rl.Use(requireAuthenticatedOwner())
	{
		rl.GET("/overview", requirePermission(rbac.PermRead), handler.Overview)
		rl.POST("/:runtimeId/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		rl.POST("/:runtimeId/self-test", requirePermission(rbac.PermApprove), handler.SelfTest)
		rl.GET("/:runtimeId/attempts", requirePermission(rbac.PermRead), handler.Attempts)
	}
}

func initializeMCPPreflightRoutes(apiVersion *gin.RouterGroup, handler *mcppreflight.Handler) {
	routes := apiVersion.Group("/mcp-preflight")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/overview", requirePermission(rbac.PermRead), handler.Overview)
		routes.POST("/:serverId/run", requirePermission(rbac.PermAdmin), handler.Run)
	}
}

// initializeA2ABridgeStatusRoutes stays inside HAI's normal authenticated
// owner API. It exposes configuration only, never peer tokens or task input.
func initializeA2ABridgeStatusRoutes(apiVersion *gin.RouterGroup, handler *a2abridge.Handler) {
	routes := apiVersion.Group("/a2a")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
	}
}

// initializeAutoGenCompatibilityRoutes exposes HAI's transient migration
// preview and non-executing Microsoft Agent Framework migration plan. It is
// not an AutoGen or Agent Framework runtime, importer, scheduler, or executor.
func initializeAutoGenCompatibilityRoutes(apiVersion *gin.RouterGroup, handler *autogencompat.Handler) {
	routes := apiVersion.Group("/autogen-compat")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/preview", requirePermission(rbac.PermWrite), handler.Preview)
		routes.POST("/migration-plan", requirePermission(rbac.PermWrite), handler.MigrationPlan)
	}
}

// initializeA2ABridgeRoutes implements a small local A2A compatibility
// boundary. The Agent Card carries no user context, and the JSON-RPC endpoint
// requires a separate bridge token rather than browser identity or API keys.
func initializeA2ABridgeRoutes(router *gin.Engine, relativePathV1 string, handler *a2abridge.Handler) {
	router.GET("/.well-known/agent-card.json", handler.AgentCard)
	router.POST(relativePathV1+"/a2a", handler.Send)
}

// initializeMCPBridgeStatusRoutes is part of the normal owner-authenticated
// API. It reports configuration only; the data surface below uses a separate
// narrow bridge token for the local FastMCP process.
func initializeMCPBridgeStatusRoutes(apiVersion *gin.RouterGroup, handler *mcpbridge.Handler) {
	routes := apiVersion.Group("/mcp-bridge")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
	}
}

// initializeMCPAgentRoutes intentionally avoids the browser identity and
// backend API-key middleware. It is reachable only with the dedicated bridge
// token, serves aggregate/sanitized read models, and is not a general API.
func initializeMCPAgentRoutes(router *gin.Engine, relativePathV1 string, handler *mcpbridge.Handler) {
	routes := router.Group(relativePathV1 + "/mcp-agent")
	{
		routes.GET("/overview", handler.Overview)
		routes.GET("/actionable", handler.Actionable)
		routes.GET("/github-repositories", handler.GitHubRepositories)
		routes.GET("/model-maintenance", handler.ModelMaintenanceReadiness)
	}
}

func initializePlanningOptimizerRoutes(apiVersion *gin.RouterGroup, handler *planningoptimizer.Handler) {
	routes := apiVersion.Group("/planning-optimizer")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.GET("/runs", requirePermission(rbac.PermRead), handler.Runs)
		routes.POST("/proposals", requirePermission(rbac.PermWrite), handler.Propose)
	}
}

func initializePydanticAIRoutes(apiVersion *gin.RouterGroup, handler *pydanticai.Handler) {
	routes := apiVersion.Group("/pydantic-ai")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/proposals", requirePermission(rbac.PermWrite), handler.Propose)
	}
}

// initializeCrewAIRoutes exposes only an opt-in, local, review-only planning
// draft. The external runner has no HAI credentials or execution authority.
func initializeCrewAIRoutes(apiVersion *gin.RouterGroup, handler *crewai.Handler) {
	routes := apiVersion.Group("/crewai")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/proposals", requirePermission(rbac.PermWrite), handler.Propose)
	}
}

// initializeAgentFrameworkRoutes exposes only a local, review-only sequential
// planning draft. Agent Framework receives no HAI credentials, tools, sources,
// memory, workflow state, or execution authority.
func initializeAgentFrameworkRoutes(apiVersion *gin.RouterGroup, handler *agentframework.Handler) {
	routes := apiVersion.Group("/agent-framework")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.POST("/probe", requirePermission(rbac.PermAdmin), handler.Probe)
		routes.POST("/proposals", requirePermission(rbac.PermWrite), handler.Propose)
	}
}

func initializeBrowserVerificationRoutes(apiVersion *gin.RouterGroup, handler *browserverify.Handler) {
	routes := apiVersion.Group("/browser-verification")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.GET("/profiles", requirePermission(rbac.PermRead), handler.Profiles)
		routes.GET("/runs", requirePermission(rbac.PermRead), handler.Runs)
		// A browser check is read-only but still approval-gated: it may expose an
		// application route's current state, so it is never an autonomous scan.
		routes.POST("/profiles/:id/run", requirePermission(rbac.PermApprove), handler.Run)
	}
}

func initializeWASIRoutes(apiVersion *gin.RouterGroup, handler *wasiexec.Handler) {
	routes := apiVersion.Group("/wasi")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.GET("/modules", requirePermission(rbac.PermRead), handler.Modules)
		routes.GET("/runs", requirePermission(rbac.PermRead), handler.Runs)
		routes.POST("/modules/:id/run", requirePermission(rbac.PermApprove), handler.Run)
	}
}

func initializeTemporalRoutes(apiVersion *gin.RouterGroup, handler *temporalbridge.Handler) {
	routes := apiVersion.Group("/temporal")
	routes.Use(requireAuthenticatedOwner())
	{
		routes.GET("/status", requirePermission(rbac.PermRead), handler.Status)
		routes.GET("/follow-up-runs", requirePermission(rbac.PermRead), handler.Runs)
		// A worker start only connects to a local, validated Temporal endpoint and
		// registers the proposal-only workflow. Scheduling it remains approval-gated.
		routes.POST("/worker/start", requirePermission(rbac.PermAdmin), handler.StartWorker)
		routes.POST("/follow-up-runs", requirePermission(rbac.PermApprove), handler.ScheduleFollowUp)
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
		// These HTTP worker controls are owner-scoped. They can advance work, so
		// operators need approval capability; global scheduler work stays internal.
		workflowRoutes.POST("/recover-stale", requirePermission(rbac.PermApprove), workflowHandler.RecoverStaleClaims)
		workflowRoutes.POST("/run-due", requirePermission(rbac.PermApprove), workflowHandler.RunDue)
		workflowRoutes.POST("/open-loops/run-due", requirePermission(rbac.PermApprove), workflowHandler.RunDueOpenLoops)
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
		pursuitRoutes.POST("/:id/candidate/accept", requirePermission(rbac.PermApprove), pursuitHandler.AcceptCandidate)
		pursuitRoutes.POST("/:id/links", requirePermission(rbac.PermWrite), pursuitHandler.Link)
		pursuitRoutes.DELETE("/:id/links/:linkId", requirePermission(rbac.PermWrite), pursuitHandler.DeleteLink)
	}
}
