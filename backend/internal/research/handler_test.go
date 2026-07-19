package research

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeService struct {
	status Status
	result *Response
	err    error
}

func (f fakeService) Status() Status                                     { return f.status }
func (f fakeService) Search(context.Context, Request) (*Response, error) { return f.result, f.err }

func TestHandlerDoesNotLeakLocalResearchErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/search", NewHandler(fakeService{err: errors.New("secret internal error")}).Search)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"test"}`)))
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unexpected error response: %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsNotConfiguredSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/search", NewHandler(fakeService{err: ErrNotConfigured}).Search)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"test"}`)))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "not configured") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
