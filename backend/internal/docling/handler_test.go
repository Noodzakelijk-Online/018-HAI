package docling

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type handlerStub struct{ err error }

func (s handlerStub) Status() Status { return Status{} }
func (s handlerStub) Probe(context.Context) (*ProbeResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &ProbeResult{Reachable: true}, nil
}
func (s handlerStub) Extract(context.Context, string) ([]Document, error) { return nil, nil }

func TestProbeMapsConfigurationAndRunnerFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "configuration", err: ErrNotConfigured, want: http.StatusServiceUnavailable},
		{name: "unavailable", err: errors.New("runner unavailable"), want: http.StatusBadGateway},
		{name: "healthy", want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/probe", NewHandler(handlerStub{err: test.err}).Probe)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/probe", nil))
			if response.Code != test.want {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
		})
	}
}
