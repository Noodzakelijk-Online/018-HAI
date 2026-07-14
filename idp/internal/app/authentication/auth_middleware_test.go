package authentication

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-idp/internal/app/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAuthMiddlewareRefreshesBeforeResolvingExpiredAccessIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	service := &middlewareAuthService{
		valid:         false,
		refreshResult: &dto.TokenDetails{AccessToken: "refreshed-access", AtExpires: time.Now().Add(time.Hour).Unix()},
		userID:        userID,
	}
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/protected", AuthMiddleware(handler), func(c *gin.Context) {
		value, ok := c.Get("userID")
		if !ok || value != userID {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access_token", Value: "expired-access"})
	request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "valid-refresh"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if service.refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", service.refreshCalls)
	}
	if service.lastIdentityToken != "refreshed-access" {
		t.Fatalf("identity token = %q, want refreshed access token", service.lastIdentityToken)
	}
	if got := recorder.Header().Get("Set-Cookie"); got == "" {
		t.Fatal("expected refreshed access-token cookie")
	}
}

type middlewareAuthService struct {
	valid             bool
	refreshResult     *dto.TokenDetails
	userID            uuid.UUID
	refreshCalls      int
	lastIdentityToken string
}

func (s *middlewareAuthService) Register(dto.UserDTO) (*dto.UserResponse, error) {
	return nil, errors.New("not implemented")
}
func (s *middlewareAuthService) Login(string, string) (*dto.TokenDetails, error) {
	return nil, errors.New("not implemented")
}
func (s *middlewareAuthService) Logout(string) error { return errors.New("not implemented") }
func (s *middlewareAuthService) RefreshToken(string) (*dto.TokenDetails, error) {
	s.refreshCalls++
	return s.refreshResult, nil
}
func (s *middlewareAuthService) IsUserAuthenticated(string) (bool, error) { return s.valid, nil }
func (s *middlewareAuthService) RequestPasswordReset(string) (string, time.Time, error) {
	return "", time.Time{}, errors.New("not implemented")
}
func (s *middlewareAuthService) ConfirmPasswordReset(string, string) error {
	return errors.New("not implemented")
}
func (s *middlewareAuthService) ChangePassword(string, string) error {
	return errors.New("not implemented")
}
func (s *middlewareAuthService) GetIdFromToken(token string) (uuid.UUID, error) {
	s.lastIdentityToken = token
	if token != "refreshed-access" {
		return uuid.Nil, errors.New("expired access token was used")
	}
	return s.userID, nil
}
