package deepteam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct{ Service }

func (s stubService) Status() Status                                { return Status{Provider: "DeepTeam local safety runner"} }
func (s stubService) Probe(_ context.Context) (*ProbeResult, error) { return nil, ErrNotConfigured }
func (s stubService) Run(_ context.Context) (*Result, error)        { return nil, ErrNotConfigured }

func TestHandlerDoesNotAcceptArbitrarySafetySuiteParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/run", NewHandler(stubService{}).Run)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/run?model=external&attack=anything", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("run endpoint must ignore caller-selected model or attack: %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(`{"target":"untrusted"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("run endpoint must reject caller-supplied run payloads: %d", recorder.Code)
	}
}

func TestHandlerRejectsChunkedCallerBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/run", NewHandler(stubService{}).Run)
	request := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(`{"model":"must-not-be-accepted"}`))
	request.ContentLength = -1
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("chunked body response=%d want=%d", response.Code, http.StatusBadRequest)
	}
}
