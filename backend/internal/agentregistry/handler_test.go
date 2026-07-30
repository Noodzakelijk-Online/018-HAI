package agentregistry

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

var handlerTestNow = time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

func TestHandlerRequiresAuthenticatedOwnerAndRejectsIdentitySpoofing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newAgentRegistryTestHandler(t, NewMemoryRepository())

	unauthenticated := gin.New()
	unauthenticated.GET("/agents", handler.List)
	response := performAgentRegistryRequest(unauthenticated, http.MethodGet, "/agents", "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, body=%s", response.Code, response.Body.String())
	}

	router := newAgentRegistryTestRouter(handler)
	body := validRegisterBody(t, "worker-1")
	body["ownerIdentity"] = "bob"
	response = performAgentRegistryJSON(t, router, http.MethodPost, "/agents", "alice", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("spoofed owner status = %d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "bob") {
		t.Fatalf("spoofed owner leaked in response: %s", response.Body.String())
	}

	valid := validRegisterBody(t, "worker-1")
	response = performAgentRegistryJSON(t, router, http.MethodPost, "/agents", "alice", valid)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%s", response.Code, response.Body.String())
	}
	var registered Agent
	decodeAgentRegistryResponse(t, response, &registered)
	if registered.OwnerIdentity != "alice" || registered.State != StateRegistered ||
		registered.Revision != 1 || registered.Reliability.Successes != 0 {
		t.Fatalf("service-controlled registration fields = %#v", registered)
	}
}

func TestHandlerAgentLifecycleUpdateAndOwnerIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newAgentRegistryTestHandler(t, NewMemoryRepository())
	router := newAgentRegistryTestRouter(handler)

	response := performAgentRegistryJSON(
		t, router, http.MethodPost, "/agents", "alice", validRegisterBody(t, "worker-1"),
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%s", response.Code, response.Body.String())
	}

	response = performAgentRegistryRequest(router, http.MethodGet, "/agents", "bob", "")
	if response.Code != http.StatusOK || response.Body.String() != `{"agents":[]}` {
		t.Fatalf("bob list = %d %s", response.Code, response.Body.String())
	}
	response = performAgentRegistryRequest(router, http.MethodGet, "/agents/worker-1", "bob", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner get status = %d, body=%s", response.Code, response.Body.String())
	}

	update := validUpdateBody(t, 1)
	update["name"] = "Updated worker"
	response = performAgentRegistryJSON(t, router, http.MethodPut, "/agents/worker-1", "alice", update)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", response.Code, response.Body.String())
	}
	var updated Agent
	decodeAgentRegistryResponse(t, response, &updated)
	if updated.Name != "Updated worker" || updated.Revision != 2 || updated.State != StateRegistered {
		t.Fatalf("updated agent = %#v", updated)
	}

	response = performAgentRegistryJSON(t, router, http.MethodPost, "/agents/worker-1/transitions", "alice", map[string]any{
		"expectedRevision": 2,
		"to":               StateEnabled,
		"reason":           "health and readiness verified",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("transition status = %d, body=%s", response.Code, response.Body.String())
	}
	var enabled Agent
	decodeAgentRegistryResponse(t, response, &enabled)
	if enabled.State != StateEnabled || enabled.Revision != 3 {
		t.Fatalf("enabled agent = %#v", enabled)
	}

	response = performAgentRegistryRequest(
		router, http.MethodGet, "/agents/worker-1/transitions", "alice", "",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("transition list status = %d, body=%s", response.Code, response.Body.String())
	}
	var transitions struct {
		Transitions []Transition `json:"transitions"`
	}
	decodeAgentRegistryResponse(t, response, &transitions)
	if len(transitions.Transitions) != 1 ||
		transitions.Transitions[0].From != StateRegistered ||
		transitions.Transitions[0].To != StateEnabled {
		t.Fatalf("transitions = %#v", transitions.Transitions)
	}

	stale := validUpdateBody(t, 2)
	response = performAgentRegistryJSON(t, router, http.MethodPut, "/agents/worker-1", "alice", stale)
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "revision") {
		t.Fatalf("stale update = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerAssignmentReadAndOutcomeRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newAgentRegistryTestHandler(t, NewMemoryRepository())
	router := newAgentRegistryTestRouter(handler)
	registerAndEnableAgent(t, router, "alice", "worker-1")

	assignmentRequest := map[string]any{
		"taskId": "task-1",
		"capabilities": []map[string]any{{
			"id":         "research",
			"minVersion": "1.0.0",
			"operations": []string{"search"},
		}},
		"compatibility": map[string]any{
			"runtimeAdapterId":   "local-script",
			"runtimeType":        "script",
			"minProtocolVersion": "1.0.0",
		},
		"requiredAuthority":  2,
		"requiredAutonomy":   2,
		"policyMaxAuthority": 4,
		"policyMaxAutonomy":  4,
		"requiredTools":      []string{"search"},
		"requiredData":       []string{"project"},
		"requiredFolders":    []string{"C:/HAI/project/work"},
		"allowedAgentTypes":  []AgentType{AgentTypeResearcher},
		"requireLocal":       true,
		"allowDegraded":      false,
	}
	response := performAgentRegistryJSON(t, router, http.MethodPost, "/assignments", "alice", assignmentRequest)
	if response.Code != http.StatusCreated {
		t.Fatalf("assignment status = %d, body=%s", response.Code, response.Body.String())
	}
	var assignment Assignment
	decodeAgentRegistryResponse(t, response, &assignment)
	if assignment.OwnerIdentity != "alice" || assignment.AgentID != "worker-1" ||
		assignment.GrantedAuthority != 2 {
		t.Fatalf("assignment = %#v", assignment)
	}

	response = performAgentRegistryRequest(
		router, http.MethodGet, "/assignments/"+assignment.ID, "alice", "",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("get assignment status = %d, body=%s", response.Code, response.Body.String())
	}
	var stored Assignment
	decodeAgentRegistryResponse(t, response, &stored)
	if stored.ID != assignment.ID || stored.RequestDigest == "" {
		t.Fatalf("stored assignment = %#v", stored)
	}
	response = performAgentRegistryRequest(
		router, http.MethodGet, "/assignments/"+assignment.ID, "bob", "",
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner assignment status = %d, body=%s", response.Code, response.Body.String())
	}

	response = performAgentRegistryJSON(t, router, http.MethodPost, "/assignments/"+assignment.ID+"/outcome", "alice", map[string]any{
		"expectedRevision": 3,
		"success":          true,
		"latency":          int64(1500 * time.Millisecond),
	})
	if response.Code != http.StatusOK {
		t.Fatalf("record outcome status = %d, body=%s", response.Code, response.Body.String())
	}
	var afterOutcome Agent
	decodeAgentRegistryResponse(t, response, &afterOutcome)
	if afterOutcome.Revision != 4 || afterOutcome.Reliability.Successes != 1 ||
		afterOutcome.Reliability.MeanLatencyMs != 1500 ||
		afterOutcome.Availability.ActiveAssignments != 0 {
		t.Fatalf("outcome evidence = %#v", afterOutcome.Reliability)
	}
}

func TestHandlerStrictBoundedJSONAndSafeErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newAgentRegistryTestHandler(t, NewMemoryRepository())
	router := newAgentRegistryTestRouter(handler)

	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "unknown field", body: `{"taskId":"task-1","ownerIdentity":"bob"}`, want: http.StatusBadRequest},
		{name: "trailing object", body: `{}` + `{}`, want: http.StatusBadRequest},
		{name: "empty body", body: ``, want: http.StatusBadRequest},
		{
			name: "oversized body",
			body: `{"taskId":"` + strings.Repeat("a", maxAgentRegistryRequestBytes) + `"}`,
			want: http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performAgentRegistryRequest(
				router, http.MethodPost, "/assignments", "alice", test.body,
			)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.want, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "bob") ||
				strings.Contains(response.Body.String(), strings.Repeat("a", 32)) {
				t.Fatalf("request material leaked in response: %s", response.Body.String())
			}
		})
	}

	response := performAgentRegistryJSON(t, router, http.MethodPost, "/assignments", "alice", map[string]any{
		"taskId":             "task-1",
		"capabilities":       []map[string]any{{"id": "missing"}},
		"compatibility":      map[string]any{},
		"requiredAuthority":  0,
		"requiredAutonomy":   0,
		"policyMaxAuthority": 0,
		"policyMaxAutonomy":  0,
	})
	if response.Code != http.StatusUnprocessableEntity ||
		response.Body.String() != `{"error":"no eligible agent is available"}` {
		t.Fatalf("no eligible mapping = %d %s", response.Code, response.Body.String())
	}

	failing := newAgentRegistryTestHandler(t, failingAgentRegistryRepository{
		err: errors.New("postgres password=top-secret unavailable"),
	})
	failingRouter := newAgentRegistryTestRouter(failing)
	response = performAgentRegistryRequest(failingRouter, http.MethodGet, "/agents", "alice", "")
	if response.Code != http.StatusInternalServerError ||
		!strings.Contains(response.Body.String(), `"errorId"`) ||
		strings.Contains(response.Body.String(), "top-secret") ||
		strings.Contains(response.Body.String(), "postgres") {
		t.Fatalf("internal error mapping = %d %s", response.Code, response.Body.String())
	}
}

func newAgentRegistryTestHandler(t *testing.T, repository Repository) *Handler {
	t.Helper()
	service, err := NewService(repository, func() time.Time { return handlerTestNow })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return NewHandler(service)
}

func newAgentRegistryTestRouter(handler *Handler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if owner := strings.TrimSpace(c.GetHeader("X-Test-Owner")); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
		c.Next()
	})
	router.POST("/agents", handler.Register)
	router.GET("/agents", handler.List)
	router.GET("/agents/:id", handler.Get)
	router.PUT("/agents/:id", handler.Update)
	router.POST("/agents/:id/transitions", handler.Transition)
	router.GET("/agents/:id/transitions", handler.ListTransitions)
	router.POST("/assignments", handler.Assign)
	router.GET("/assignments/:id", handler.GetAssignment)
	router.POST("/assignments/:id/outcome", handler.RecordAssignmentOutcome)
	return router
}

func validRegisterBody(t *testing.T, id string) map[string]any {
	t.Helper()
	return map[string]any{
		"id":               id,
		"name":             "Research worker",
		"type":             AgentTypeResearcher,
		"runtime":          map[string]any{"id": "local-script", "type": "script", "protocolVersion": "1.0.0"},
		"capabilities":     []map[string]any{{"id": "research", "version": "1.2.0", "operations": []string{"search"}}},
		"authorityCeiling": 4,
		"autonomyCeiling":  4,
		"toolAllowlist":    []string{"search"},
		"dataAllowlist":    []string{"project"},
		"folderAllowlist":  []string{"C:/HAI/project"},
		"health": map[string]any{
			"status":    HealthHealthy,
			"ready":     true,
			"checkedAt": handlerTestNow.Format(time.RFC3339),
			"freshFor":  int64(time.Hour),
		},
		"availability": map[string]any{"available": true, "activeAssignments": 0, "maxConcurrent": 2},
		"performance":  map[string]any{"estimatedCostEur": 0, "p95LatencyMs": 100, "locality": LocalityLocal},
	}
}

func validUpdateBody(t *testing.T, revision uint64) map[string]any {
	t.Helper()
	body := validRegisterBody(t, "ignored")
	delete(body, "id")
	body["expectedRevision"] = revision
	return body
}

func registerAndEnableAgent(t *testing.T, router *gin.Engine, owner, id string) {
	t.Helper()
	response := performAgentRegistryJSON(
		t, router, http.MethodPost, "/agents", owner, validRegisterBody(t, id),
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%s", response.Code, response.Body.String())
	}
	response = performAgentRegistryJSON(t, router, http.MethodPost, "/agents/"+id+"/transitions", owner, map[string]any{
		"expectedRevision": 1,
		"to":               StateEnabled,
		"reason":           "health and readiness verified",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body=%s", response.Code, response.Body.String())
	}
}

func performAgentRegistryJSON(
	t *testing.T,
	router http.Handler,
	method, path, owner string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return performAgentRegistryRequest(router, method, path, owner, string(encoded))
}

func performAgentRegistryRequest(
	router http.Handler,
	method, path, owner, body string,
) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if owner != "" {
		request.Header.Set("X-Test-Owner", owner)
	}
	router.ServeHTTP(response, request)
	return response
}

func decodeAgentRegistryResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}

type failingAgentRegistryRepository struct {
	err error
}

func (repository failingAgentRegistryRepository) Create(context.Context, Agent) (Agent, error) {
	return Agent{}, repository.err
}

func (repository failingAgentRegistryRepository) Get(context.Context, string, string) (Agent, error) {
	return Agent{}, repository.err
}

func (repository failingAgentRegistryRepository) List(context.Context, string) ([]Agent, error) {
	return nil, repository.err
}

func (repository failingAgentRegistryRepository) CompareAndSwap(context.Context, Agent, uint64) (Agent, error) {
	return Agent{}, repository.err
}

func (repository failingAgentRegistryRepository) Transition(context.Context, Agent, uint64, Transition) (Agent, error) {
	return Agent{}, repository.err
}

func (repository failingAgentRegistryRepository) ListTransitions(context.Context, string, string) ([]Transition, error) {
	return nil, repository.err
}

func (repository failingAgentRegistryRepository) CreateAssignment(context.Context, Assignment) (Agent, error) {
	return Agent{}, repository.err
}

func (repository failingAgentRegistryRepository) GetAssignment(context.Context, string, string) (Assignment, error) {
	return Assignment{}, repository.err
}

func (repository failingAgentRegistryRepository) RecordAssignmentOutcome(context.Context, AssignmentOutcome, Agent, uint64) (Agent, error) {
	return Agent{}, repository.err
}
