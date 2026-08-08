package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestWorkflowAndPursuitDecisionsRequireOwnerApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	api := engine.Group("/api/v1")
	initializeWorkflowRoutes(api, workflow.NewHandler(nil))
	initializePursuitRoutes(api, pursuit.NewHandler(nil))

	decisionPaths := []string{
		"/api/v1/workflow/not-a-uuid/approval",
		"/api/v1/workflow/not-a-uuid/interruption/resolve",
		"/api/v1/workflow/not-a-uuid/proposals/not-a-uuid/resolve",
		"/api/v1/pursuits/not-a-uuid/decisions/resolve",
		"/api/v1/pursuits/not-a-uuid/candidate/accept",
	}
	for _, path := range decisionPaths {
		t.Run(path, func(t *testing.T) {
			for _, role := range []string{"viewer", "operator", "unknown"} {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, nil)
				request.Header.Set("X-Test-Verified-Role", role)
				engine.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusForbidden {
					t.Errorf("role %s status = %d, want 403: %s", role, recorder.Code, recorder.Body.String())
				}
			}

			owner := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Header.Set("X-Test-Verified-Role", "owner")
			engine.ServeHTTP(owner, request)
			if owner.Code != http.StatusBadRequest {
				t.Errorf("owner status = %d, want handler validation 400: %s", owner.Code, owner.Body.String())
			}
		})
	}
}

func TestPursuitPortfolioPlanRequiresAuthenticationAndReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pursuits/portfolio-plan", nil)
	request.Header.Set("X-HAI-Role", "owner")
	unauthenticated.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "owner"} {
		t.Run(role+" read permission", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/pursuits/portfolio-plan", nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want handler validation 400: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPursuitPortfolioAllocationAcceptanceRequiresOwnerApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pursuits/portfolio-plan/accept", nil)
	request.Header.Set("X-HAI-Role", "owner")
	unauthenticated.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "unknown"} {
		t.Run(role+" denied", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/pursuits/portfolio-plan/accept", nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	owner := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/pursuits/portfolio-plan/accept", nil)
	request.Header.Set("X-Test-Verified-Role", "owner")
	authenticated.ServeHTTP(owner, request)
	if owner.Code != http.StatusServiceUnavailable {
		t.Fatalf("owner status = %d, want handler availability check 503: %s", owner.Code, owner.Body.String())
	}
}

func TestPursuitPortfolioAllocationHistoryRequiresAuthenticationAndReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pursuits/portfolio-allocations", nil)
	request.Header.Set("X-HAI-Role", "owner")
	unauthenticated.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "owner"} {
		t.Run(role+" reads history", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/pursuits/portfolio-allocations", nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want handler availability check 503: %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	// Legacy read-only routes deliberately map an unsupported verified role to
	// viewer. The authenticated owner boundary still scopes the returned data.
	unknown := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/pursuits/portfolio-allocations", nil)
	request.Header.Set("X-Test-Verified-Role", "unknown")
	authenticated.ServeHTTP(unknown, request)
	if unknown.Code != http.StatusServiceUnavailable {
		t.Fatalf("unknown role status = %d, want viewer-level handler availability check 503: %s", unknown.Code, unknown.Body.String())
	}
}

func TestPursuitPortfolioExecutionProposalRequiresOwnerApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)
	path := "/api/v1/pursuits/portfolio-allocations/not-a-uuid/execution-proposals"

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("X-HAI-Role", "owner")
	unauthenticated.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "unknown"} {
		t.Run(role+" denied", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	owner := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("X-Test-Verified-Role", "owner")
	authenticated.ServeHTTP(owner, request)
	if owner.Code != http.StatusBadRequest {
		t.Fatalf("owner status = %d, want allocation validation 400: %s", owner.Code, owner.Body.String())
	}
}

func TestPursuitPortfolioExecutionProposalHistoryRequiresAuthenticatedRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)
	path := "/api/v1/pursuits/portfolio-execution-proposals?allocationIds=" + uuid.NewString()

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	unauthenticated.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "owner"} {
		t.Run(role+" reads proposal history", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want handler availability check 503: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPursuitPortfolioDispatchCoordinationBatchRequiresAuthenticatedRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)
	path := "/api/v1/pursuits/portfolio-execution-proposals/coordination?proposalIds=" + uuid.NewString()

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	recorder := httptest.NewRecorder()
	unauthenticated.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "owner"} {
		t.Run(role+" reads coordination", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want handler availability check 503: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPursuitPortfolioExecutionProposalDecisionRequiresOwnerApprovalAndHistoryRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)
	decisionPath := "/api/v1/pursuits/portfolio-execution-proposal-items/not-a-uuid/decisions"

	unauthenticated := gin.New()
	initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, decisionPath, nil)
		request.Header.Set("X-HAI-Role", "owner")
		unauthenticated.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status = %d, want 401: %s", method, recorder.Code, recorder.Body.String())
		}
	}

	authenticated := gin.New()
	authenticated.Use(testIdentityMiddleware())
	initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
	for _, role := range []string{"viewer", "operator", "unknown"} {
		t.Run(role+" cannot decide", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, decisionPath, nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	ownerDecision := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, decisionPath, nil)
	request.Header.Set("X-Test-Verified-Role", "owner")
	authenticated.ServeHTTP(ownerDecision, request)
	if ownerDecision.Code != http.StatusBadRequest {
		t.Fatalf("owner decision status = %d, want item validation 400: %s", ownerDecision.Code, ownerDecision.Body.String())
	}
	for _, role := range []string{"viewer", "operator", "owner"} {
		t.Run(role+" reads decision history", func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, decisionPath, nil)
			request.Header.Set("X-Test-Verified-Role", role)
			authenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want item validation 400: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPursuitPortfolioWorkflowEffectsRequireOwnerApprovalAndExecutePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := pursuit.NewHandler(nil)
	for _, suffix := range []string{"authorize-workflow", "execute-workflow", "settle-workflow"} {
		t.Run(suffix, func(t *testing.T) {
			path := "/api/v1/pursuits/portfolio-execution-proposal-items/not-a-uuid/" + suffix
			unauthenticated := gin.New()
			initializePursuitRoutes(unauthenticated.Group("/api/v1"), handler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Header.Set("X-HAI-Role", "owner")
			unauthenticated.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d, want 401: %s", recorder.Code, recorder.Body.String())
			}

			authenticated := gin.New()
			authenticated.Use(testIdentityMiddleware())
			initializePursuitRoutes(authenticated.Group("/api/v1"), handler)
			for _, role := range []string{"viewer", "operator", "unknown"} {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, nil)
				request.Header.Set("X-Test-Verified-Role", role)
				authenticated.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusForbidden {
					t.Errorf("role %s status = %d, want 403: %s", role, recorder.Code, recorder.Body.String())
				}
			}

			owner := httptest.NewRecorder()
			request = httptest.NewRequest(http.MethodPost, path, nil)
			request.Header.Set("X-Test-Verified-Role", "owner")
			authenticated.ServeHTTP(owner, request)
			if owner.Code != http.StatusBadRequest {
				t.Fatalf("owner status = %d, want item validation 400: %s", owner.Code, owner.Body.String())
			}
		})
	}
}
