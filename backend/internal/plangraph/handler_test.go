package plangraph

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestHandlerRequiresOwnerAndRejectsUnknownJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newTestService()
	handler := NewHandler(service)

	unauthenticated := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(unauthenticated)
	context.Request = httptest.NewRequest(http.MethodGet, "/plans", nil)
	handler.List(context)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthenticated.Code)
	}

	invalid := httptest.NewRecorder()
	context, _ = gin.CreateTestContext(invalid)
	context.Set(identity.ContextSubjectKey, "owner-a")
	context.Request = httptest.NewRequest(http.MethodPost, "/plans/preview", strings.NewReader(`{"idempotencyKey":"x","unknown":true}`))
	context.Request.Header.Set("Content-Type", "application/json")
	handler.Preview(context)
	if invalid.Code != http.StatusBadRequest || strings.Contains(invalid.Body.String(), "unknown") {
		t.Fatalf("expected safe 400 response, got %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestHandlerMapsRevisionConflictWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newTestService()
	draft, err := service.Preview(t.Context(), "owner-a", previewRequest("handler-preview"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(identity.ContextSubjectKey, "owner-a")
	context.Params = gin.Params{{Key: "id", Value: draft.ID.String()}}
	context.Request = httptest.NewRequest(http.MethodPost, "/plans/"+draft.ID.String()+"/accept", strings.NewReader(`{"expectedRevision":99,"expectedDigest":"`+draft.Digest+`","acceptedBy":"spoofed"}`))
	handler.Accept(context)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "state conflict") {
		t.Fatalf("expected safe 409 response, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerUsesAuthenticatedOwnerAsMutationActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, repository := newTestService()
	handler := NewHandler(service)
	request := previewRequest("authenticated-actor")
	request.CreatedBy = "spoofed-actor"
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(identity.ContextSubjectKey, "owner-a")
	context.Request = httptest.NewRequest(http.MethodPost, "/plans/preview", strings.NewReader(string(body)))
	handler.Preview(context)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", recorder.Code, recorder.Body.String())
	}
	plans, err := repository.ListLatest(t.Context(), "owner-a")
	if err != nil || len(plans) != 1 {
		t.Fatalf("list persisted plan: plans=%v err=%v", plans, err)
	}
	if plans[0].CreatedBy != "owner-a" {
		t.Fatalf("createdBy = %q, want authenticated owner", plans[0].CreatedBy)
	}
}
