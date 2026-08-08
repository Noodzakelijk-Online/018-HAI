package proactivity

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesRefusesEveryMissingSecurityGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, err := NewAdvisoryAPI(NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	complete := testRouteGuards()
	tests := []struct {
		name   string
		mutate func(*RouteGuards)
	}{
		{"authenticated owner", func(value *RouteGuards) { value.AuthenticatedOwner = nil }},
		{"recognized role", func(value *RouteGuards) { value.RecognizedRole = nil }},
		{"read", func(value *RouteGuards) { value.Read = nil }},
		{"write", func(value *RouteGuards) { value.Write = nil }},
		{"govern", func(value *RouteGuards) { value.Govern = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guards := complete
			test.mutate(&guards)
			router := gin.New()
			if err := RegisterRoutes(router.Group("/api/v1"), handler, guards); err == nil {
				t.Fatalf("routes registered without %s guard", test.name)
			}
			if len(router.Routes()) != 0 {
				t.Fatalf("partial routes registered without %s guard", test.name)
			}
		})
	}
}

func TestAdvisoryConstructorRejectsTypedNilRepository(t *testing.T) {
	t.Parallel()
	var repository *MemoryRepository
	if handler, err := NewAdvisoryAPI(repository); err == nil || handler != nil {
		t.Fatalf("typed-nil repository constructor result: handler=%#v err=%v", handler, err)
	}
}

func TestDecisionHTTPResponseIsRedactedAndCannotGrantOrDeliverAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.August, 1, 18, 0, 0, 0, time.UTC)
	router := gin.New()
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	if err := RegisterRoutes(router.Group("/api/v1"), NewHandler(service), testRouteGuards()); err != nil {
		t.Fatal(err)
	}
	policy := marshalTestJSON(t, policyHTTPReq{IdempotencyKey: "security-policy", Policy: DefaultPreferences("owner-a")})
	if response := performAdvisoryRequest(router, http.MethodPut, "/api/v1/proactivity/policy", policy, "owner-a", "owner", "application/json"); response.Code != http.StatusCreated {
		t.Fatalf("policy status = %d: %s", response.Code, response.Body.String())
	}
	signal := testSignal("owner-a", "security-signal", "security-loop", now)
	signal.Title = "Token check sk-supersecret123"
	signal.Summary = "Authorization: Bearer private-delivery-token"
	signals := marshalTestJSON(t, signalsHTTPReq{IdempotencyKey: "security-signals", Signals: []OpenLoopSignal{signal}})
	if response := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/signals", signals, "owner-a", "operator", "application/json"); response.Code != http.StatusCreated {
		t.Fatalf("signals status = %d: %s", response.Code, response.Body.String())
	}
	evaluate := marshalTestJSON(t, EvaluateStoredRequest{IdempotencyKey: "security-evaluate", Now: now})
	response := performAdvisoryRequest(router, http.MethodPost, "/api/v1/proactivity/decisions/evaluate", evaluate, "owner-a", "operator", "application/json")
	if response.Code != http.StatusCreated {
		t.Fatalf("evaluate status = %d: %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{"sk-supersecret123", "private-delivery-token"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("decision response leaked %q: %s", secret, response.Body.String())
		}
	}
	var batch DecisionBatch
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Result.Decisions) != 1 {
		t.Fatalf("decision count = %d", len(batch.Result.Decisions))
	}
	assertNoAuthority(t, batch.Result.Decisions[0])

	for _, route := range router.Routes() {
		lower := strings.ToLower(route.Path)
		for _, forbidden := range []string{"deliver", "send", "execute", "approve", "authorize", "grant"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("unsafe proactivity route registered: %s %s", route.Method, route.Path)
			}
		}
	}
}
