package router

import (
	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/identity"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentRuntimeConfigurationRequiresOwnerRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newEngine := func(role string) *gin.Engine {
		engine := gin.New()
		v1 := engine.Group("/api/v1")
		v1.Use(func(c *gin.Context) {
			c.Set(identity.ContextSubjectKey, "alice")
			c.Set(identity.ContextRoleKey, role)
			c.Next()
		})
		initializeAgentRuntimeRoutes(v1, agentruntime.NewHandler(agentruntime.NewRegistry()))
		return engine
	}

	operator := httptest.NewRecorder()
	newEngine("operator").ServeHTTP(operator, httptest.NewRequest(http.MethodPatch, "/api/v1/agent-runtimes/openclaw/ecosystem", nil))
	if operator.Code != http.StatusForbidden {
		t.Fatalf("operator configuration status = %d, want %d: %s", operator.Code, http.StatusForbidden, operator.Body.String())
	}

	owner := httptest.NewRecorder()
	newEngine("owner").ServeHTTP(owner, httptest.NewRequest(http.MethodPatch, "/api/v1/agent-runtimes/openclaw/ecosystem", nil))
	if owner.Code != http.StatusBadRequest {
		t.Fatalf("owner should pass RBAC and reach request validation, got %d: %s", owner.Code, owner.Body.String())
	}
}

func TestAgentRuntimeStopAllowsApprovedOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	v1.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "operator-1")
		c.Set(identity.ContextRoleKey, "operator")
		c.Next()
	})
	initializeAgentRuntimeRoutes(v1, agentruntime.NewHandler(agentruntime.NewRegistry()))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtimes/openclaw/tasks/task-1/stop", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("operator should pass RBAC and reach the unregistered-runtime guard, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
