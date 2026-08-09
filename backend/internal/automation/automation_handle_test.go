package automation

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAutomationRoutesRequireVerifiedOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	unauthenticated := gin.New()
	unauthenticatedRoutes := unauthenticated.Group("/automation")
	unauthenticatedRoutes.Use(RequireAuthenticatedOperator())
	unauthenticatedRoutes.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	unauthenticatedRecorder := httptest.NewRecorder()
	unauthenticated.ServeHTTP(unauthenticatedRecorder, httptest.NewRequest(http.MethodGet, "/automation/", nil))
	if unauthenticatedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated automation route status = %d, want %d: %s", unauthenticatedRecorder.Code, http.StatusUnauthorized, unauthenticatedRecorder.Body.String())
	}

	authenticated := gin.New()
	authenticated.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "robert")
		c.Next()
	})
	authenticatedRoutes := authenticated.Group("/automation")
	authenticatedRoutes.Use(RequireAuthenticatedOperator())
	authenticatedRoutes.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	authenticatedRecorder := httptest.NewRecorder()
	authenticated.ServeHTTP(authenticatedRecorder, httptest.NewRequest(http.MethodGet, "/automation/", nil))
	if authenticatedRecorder.Code != http.StatusNoContent {
		t.Fatalf("authenticated automation route status = %d, want %d: %s", authenticatedRecorder.Code, http.StatusNoContent, authenticatedRecorder.Body.String())
	}
}

func TestGetAutomationReturnsNotFoundForDeletedRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)

	existingID := uuid.New()
	handler := NewHandler(newTestService(
		newFakeAutomationRepo(&models.Automation{ID: existingID}),
		events.Publisher{},
	))
	router := gin.New()
	router.GET("/automation/:id", handler.GetByID)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/automation/"+uuid.NewString(), nil),
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing automation status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}
