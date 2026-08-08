package outcomeevaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesRefusesIncompleteSecurityWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewHandler(NewService(NewMemoryRepository()))
	if err := RegisterRoutes(engine.Group("/api/v1"), handler, RouteGuards{}); err == nil {
		t.Fatal("unguarded outcome evaluation routes were registered")
	}
	guards := testRouteGuards()
	guards.Govern = nil
	if err := RegisterRoutes(engine.Group("/api/v1"), handler, guards); err == nil {
		t.Fatal("routes without a govern guard were registered")
	}
}

func TestHTTPAPIIsOwnerScopedStrictAndPermissionGated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	engine := gin.New()
	if err := RegisterRoutes(engine.Group("/api/v1"), NewHandler(service), testRouteGuards()); err != nil {
		t.Fatal(err)
	}

	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
		if strings.Contains(route.Path, "execute") || strings.Contains(route.Path, "apply") || strings.Contains(route.Path, "policy") {
			t.Fatalf("authority-bearing route registered: %s %s", route.Method, route.Path)
		}
	}
	for _, route := range []string{
		"PUT /api/v1/outcome-evaluations/workspaces/:workspaceId/outcomes/:outcomeId",
		"GET /api/v1/outcome-evaluations/workspaces/:workspaceId/outcomes/:outcomeId/history",
		"POST /api/v1/outcome-evaluations/workspaces/:workspaceId/outcomes/:outcomeId/evaluations",
		"GET /api/v1/outcome-evaluations/workspaces/:workspaceId/outcomes/:outcomeId/evaluations/:evaluationId",
		"POST /api/v1/outcome-evaluations/workspaces/:workspaceId/outcomes/:outcomeId/corrections",
	} {
		if !registered[route] {
			t.Fatalf("required route not registered: %s", route)
		}
	}

	storeBody := mustJSON(t, StoreOutcomeRequest{
		IdempotencyKey: "http-outcome-1", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	path := "/api/v1/outcome-evaluations/workspaces/workspace-1/outcomes/outcome-1"
	operatorStore := performRequest(engine, http.MethodPut, path, storeBody, "owner-1", "operator", "application/json")
	if operatorStore.Code != http.StatusForbidden {
		t.Fatalf("operator store status = %d: %s", operatorStore.Code, operatorStore.Body.String())
	}
	store := performRequest(engine, http.MethodPut, path, storeBody, "owner-1", "owner", "application/json")
	if store.Code != http.StatusCreated {
		t.Fatalf("owner store status = %d: %s", store.Code, store.Body.String())
	}
	spoofedOutcome := validRequest().Outcome
	spoofedOutcome.ID = "body-controlled-id"
	spoofedOutcome.Scope = Scope{OwnerID: "other-owner", WorkspaceID: "other-workspace"}
	for index := range spoofedOutcome.Indicators {
		spoofedOutcome.Indicators[index].Baseline.Scope = spoofedOutcome.Scope
	}
	boundStore := performRequest(
		engine,
		http.MethodPut,
		"/api/v1/outcome-evaluations/workspaces/workspace-1/outcomes/outcome-bound-by-server",
		mustJSON(t, StoreOutcomeRequest{
			IdempotencyKey: "http-outcome-server-scope", ExpectedRevision: 0, Outcome: spoofedOutcome,
		}),
		"owner-1",
		"owner",
		"application/json",
	)
	if boundStore.Code != http.StatusCreated {
		t.Fatalf("server-bound store status = %d: %s", boundStore.Code, boundStore.Body.String())
	}
	var boundRevision OutcomeRevision
	if err := json.Unmarshal(boundStore.Body.Bytes(), &boundRevision); err != nil {
		t.Fatal(err)
	}
	if boundRevision.Outcome.ID != "outcome-bound-by-server" || boundRevision.Outcome.Scope != (Scope{OwnerID: "owner-1", WorkspaceID: "workspace-1"}) {
		t.Fatalf("browser controlled outcome scope: %#v", boundRevision.Outcome)
	}
	viewerRead := performRequest(engine, http.MethodGet, path, nil, "owner-1", "viewer", "")
	if viewerRead.Code != http.StatusOK {
		t.Fatalf("viewer read status = %d: %s", viewerRead.Code, viewerRead.Body.String())
	}
	otherOwner := performRequest(engine, http.MethodGet, path, nil, "other-owner", "owner", "")
	if otherOwner.Code != http.StatusNotFound {
		t.Fatalf("cross-owner read status = %d: %s", otherOwner.Code, otherOwner.Body.String())
	}
	unknownRole := performRequest(engine, http.MethodGet, path, nil, "owner-1", "unknown", "")
	if unknownRole.Code != http.StatusForbidden {
		t.Fatalf("unknown role status = %d", unknownRole.Code)
	}
	missingOwner := performRequest(engine, http.MethodGet, path, nil, "", "viewer", "")
	if missingOwner.Code != http.StatusUnauthorized {
		t.Fatalf("missing owner status = %d", missingOwner.Code)
	}

	evaluationObservations := []Observation{
		observation("obs-1", 12, testStart.Add(5*24*time.Hour)),
		observation("obs-2", 16, testStart.Add(15*24*time.Hour)),
	}
	evaluationObservations[0].Scope = Scope{OwnerID: "other-owner", WorkspaceID: "other-workspace"}
	evaluationBody := mustJSON(t, CreateEvaluationRequest{
		IdempotencyKey: "http-evaluation-1", OutcomeRevision: 1,
		Observations: evaluationObservations,
		AsOf:         testAsOf,
	})
	evaluation := performRequest(engine, http.MethodPost, path+"/evaluations", evaluationBody, "owner-1", "operator", "application/json")
	if evaluation.Code != http.StatusCreated || bytes.Contains(evaluation.Body.Bytes(), []byte(`"mayExecute":true`)) || bytes.Contains(evaluation.Body.Bytes(), []byte(`"mayChangePolicy":true`)) {
		t.Fatalf("unsafe or failed evaluation response = %d: %s", evaluation.Code, evaluation.Body.String())
	}

	unknownField := append(storeBody[:len(storeBody)-1], []byte(`,"unexpected":true}`)...)
	strict := performRequest(engine, http.MethodPut, path, unknownField, "owner-1", "owner", "application/json")
	if strict.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON field status = %d: %s", strict.Code, strict.Body.String())
	}
	wrongMedia := performRequest(engine, http.MethodPut, path, storeBody, "owner-1", "owner", "text/plain")
	if wrongMedia.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type status = %d", wrongMedia.Code)
	}
	trailing := append(append([]byte(nil), storeBody...), []byte(` {}`)...)
	trailingResponse := performRequest(engine, http.MethodPut, path, trailing, "owner-1", "owner", "application/json")
	if trailingResponse.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d", trailingResponse.Code)
	}
}

func TestHTTPResponsesDoNotExposeSecretsOrRepositoryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	engine := gin.New()
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	if err := RegisterRoutes(engine.Group("/api/v1"), NewHandler(service), testRouteGuards()); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/outcome-evaluations/workspaces/workspace-1/outcomes/outcome-1"
	request := validRequest().Outcome
	request.Statement = "api_key=super-secret-value"
	response := performRequest(engine, http.MethodPut, path, mustJSON(t, StoreOutcomeRequest{
		IdempotencyKey: "secret-test", ExpectedRevision: 0, Outcome: request,
	}), "owner-1", "owner", "application/json")
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "super-secret-value") || strings.Contains(strings.ToLower(response.Body.String()), "api_key") {
		t.Fatalf("secret-safe validation response = %d: %s", response.Code, response.Body.String())
	}

	leaking := &leakingRepository{Repository: NewMemoryRepository()}
	leakingEngine := gin.New()
	if err := RegisterRoutes(leakingEngine.Group("/api/v1"), NewHandler(newService(leaking, func() time.Time { return now })), testRouteGuards()); err != nil {
		t.Fatal(err)
	}
	leaked := performRequest(leakingEngine, http.MethodGet, path, nil, "owner-1", "owner", "")
	if leaked.Code != http.StatusInternalServerError || strings.Contains(leaked.Body.String(), "database-password") {
		t.Fatalf("repository error leaked = %d: %s", leaked.Code, leaked.Body.String())
	}
}

type leakingRepository struct {
	Repository
}

func (r *leakingRepository) GetOutcome(context.Context, string, string, string) (OutcomeRevision, error) {
	return OutcomeRevision{}, errors.New("postgres://admin:database-password@example.test/private")
}

func testRouteGuards() RouteGuards {
	authenticated := func(c *gin.Context) {
		subject := strings.TrimSpace(c.GetHeader("X-Test-Subject"))
		if subject == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Set(identity.ContextSubjectKey, subject)
		c.Next()
	}
	recognized := func(c *gin.Context) {
		switch c.GetHeader("X-Test-Role") {
		case "viewer", "operator", "owner":
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "recognized role required"})
		}
	}
	permission := func(roles ...string) gin.HandlerFunc {
		return func(c *gin.Context) {
			for _, role := range roles {
				if c.GetHeader("X-Test-Role") == role {
					c.Next()
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		}
	}
	return RouteGuards{
		AuthenticatedOwner: authenticated,
		RecognizedRole:     recognized,
		Read:               permission("viewer", "operator", "owner"),
		Write:              permission("operator", "owner"),
		Govern:             permission("owner"),
	}
}

func performRequest(engine *gin.Engine, method, path string, body []byte, subject, role, contentType string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if subject != "" {
		request.Header.Set("X-Test-Subject", subject)
	}
	if role != "" {
		request.Header.Set("X-Test-Role", role)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	engine.ServeHTTP(recorder, request)
	return recorder
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
