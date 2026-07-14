package ambient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAcceptHandlerRejectsAnotherOwnersPursuitOpportunity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pursuitID := uuid.New()
	opportunity := &models.AmbientOpportunity{
		ID:         uuid.New(),
		Status:     StatusProposed,
		NeedKey:    "safety",
		Title:      "Private pursuit decision",
		Rationale:  "Requires review.",
		NextAction: "Review private evidence.",
		SourceType: "pursuit_decision",
		SourceID:   pursuitID.String(),
	}
	workflowSpy := &ambientWorkflowSpy{}
	pursuitSpy := &ambientPursuitSpy{owners: map[uuid.UUID]string{pursuitID: "bob"}}
	handler := NewHandler(NewServiceWithPursuits(&ambientRepositoryStub{opportunity: opportunity}, workflowSpy, nil, pursuitSpy))

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	engine.POST("/ambient/:id/accept", handler.Accept)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ambient/"+opportunity.ID.String()+"/accept", strings.NewReader(`{"note":"approve"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("accept status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if len(workflowSpy.intakeRequests) != 0 {
		t.Fatalf("cross-owner ambient acceptance created workflow work: %#v", workflowSpy.intakeRequests)
	}
}

func TestAcceptHandlerUsesVerifiedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opportunity := &models.AmbientOpportunity{
		ID:         uuid.New(),
		Status:     StatusProposed,
		NeedKey:    "growth",
		Title:      "Prepare an internal draft",
		Rationale:  "Low-risk preparation.",
		NextAction: "Prepare the internal draft.",
		SourceType: "workflow",
		SourceID:   "shared-workflow",
	}
	workflowSpy := &ambientWorkflowSpy{}
	handler := NewHandler(NewService(&ambientRepositoryStub{opportunity: opportunity}, workflowSpy, nil))

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	engine.POST("/ambient/:id/accept", handler.Accept)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ambient/"+opportunity.ID.String()+"/accept", strings.NewReader(`{"note":"approve","actor":"untrusted"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("accept status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(workflowSpy.intakeRequests) != 1 || workflowSpy.intakeRequests[0].Actor != "alice" {
		t.Fatalf("workflow request actor = %#v, want verified alice", workflowSpy.intakeRequests)
	}
}
