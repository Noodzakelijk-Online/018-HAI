package guardrails

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct{ Service }

func (s stubService) Status() Status                                { return Status{Provider: "Guardrails AI local runner"} }
func (s stubService) Probe(_ context.Context) (*ProbeResult, error) { return nil, ErrNotConfigured }
func (s stubService) Validate(_ context.Context, _ Request) (*Response, error) {
	return nil, ErrUnsafeProposal
}

func TestHandlerExplainsUnsafeProposalWithoutEchoingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/validate", NewHandler(stubService{}).Validate)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{"schema":"action_proposal","proposal":"raw-proposal-token"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "redact") || strings.Contains(recorder.Body.String(), "raw-proposal-token") {
		t.Fatalf("unsafe proposal response should be safe and actionable: %d %s", recorder.Code, recorder.Body.String())
	}
}
