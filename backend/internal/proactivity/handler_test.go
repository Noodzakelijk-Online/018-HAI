package proactivity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesExposesGuardedOwnerScopedAdvisoryAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.August, 1, 16, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	router := gin.New()
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(service), testRouteGuards()); err != nil {
		t.Fatal(err)
	}

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/proactivity/policy",
		"PUT /api/v1/proactivity/policy",
		"GET /api/v1/proactivity/signals",
		"POST /api/v1/proactivity/signals",
		"GET /api/v1/proactivity/decisions",
		"POST /api/v1/proactivity/decisions/evaluate",
		"GET /api/v1/proactivity/inbox",
		"GET /api/v1/proactivity/feedback",
		"POST /api/v1/proactivity/feedback",
	} {
		if !registered[route] {
			t.Fatalf("route not registered: %s", route)
		}
	}

	policyBody := marshalTestJSON(t, policyHTTPReq{IdempotencyKey: "policy-http", Policy: DefaultPreferences("owner-a")})
	ownerPut := performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", policyBody, "owner-a", "owner", "application/json")
	if ownerPut.Code != http.StatusCreated {
		t.Fatalf("owner policy status = %d: %s", ownerPut.Code, ownerPut.Body.String())
	}
	operatorPut := performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", policyBody, "owner-a", "operator", "application/json")
	if operatorPut.Code != http.StatusForbidden {
		t.Fatalf("operator policy status = %d", operatorPut.Code)
	}
	viewerGet := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/policy", nil, "owner-a", "viewer", "")
	if viewerGet.Code != http.StatusOK || !bytes.Contains(viewerGet.Body.Bytes(), []byte("owner-a")) {
		t.Fatalf("viewer policy status = %d: %s", viewerGet.Code, viewerGet.Body.String())
	}
	otherOwnerGet := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/policy", nil, "owner-b", "owner", "")
	if otherOwnerGet.Code != http.StatusNotFound {
		t.Fatalf("cross-owner policy status = %d: %s", otherOwnerGet.Code, otherOwnerGet.Body.String())
	}
	missingOwner := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/signals", nil, "", "viewer", "")
	if missingOwner.Code != http.StatusUnauthorized {
		t.Fatalf("missing owner status = %d", missingOwner.Code)
	}
	unknownRole := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/signals", nil, "owner-a", "unknown", "")
	if unknownRole.Code != http.StatusForbidden {
		t.Fatalf("unknown role status = %d", unknownRole.Code)
	}

	signal := testSignal("owner-a", "signal-http", "loop-http", now)
	signalBody := marshalTestJSON(t, signalsHTTPReq{IdempotencyKey: "signals-http", Signals: []OpenLoopSignal{signal}})
	operatorSignal := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/signals", signalBody, "owner-a", "operator", "application/json")
	if operatorSignal.Code != http.StatusCreated {
		t.Fatalf("operator signal status = %d: %s", operatorSignal.Code, operatorSignal.Body.String())
	}
	evaluateBody := marshalTestJSON(t, EvaluateStoredRequest{IdempotencyKey: "evaluate-http", Now: now})
	viewerEvaluate := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/decisions/evaluate", evaluateBody, "owner-a", "viewer", "application/json")
	if viewerEvaluate.Code != http.StatusForbidden {
		t.Fatalf("viewer evaluate status = %d", viewerEvaluate.Code)
	}
	operatorEvaluate := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/decisions/evaluate", evaluateBody, "owner-a", "operator", "application/json")
	if operatorEvaluate.Code != http.StatusCreated {
		t.Fatalf("operator evaluate status = %d: %s", operatorEvaluate.Code, operatorEvaluate.Body.String())
	}
	var batch DecisionBatch
	if err := json.Unmarshal(operatorEvaluate.Body.Bytes(), &batch); err != nil || len(batch.Result.Decisions) != 1 {
		t.Fatalf("decode evaluated decision: batch=%#v err=%v", batch, err)
	}
	decision := batch.Result.Decisions[0]
	feedbackBody := marshalTestJSON(t, FeedbackRequest{
		IdempotencyKey: "feedback-http", SignalID: decision.SignalID,
		OpenLoopKey: decision.OpenLoopKey, SignalDigest: decision.SignalDigest,
		Action: FeedbackSnooze, Reason: "Show this again tomorrow.", SnoozedUntil: timePointer(now.Add(24 * time.Hour)),
	})
	feedbackResponse := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/feedback", feedbackBody, "owner-a", "operator", "application/json")
	if feedbackResponse.Code != http.StatusCreated || !bytes.Contains(feedbackResponse.Body.Bytes(), []byte(`"canExecute":false`)) {
		t.Fatalf("feedback status = %d: %s", feedbackResponse.Code, feedbackResponse.Body.String())
	}
	viewerFeedback := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/feedback", nil, "owner-a", "viewer", "")
	if viewerFeedback.Code != http.StatusOK || !bytes.Contains(viewerFeedback.Body.Bytes(), []byte(`"action":"snooze"`)) {
		t.Fatalf("feedback list status = %d: %s", viewerFeedback.Code, viewerFeedback.Body.String())
	}
	viewerInbox := performAdvisoryRequest(router, http.MethodGet, "/api/v1/proactivity/inbox", nil, "owner-a", "viewer", "")
	if viewerInbox.Code != http.StatusOK || !bytes.Contains(viewerInbox.Body.Bytes(), []byte(`"snoozed":1`)) ||
		!bytes.Contains(viewerInbox.Body.Bytes(), []byte(`"canExecute":false`)) {
		t.Fatalf("inbox status = %d: %s", viewerInbox.Code, viewerInbox.Body.String())
	}
}

func TestHandlerRequiresStrictJSONAndAuthenticatedBodyOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.August, 1, 17, 0, 0, 0, time.UTC)
	router := gin.New()
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(newService(NewMemoryRepository(), func() time.Time { return now })), testRouteGuards()); err != nil {
		t.Fatal(err)
	}

	unknown := []byte(`{"idempotencyKey":"policy-unknown","policy":{},"unexpected":true}`)
	response := performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", unknown, "owner-a", "owner", "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d: %s", response.Code, response.Body.String())
	}
	valid := marshalTestJSON(t, policyHTTPReq{IdempotencyKey: "policy-strict", Policy: DefaultPreferences("owner-a")})
	response = performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", append(valid, []byte(` {}`)...), "owner-a", "owner", "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d: %s", response.Code, response.Body.String())
	}
	response = performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", valid, "owner-a", "owner", "text/plain")
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status = %d: %s", response.Code, response.Body.String())
	}
	crossOwner := marshalTestJSON(t, policyHTTPReq{IdempotencyKey: "policy-cross", Policy: DefaultPreferences("owner-b")})
	response = performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", crossOwner, "owner-a", "owner", "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("cross-owner body status = %d: %s", response.Code, response.Body.String())
	}
}

func testRouteGuards() RouteGuards {
	authenticated := func(c *gin.Context) {
		subject := c.GetHeader("X-Test-Subject")
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
	permission := func(allowed ...string) gin.HandlerFunc {
		return func(c *gin.Context) {
			role := c.GetHeader("X-Test-Role")
			for _, candidate := range allowed {
				if role == candidate {
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

func performAdvisoryRequest(router *gin.Engine, method, path string, body []byte, subject, role, contentType string) *httptest.ResponseRecorder {
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
	router.ServeHTTP(recorder, request)
	return recorder
}

func marshalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
