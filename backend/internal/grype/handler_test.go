package grype

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

type handlerStubService struct {
	status Status
	result *ScanResult
	err    error
}

func (s handlerStubService) Status() Status { return s.status }
func (s handlerStubService) Probe(context.Context) (*ProbeResult, error) {
	return &ProbeResult{Reachable: true, Engine: "grype 0.116.0", CheckedAt: time.Now()}, nil
}
func (s handlerStubService) Scan(_ context.Context, workspace string) (*ScanResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := *s.result
	result.WorkspaceID = workspace
	return &result, nil
}

func TestHandlerRequiresAuthenticatedSubjectAndReturnsAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	result := &ScanResult{Status: "completed", Engine: "grype 0.116.0", VulnerabilityCount: 0, FixAvailableCount: 0, Severities: []SeverityCount{}, DurationMS: 1, ResultDigest: strings.Repeat("a", 64)}
	handler := NewHandler(handlerStubService{status: Status{Configured: true}, result: result})
	router := gin.New()
	router.POST("/scan", handler.Scan)

	missingOwner := httptest.NewRecorder()
	router.ServeHTTP(missingOwner, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{"workspaceId":"review-snapshot"}`)))
	if missingOwner.Code != http.StatusUnauthorized {
		t.Fatalf("missing owner status = %d", missingOwner.Code)
	}

	authenticated := gin.New()
	authenticated.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "owner@example.test")
		c.Next()
	})
	authenticated.POST("/scan", handler.Scan)
	response := httptest.NewRecorder()
	authenticated.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{"workspaceId":"review-snapshot"}`)))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "CVE") || !strings.Contains(response.Body.String(), "vulnerabilityCount") {
		t.Fatalf("scan response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsMissingWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(handlerStubService{status: Status{Configured: true}, result: &ScanResult{}})
	router := gin.New()
	router.POST("/scan", handler.Scan)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/scan", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d", response.Code)
	}
}
