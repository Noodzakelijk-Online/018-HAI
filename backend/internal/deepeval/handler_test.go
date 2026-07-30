package deepeval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct{ Service }

func (s stubService) Status() Status                                { return Status{Provider: "DeepEval local grounding evaluator"} }
func (s stubService) Probe(_ context.Context) (*ProbeResult, error) { return nil, ErrNotConfigured }
func (s stubService) Run(_ context.Context) (*Result, error)        { return nil, ErrNotConfigured }

func TestHandlerDoesNotAcceptArbitraryEvaluationParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/run", NewHandler(stubService{}).Run)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/run?model=external&metric=all", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("run endpoint must ignore caller-selected model or metric: %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(`{"source":"untrusted"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("run endpoint must reject caller-supplied evaluation payloads: %d", recorder.Code)
	}
}
