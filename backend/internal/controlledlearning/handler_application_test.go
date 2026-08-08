package controlledlearning

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestApplicationHandlerReadsOwnerLedgerWithoutExposingRollbackToken(t *testing.T) {
	service, application := appliedApplicationForHandlerTest(t)
	engine := applicationHandlerTestEngine(service, "robert")

	response := performApplicationHandlerRequest(
		engine,
		http.MethodGet,
		"/applications/"+application.ID,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("get application = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), application.ID) ||
		strings.Contains(response.Body.String(), "rollbackToken") ||
		strings.Contains(response.Body.String(), application.RollbackToken) {
		t.Fatalf("public application response = %s", response.Body.String())
	}

	crossOwner := applicationHandlerTestEngine(service, "other-owner")
	response = performApplicationHandlerRequest(
		crossOwner,
		http.MethodGet,
		"/applications/"+application.ID,
		"",
	)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "robert") {
		t.Fatalf("cross-owner get = %d: %s", response.Code, response.Body.String())
	}
}

func TestApplicationHandlerRollbackRequiresExplicitValidatedIntent(t *testing.T) {
	service, application := appliedApplicationForHandlerTest(t)
	engine := applicationHandlerTestEngine(service, "robert")
	path := "/applications/" + application.ID + "/rollback"

	for _, body := range []string{
		`{"idempotencyKey":"handler-rollback","expectedVersion":"1.1.0","humanConfirmed":false,"rationale":"Restore the prior version."}`,
		`{"expectedVersion":"1.1.0","humanConfirmed":true,"rationale":"Restore the prior version."}`,
		`{"idempotencyKey":"handler-rollback","expectedVersion":"1.1.0","humanConfirmed":true,"rationale":"Restore the prior version.","actorIdentity":"attacker"}`,
	} {
		response := performApplicationHandlerRequest(engine, http.MethodPost, path, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid rollback = %d: %s", response.Code, response.Body.String())
		}
	}

	validBody := `{"idempotencyKey":"handler-rollback","expectedVersion":"1.1.0","humanConfirmed":true,"rationale":"Restore the prior verified version."}`
	response := performApplicationHandlerRequest(engine, http.MethodPost, path, validBody)
	if response.Code != http.StatusOK {
		t.Fatalf("rollback = %d: %s", response.Code, response.Body.String())
	}
	var result ApplicationRecord
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode rollback: %v", err)
	}
	if result.Status != ApplicationRolledBack ||
		result.RestoredVersion != application.CurrentVersion ||
		result.RollbackToken != "" {
		t.Fatalf("rollback result = %#v", result)
	}

	replay := performApplicationHandlerRequest(engine, http.MethodPost, path, validBody)
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), application.ID) {
		t.Fatalf("rollback replay = %d: %s", replay.Code, replay.Body.String())
	}
}

func appliedApplicationForHandlerTest(t *testing.T) (*Service, ApplicationRecord) {
	t.Helper()
	service, _ := newTestServiceWithPromoter(t)
	outcome, err := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("handler-application-evidence"),
	)
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	proposal, err := service.Propose(context.Background(), proposalRequest(outcome.ID))
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	decision := approvedDecision(proposal)
	decision.IdempotencyKey = "handler-application-approval"
	result, err := service.DecideAndApply(context.Background(), decision)
	if err != nil {
		t.Fatalf("DecideAndApply: %v", err)
	}
	if result.Application == nil {
		t.Fatal("DecideAndApply did not return an application")
	}
	return service, *result.Application
}

func applicationHandlerTestEngine(service *Service, owner string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, owner)
		c.Next()
	})
	handler := NewHandler(service)
	engine.GET("/applications", handler.ListApplications)
	engine.GET("/applications/:id", handler.GetApplication)
	engine.GET("/applications/:id/events", handler.ListApplicationEvents)
	engine.POST("/applications/:id/rollback", handler.RollbackApplication)
	return engine
}

func performApplicationHandlerRequest(
	engine *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	engine.ServeHTTP(response, request)
	return response
}
