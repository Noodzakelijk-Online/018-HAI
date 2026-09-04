package autonomy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
)

type failingHandlerService struct {
	overviewErr error
	stressErr   error
}

func (s failingHandlerService) Overview() (*Overview, error) {
	return nil, s.overviewErr
}

func (s failingHandlerService) RunStressSuite() (*models.AutonomyStressRun, []StressCaseResult, error) {
	return nil, nil, s.stressErr
}

func TestHandlerDoesNotExposeUnexpectedServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(failingHandlerService{overviewErr: errors.New(`postgres password=not-for-http at C:\\private`), stressErr: errors.New(`token=not-for-http`)})
	router := gin.New()
	router.GET("/overview", handler.Overview)
	router.POST("/stress", handler.Stress)

	for _, test := range []struct {
		name      string
		method    string
		path      string
		wantError string
	}{
		{name: "overview", method: http.MethodGet, path: "/overview", wantError: "autonomy overview is unavailable"},
		{name: "stress", method: http.MethodPost, path: "/stress", wantError: "autonomy stress suite could not be completed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
			}
			for _, forbidden := range []string{"password", "token", "not-for-http", "C:\\\\private"} {
				if strings.Contains(strings.ToLower(recorder.Body.String()), strings.ToLower(forbidden)) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
			if !strings.Contains(recorder.Body.String(), test.wantError) {
				t.Fatalf("response lacks stable error %q: %s", test.wantError, recorder.Body.String())
			}
		})
	}
}
