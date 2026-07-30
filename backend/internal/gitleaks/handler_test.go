package gitleaks

import (
	"automation-hub-backend/internal/identity"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type stubService struct {
	status Status
	result *ScanResult
	err    error
}

type workflowStubService struct {
	stubService
	owner      string
	workflowID string
}

func (s *workflowStubService) ScanWithWorkflow(_ context.Context, ownerIdentity, workspaceID, workflowID string) (*ScanResult, error) {
	s.owner = ownerIdentity
	s.workflowID = workflowID
	result, err := s.stubService.Scan(context.Background(), workspaceID)
	if result != nil {
		result.WorkflowID = workflowID
		result.WorkflowLinkStatus = "linked_security_signal"
	}
	return result, err
}

func (s stubService) Status() Status { return s.status }
func (s stubService) Probe(context.Context) (*ProbeResult, error) {
	return &ProbeResult{Reachable: true, Engine: "gitleaks 8.30.1", CheckedAt: time.Now()}, nil
}
func (s stubService) Scan(_ context.Context, workspace string) (*ScanResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := *s.result
	result.WorkspaceID = workspace
	return &result, nil
}

func TestHandlerRejectsMissingWorkspaceAndReturnsAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	result := &ScanResult{Status: "completed", Engine: "gitleaks 8.30.1", FindingCount: 0, AffectedFiles: 0, Rules: []RuleCount{}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	handler := NewHandler(stubService{status: Status{Configured: true}, result: result})
	router := gin.New()
	router.POST("/scan", handler.Scan)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{}`)))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d", missing.Code)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{"workspaceId":"review-snapshot"}`)))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("scan response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerForwardsOwnerAndWorkflowToWorkflowScanner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	result := &ScanResult{Status: "completed", Engine: "gitleaks 8.30.1", FindingCount: 0, AffectedFiles: 0, Rules: []RuleCount{}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	service := &workflowStubService{stubService: stubService{status: Status{Configured: true}, result: result}}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "owner@example.test")
		c.Next()
	})
	router.POST("/scan", handler.Scan)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{"workspaceId":"review-snapshot","workflowId":"7f4b2da3-4678-47dc-9558-feb9540c3a3a"}`)))
	if response.Code != http.StatusOK || service.owner != "owner@example.test" || service.workflowID != "7f4b2da3-4678-47dc-9558-feb9540c3a3a" || !strings.Contains(response.Body.String(), "linked_security_signal") {
		t.Fatalf("response=%d %s owner=%q workflow=%q", response.Code, response.Body.String(), service.owner, service.workflowID)
	}
}
