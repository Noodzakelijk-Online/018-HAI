package ragflow

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
	err    error
	result *Response
	probe  *ProbeResult
}

func (f fakeService) Status() Status                                       { return Status{} }
func (f fakeService) Probe(context.Context) (*ProbeResult, error)          { return f.probe, f.err }
func (f fakeService) Retrieve(context.Context, Request) (*Response, error) { return f.result, f.err }

func TestHandlerDoesNotLeakLocalRAGFlowErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/retrieve", NewHandler(fakeService{err: errors.New("local-token secret internal error")}).Retrieve)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/retrieve", strings.NewReader(`{"query":"test"}`)))
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unexpected error response: %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsInvalidRequestSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/retrieve", NewHandler(fakeService{err: ErrInvalidRequest}).Retrieve)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/retrieve", strings.NewReader(`{"query":"test"}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "512") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsReadOnlyCandidateEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/retrieve", NewHandler(fakeService{result: &Response{
		Query:   "evidence",
		Results: []Result{{ChunkID: "chunk-a", DatasetID: "approved-dataset", Content: "Candidate evidence"}},
		Scope:   "candidate evidence only",
	}}).Retrieve)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/retrieve", strings.NewReader(`{"query":"evidence"}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "approved-dataset") || strings.Contains(response.Body.String(), "local-token") {
		t.Fatalf("unexpected success response: %d %s", response.Code, response.Body.String())
	}
}
