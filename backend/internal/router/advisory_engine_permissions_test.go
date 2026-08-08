package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/ambientmonitor"
	"automation-hub-backend/internal/outcomeevaluation"
	"automation-hub-backend/internal/proactivity"
	"automation-hub-backend/internal/resilience"

	"github.com/gin-gonic/gin"
)

func TestAdvisoryEngineRoutesFailClosedAtRouterBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	proactivityHandler, err := proactivity.NewAdvisoryAPI(proactivity.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	if err := initializeProactivityRoutes(engine.Group("/api/v1"), proactivityHandler); err != nil {
		t.Fatal(err)
	}
	outcomeService := outcomeevaluation.NewService(outcomeevaluation.NewMemoryRepository())
	if err := initializeOutcomeEvaluationRoutes(
		engine.Group("/api/v1"),
		outcomeevaluation.NewHandler(outcomeService),
	); err != nil {
		t.Fatal(err)
	}
	monitorService := ambientmonitor.NewService(
		ambientmonitor.NewMemoryRepository(), ambientMonitorCollectorStub{}, nil,
	)
	if err := initializeAmbientMonitorRoutes(
		engine.Group("/api/v1"), ambientmonitor.NewHandler(monitorService, outcomeService),
	); err != nil {
		t.Fatal(err)
	}
	resilienceHandler, err := resilience.NewAdvisoryAPI(resilience.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	if err := initializeResilienceRoutes(engine.Group("/api/v1"), resilienceHandler); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		method string
		path   string
		role   string
		want   int
	}{
		{name: "viewer reads proactive signals", method: http.MethodGet, path: "/api/v1/proactivity/signals", role: "viewer", want: http.StatusOK},
		{name: "viewer reads owner attention inbox", method: http.MethodGet, path: "/api/v1/proactivity/inbox", role: "viewer", want: http.StatusOK},
		{name: "viewer reads attention feedback", method: http.MethodGet, path: "/api/v1/proactivity/feedback", role: "viewer", want: http.StatusOK},
		{name: "unknown role cannot read proactive signals", method: http.MethodGet, path: "/api/v1/proactivity/signals", role: "future-role", want: http.StatusForbidden},
		{name: "viewer cannot write proactive signals", method: http.MethodPost, path: "/api/v1/proactivity/signals", role: "viewer", want: http.StatusForbidden},
		{name: "viewer cannot record attention feedback", method: http.MethodPost, path: "/api/v1/proactivity/feedback", role: "viewer", want: http.StatusForbidden},
		{name: "operator reaches attention feedback validation", method: http.MethodPost, path: "/api/v1/proactivity/feedback", role: "operator", want: http.StatusBadRequest},
		{name: "viewer cannot govern outcomes", method: http.MethodPut, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1", role: "viewer", want: http.StatusForbidden},
		{name: "unknown role cannot inspect outcomes", method: http.MethodGet, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1", role: "future-role", want: http.StatusForbidden},
		{name: "viewer reaches monitor read scope", method: http.MethodGet, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1/monitor", role: "viewer", want: http.StatusNotFound},
		{name: "unknown role cannot inspect monitor", method: http.MethodGet, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1/monitor", role: "future-role", want: http.StatusForbidden},
		{name: "viewer cannot run due monitors", method: http.MethodPost, path: "/api/v1/outcome-evaluations/workspaces/work-1/monitors/run-due", role: "viewer", want: http.StatusForbidden},
		{name: "operator reaches run due validation", method: http.MethodPost, path: "/api/v1/outcome-evaluations/workspaces/work-1/monitors/run-due", role: "operator", want: http.StatusBadRequest},
		{name: "operator cannot configure monitor", method: http.MethodPut, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1/monitor", role: "operator", want: http.StatusForbidden},
		{name: "owner reaches monitor configuration validation", method: http.MethodPut, path: "/api/v1/outcome-evaluations/workspaces/work-1/outcomes/outcome-1/monitor", role: "owner", want: http.StatusBadRequest},
		{name: "viewer reads resilience status", method: http.MethodGet, path: "/api/v1/resilience/workspaces/work-1/status", role: "viewer", want: http.StatusOK},
		{name: "viewer cannot register resilience work", method: http.MethodPost, path: "/api/v1/resilience/workspaces/work-1/work-registrations", role: "viewer", want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestAdvisoryEngineRouterRegistrationRejectsNilHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := gin.New().Group("/api/v1")
	if err := initializeProactivityRoutes(group, nil); err == nil {
		t.Fatal("expected proactivity registration failure")
	}
	if err := initializeOutcomeEvaluationRoutes(group, nil); err == nil {
		t.Fatal("expected outcome registration failure")
	}
	if err := initializeAmbientMonitorRoutes(group, nil); err == nil {
		t.Fatal("expected ambient monitor registration failure")
	}
	if err := initializeResilienceRoutes(group, nil); err == nil {
		t.Fatal("expected resilience registration failure")
	}
}

type ambientMonitorCollectorStub struct{}

func (ambientMonitorCollectorStub) Collect(context.Context, ambientmonitor.MonitorTarget) (ambientmonitor.CollectedObservation, error) {
	return ambientmonitor.CollectedObservation{}, ambientmonitor.ErrCollectorUnavailable
}
