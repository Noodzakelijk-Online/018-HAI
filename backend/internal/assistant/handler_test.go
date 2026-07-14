package assistant

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestCommandHandlerUsesVerifiedOwnerForSelectedPursuit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := &fakePursuitCommandRouter{}
	handler := NewHandler(NewService(&fakeTaskEngine{}, nil, router))
	pursuitID := uuid.New()

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	engine.POST("/assistant/command", handler.Command)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assistant/command", strings.NewReader(`{"message":"Plan the evidence review","pursuitId":"`+pursuitID.String()+`"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("command status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if router.lastOwner != "alice" {
		t.Fatalf("selected pursuit owner = %q, want alice", router.lastOwner)
	}
}

func TestCommandHandlerRejectsUnauthenticatedCycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewService(&fakeTaskEngine{}, &fakeAgentCycleRunner{}))
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Next() })
	engine.POST("/assistant/command", handler.Command)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assistant/command", strings.NewReader(`{"message":"Run maintenance","runCycle":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cycle status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
}

func TestCommandHandlerAllowsAuthenticatedPersonalCycleForOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cycle := &fakeAgentCycleRunner{}
	handler := NewHandler(NewService(&fakeTaskEngine{}, cycle))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, "operator")
		c.Next()
	})
	engine.POST("/assistant/command", handler.Command)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assistant/command", strings.NewReader(`{"message":"Refresh my operating brief","runCycle":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("personal cycle status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if cycle.lastRequest.OwnerIdentity != "alice" {
		t.Fatalf("personal cycle owner = %q, want alice", cycle.lastRequest.OwnerIdentity)
	}
}
