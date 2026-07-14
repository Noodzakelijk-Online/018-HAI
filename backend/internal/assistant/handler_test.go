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

func TestCommandHandlerRejectsGlobalCycleForNonOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewService(&fakeTaskEngine{}, &fakeAgentCycleRunner{}))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextRoleKey, "operator")
		c.Next()
	})
	engine.POST("/assistant/command", handler.Command)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assistant/command", strings.NewReader(`{"message":"Run maintenance","runCycle":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cycle status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}
