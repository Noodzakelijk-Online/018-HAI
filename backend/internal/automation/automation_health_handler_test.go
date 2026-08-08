package automation

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestLaunchHandlerForwardsMandateReferenceButOwnsIdentityAndApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	automationID := uuid.New()
	mandateID := uuid.NewString()
	repository := newFakeAutomationRepo(&models.Automation{
		ID:                 automationID,
		Name:               "Owner-scoped launch",
		URLPath:            "owner-scoped-launch",
		LaunchType:         "api",
		LaunchTarget:       "GET " + target.URL,
		ExpectedHTTPStatus: http.StatusNoContent,
	})
	authorizer := &recordingExecutionAuthorizer{}
	handler := NewHandler(&service{
		repo:          repository,
		executionAuth: authorizer,
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.POST("/automations/:id/launch", handler.Launch)
	body := []byte(`{
		"ownerIdentity":"mallory",
		"actorIdentity":"mallory",
		"approvalSourceId":"forged",
		"approvalBindingDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task":"Read current state",
		"projectKey":"018-hai",
		"mandateId":"` + mandateID + `"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/automations/"+automationID.String()+"/launch",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if authorizer.calls.Load() != 1 {
		t.Fatalf("authorization calls = %d, want 1", authorizer.calls.Load())
	}
	if authorizer.request.OwnerIdentity != "alice" ||
		authorizer.request.ActorIdentity != "alice" ||
		authorizer.request.ApprovalSourceID != "" ||
		authorizer.request.ApprovalBindingDigest != "" ||
		authorizer.request.MandateID != mandateID {
		t.Fatalf("authorization request = %#v", authorizer.request)
	}
}

func TestLaunchHandlerRejectsMalformedOptionalLaunchContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(&service{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.POST("/automations/:id/launch", handler.Launch)
	request := httptest.NewRequest(
		http.MethodPost,
		"/automations/"+uuid.NewString()+"/launch",
		bytes.NewBufferString(`{"mandateId":`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
