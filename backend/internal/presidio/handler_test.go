package presidio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeService struct{ err error }

func (f fakeService) Status() Status                                      { return Status{} }
func (f fakeService) Analyze(context.Context, Request) (*Response, error) { return nil, f.err }

func TestHandlerDoesNotLeakAnalyzerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/analyze", NewHandler(fakeService{err: errors.New("internal source secret")}).Analyze)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(`{"text":"test"}`)))
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
