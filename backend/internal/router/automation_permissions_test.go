package router

import (
	"automation-hub-backend/internal/agentruntime"
	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAutomationRoutesApplySignedRolePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firstID := uuid.New()
	secondID := uuid.New()

	newEngine := func(role string) *gin.Engine {
		engine := gin.New()
		v1 := engine.Group("/api/v1")
		v1.Use(func(c *gin.Context) {
			c.Set(identity.ContextSubjectKey, "robert")
			c.Set(identity.ContextRoleKey, role)
			c.Next()
		})
		if err := initializeAutomationsRoutes(v1, automation.NewHandler(automationRouteServiceStub{})); err != nil {
			t.Fatalf("initialize automation routes: %v", err)
		}
		return engine
	}

	operatorConfig := httptest.NewRecorder()
	newEngine("operator").ServeHTTP(operatorConfig, httptest.NewRequest(http.MethodPatch, "/api/v1/automation/swap/"+firstID.String()+"/"+secondID.String(), nil))
	if operatorConfig.Code != http.StatusForbidden {
		t.Fatalf("operator reorder status = %d, want %d: %s", operatorConfig.Code, http.StatusForbidden, operatorConfig.Body.String())
	}

	operatorLaunch := httptest.NewRecorder()
	newEngine("operator").ServeHTTP(operatorLaunch, httptest.NewRequest(http.MethodPost, "/api/v1/automation/"+firstID.String()+"/launch", nil))
	if operatorLaunch.Code != http.StatusOK {
		t.Fatalf("operator launch status = %d, want %d: %s", operatorLaunch.Code, http.StatusOK, operatorLaunch.Body.String())
	}

	viewerLaunch := httptest.NewRecorder()
	newEngine("viewer").ServeHTTP(viewerLaunch, httptest.NewRequest(http.MethodPost, "/api/v1/automation/"+firstID.String()+"/launch", nil))
	if viewerLaunch.Code != http.StatusForbidden {
		t.Fatalf("viewer launch status = %d, want %d: %s", viewerLaunch.Code, http.StatusForbidden, viewerLaunch.Body.String())
	}

	ownerConfig := httptest.NewRecorder()
	newEngine("owner").ServeHTTP(ownerConfig, httptest.NewRequest(http.MethodPatch, "/api/v1/automation/swap/"+firstID.String()+"/"+secondID.String(), nil))
	if ownerConfig.Code != http.StatusOK {
		t.Fatalf("owner reorder status = %d, want %d: %s", ownerConfig.Code, http.StatusOK, ownerConfig.Body.String())
	}
}

type automationRouteServiceStub struct{}

func (automationRouteServiceStub) FindByID(uuid.UUID) (*models.Automation, error) {
	return &models.Automation{}, nil
}
func (automationRouteServiceStub) Create(item *models.Automation) (*models.Automation, error) {
	return item, nil
}
func (automationRouteServiceStub) Update(item *models.Automation) (*models.Automation, error) {
	return item, nil
}
func (automationRouteServiceStub) Delete(uuid.UUID) error { return nil }
func (automationRouteServiceStub) FindAll() ([]*models.Automation, error) {
	return []*models.Automation{}, nil
}
func (automationRouteServiceStub) SwapOrder(uuid.UUID, uuid.UUID) error { return nil }
func (automationRouteServiceStub) RunHealthCheck(id uuid.UUID) (*automation.HealthResult, error) {
	return &automation.HealthResult{AutomationID: id, Status: "healthy"}, nil
}
func (automationRouteServiceStub) HealthSummary() (*automation.HealthSummary, error) {
	return &automation.HealthSummary{}, nil
}
func (automationRouteServiceStub) Launch(id uuid.UUID) (*automation.LaunchResult, error) {
	return &automation.LaunchResult{AutomationID: id, Status: "completed"}, nil
}
func (automationRouteServiceStub) LaunchTask(id uuid.UUID, _ automation.TaskLaunchRequest) (*automation.LaunchResult, error) {
	return &automation.LaunchResult{AutomationID: id, Status: "completed"}, nil
}
func (automationRouteServiceStub) StopRuntimeTask(id uuid.UUID) (*agentruntime.StopResult, error) {
	return &agentruntime.StopResult{TaskID: id.String(), Status: "stopped"}, nil
}
func (automationRouteServiceStub) Diagnostics(id uuid.UUID) (*automation.DiagnosticResult, error) {
	return &automation.DiagnosticResult{AutomationID: id}, nil
}

var _ automation.Service = automationRouteServiceStub{}
