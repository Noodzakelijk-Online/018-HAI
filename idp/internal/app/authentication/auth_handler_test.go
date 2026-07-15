package authentication

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestPasswordResetDoesNotRevealServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(&middlewareAuthService{})
	router := gin.New()
	router.POST("/request-password-reset", handler.RequestPasswordReset)

	form := url.Values{"email": {"missing@example.com"}}
	request := httptest.NewRequest(http.MethodPost, "/request-password-reset", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "If recovery is available")
	require.NotContains(t, recorder.Body.String(), "not implemented")
}
