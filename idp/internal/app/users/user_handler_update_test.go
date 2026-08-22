package users

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-idp/internal/app/models"
	"automation-hub-idp/internal/app/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUpdatePreservesEmailForPasswordOnlyRequest(t *testing.T) {
	userID := uuid.New()
	service := &recordingUserService{user: &models.User{ID: userID, Email: "operator@example.com"}}
	recorder := performUserUpdate(t, userID, service, `{"password":"a-strong-password"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "a-strong-password", service.password)
	require.Nil(t, service.updatedUser)
	require.Contains(t, recorder.Body.String(), "operator@example.com")
}

func TestUpdateRejectsEmptyRequest(t *testing.T) {
	service := &recordingUserService{}
	recorder := performUserUpdate(t, uuid.New(), service, `{}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Nil(t, service.updatedUser)
	require.Empty(t, service.password)
}

func TestUpdateRejectsInvalidEmail(t *testing.T) {
	service := &recordingUserService{}
	recorder := performUserUpdate(t, uuid.New(), service, `{"email":"not-an-email"}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Nil(t, service.updatedUser)
	require.Empty(t, service.password)
}

func performUserUpdate(t *testing.T, userID uuid.UUID, service UserService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	})
	router.PATCH("/users", NewHandler(service).Update)

	request := httptest.NewRequest(http.MethodPatch, "/users", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type recordingUserService struct {
	user        *models.User
	updatedUser *models.User
	password    string
}

func (s *recordingUserService) CreateUser(models.User) (*models.User, error)     { return nil, nil }
func (s *recordingUserService) GetUserByID(uuid.UUID) (*models.User, error)      { return s.user, nil }
func (s *recordingUserService) GetUserByEmail(string) (*models.User, error)      { return nil, nil }
func (s *recordingUserService) GetUserByResetToken(string) (*models.User, error) { return nil, nil }
func (s *recordingUserService) UpdateUser(user models.User) (*models.User, error) {
	s.updatedUser = &user
	return &user, nil
}
func (s *recordingUserService) DeleteUser(uuid.UUID) error { return nil }
func (s *recordingUserService) GetAllUsers(*utils.Pagination) ([]*models.User, error) {
	return nil, nil
}
func (s *recordingUserService) UpdatePassword(_ uuid.UUID, password string) error {
	s.password = password
	return nil
}
