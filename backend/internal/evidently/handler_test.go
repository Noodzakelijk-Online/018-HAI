package evidently

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct{ Service }

func (s stubService) Status() Status                                { return Status{Provider: "Evidently local runner"} }
func (s stubService) Probe(_ context.Context) (*ProbeResult, error) { return nil, ErrNotConfigured }
func (s stubService) Evaluate(_ context.Context, _ Request) (*Response, error) {
	return nil, ErrUnsafeFixture
}

func TestHandlerExplainsUnsafeFixtureWithoutEchoingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(stubService{})
	router.POST("/evaluate", handler.Evaluate)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(`{"fixtureKind":"synthetic","cases":[{"id":"case","input":"raw-fixture-token","output":"x"}]}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "redact") || strings.Contains(recorder.Body.String(), "raw-fixture-token") {
		t.Fatalf("unsafe fixture response should be safe and actionable: %d %s", recorder.Code, recorder.Body.String())
	}
}
