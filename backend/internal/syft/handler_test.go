package syft

import (
	"automation-hub-backend/internal/identity"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeService struct{ workspace string }

func (s *fakeService) Status() Status { return Status{Enabled: true, Configured: true} }
func (s *fakeService) Probe(context.Context) (*ProbeResult, error) { return &ProbeResult{Reachable: true, Engine: "syft 1.48.0"}, nil }
func (s *fakeService) Inventory(_ context.Context, workspaceID string) (*InventoryResult, error) {
	s.workspace = workspaceID
	if workspaceID != "review-snapshot" { return nil, ErrWorkspace }
	return &InventoryResult{Status: "completed", Engine: "syft 1.48.0", WorkspaceID: workspaceID, PackageCount: 0, Ecosystems: []EcosystemCount{}, ResultDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil
}

type workflowInventoryStub struct {
	fakeService
	owner      string
	workflowID string
}

func (s *workflowInventoryStub) InventoryWithWorkflow(_ context.Context, ownerIdentity, workspaceID, workflowID string) (*InventoryResult, error) {
	s.owner = ownerIdentity
	s.workflowID = workflowID
	result, err := s.fakeService.Inventory(context.Background(), workspaceID)
	if result != nil {
		result.WorkflowID = workflowID
		result.WorkflowLinkStatus = "linked_review_signal"
	}
	return result, err
}

func TestInventoryHandlerRequiresNamedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	handler := NewHandler(service)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/inventory", strings.NewReader(`{}`))
	context.Request.Header.Set("Content-Type", "application/json")
	handler.Inventory(context)
	if response.Code != http.StatusBadRequest || service.workspace != "" { t.Fatalf("response=%d workspace=%q body=%s", response.Code, service.workspace, response.Body.String()) }
}

func TestInventoryHandlerPassesOnlyWorkspaceIdentifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	handler := NewHandler(service)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/inventory", strings.NewReader(`{"workspaceId":"review-snapshot","path":"C:/not-accepted"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	handler.Inventory(context)
	if response.Code != http.StatusOK || service.workspace != "review-snapshot" { t.Fatalf("response=%d workspace=%q body=%s", response.Code, service.workspace, response.Body.String()) }
}

func TestInventoryHandlerForwardsOwnerAndWorkflowToWorkflowInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &workflowInventoryStub{}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "owner@example.test")
		c.Next()
	})
	router.POST("/inventory", handler.Inventory)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/inventory", strings.NewReader(`{"workspaceId":"review-snapshot","workflowId":"7f4b2da3-4678-47dc-9558-feb9540c3a3a"}`)))
	if response.Code != http.StatusOK || service.owner != "owner@example.test" || service.workflowID != "7f4b2da3-4678-47dc-9558-feb9540c3a3a" || !strings.Contains(response.Body.String(), "linked_review_signal") {
		t.Fatalf("response=%d body=%s owner=%q workflow=%q", response.Code, response.Body.String(), service.owner, service.workflowID)
	}
}
