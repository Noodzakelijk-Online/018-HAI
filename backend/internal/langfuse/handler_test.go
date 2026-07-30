package langfuse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct{ Service }

func (s stubService) Status() Status                              { return Status{Provider: "Langfuse self-hosted observability"} }
func (s stubService) Probe(context.Context) (*ProbeResult, error) { return nil, ErrNotConfigured }
func (s stubService) ExportOperationalSnapshot(context.Context) (*ExportResult, error) {
	return nil, ErrNotConfigured
}

func TestHandlerRejectsCallerProvidedTraceData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/export", NewHandler(stubService{}).ExportOperationalSnapshot)
	request := httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(`{"prompt":"secret"}`))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("response=%d want=%d", recorder.Code, http.StatusBadRequest)
	}
}
