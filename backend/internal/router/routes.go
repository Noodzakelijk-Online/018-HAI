package router

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"automation-hub-backend/docs"
	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/haios"
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
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

	relativePathV1 := config.AppConfig.BaseUrl + "/v1"
	docs.SwaggerInfo.BasePath = relativePathV1
	v1 := router.Group(relativePathV1)
	v1.Use(backendAPIKeyMiddleware())
	{
		autoHandler := automation.DefaultHandler()
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
		sourceService := source.NewServiceWithWorkflow(source.DefaultRepository(), memoryService, workflow.DefaultService())
		source.StartScheduler(context.Background(), sourceService)
		initializeSourceRoutes(v1, source.NewHandler(sourceService))
		verificationService := verification.NewService(verification.DefaultRepository(), sourceService, memoryService)
		initializeVerificationRoutes(v1, verification.NewHandler(verificationService))
		taskService := task.NewServiceWithEngines(memoryService, llmService, sourceService, verificationService)
		workflowRunner := workflowtask.NewRunner(taskService)
		workflowService := workflow.NewServiceWithTaskRunner(workflow.DefaultRepository(), workflowRunner)
		workflow.StartScheduler(context.Background(), workflowService)
		initializeWorkflowRoutes(v1, workflow.NewHandler(workflowService))
		osHandler, err := haios.DefaultHandler()
		if err != nil {
			return err
		}
		initializeHAIOSRoutes(v1, osHandler)
		initializeTaskRoutes(v1, task.NewHandler(taskService))
	}
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	return nil
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

func initializeSourceRoutes(apiVersion *gin.RouterGroup, sourceHandler *source.Handler) {
	sourceRoutes := apiVersion.Group("/sources")
	{
		sourceRoutes.GET("/connectors", sourceHandler.Connectors)
		sourceRoutes.GET("/", sourceHandler.Sources)
		sourceRoutes.POST("/", sourceHandler.CreateSource)
		sourceRoutes.POST("/search", sourceHandler.Search)
		sourceRoutes.POST("/sync-due", sourceHandler.RunDueScheduledSyncs)
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
		workflowRoutes.POST("/run-due", workflowHandler.RunDue)
		workflowRoutes.POST("/open-loops/run-due", workflowHandler.RunDueOpenLoops)
		workflowRoutes.GET("/:id", workflowHandler.Get)
		workflowRoutes.POST("/:id/transition", workflowHandler.Transition)
		workflowRoutes.POST("/:id/approval", workflowHandler.ResolveApproval)
		workflowRoutes.POST("/:id/proposals/:proposalId/resolve", workflowHandler.ResolveProposal)
		workflowRoutes.PATCH("/:id/checklist/:itemId", workflowHandler.UpdateChecklistItem)
	}
}
