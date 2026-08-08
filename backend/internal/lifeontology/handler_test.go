package lifeontology

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesRequiresEveryGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	err := RegisterRoutes(router.Group("/api/v1"), NewHandler(NewService(nil, nil)), RouteGuards{})
	if err == nil {
		t.Fatal("unguarded life ontology routes were accepted")
	}
}

func TestLifeOntologyRoutesDeriveOwnerAndRejectSpoofedField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(nil, func() time.Time { return fixedNow() })
	router := testLifeOntologyRouter(t, service, "alice")
	body := entityRequest(EntityGoal, "Improve health")
	payload := map[string]any{
		"ownerIdentity": "mallory", "type": body.Type, "domain": body.Domain,
		"name": body.Name, "status": body.Status, "priority": body.Priority,
		"validFrom": body.ValidFrom, "observedAt": body.ObservedAt,
		"confidence": body.Confidence, "verificationStatus": body.VerificationStatus,
		"provenance": body.Provenance, "sensitivity": body.Sensitivity,
	}
	encoded, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/life-ontology/entities", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("spoofed owner field status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	delete(payload, "ownerIdentity")
	encoded, _ = json.Marshal(payload)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/life-ontology/entities", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result EntityWriteResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Entity.OwnerIdentity != "alice" {
		t.Fatalf("owner=%q, want authenticated owner", result.Entity.OwnerIdentity)
	}

	bob := testLifeOntologyRouter(t, service, "bob")
	request = httptest.NewRequest(http.MethodGet, "/api/v1/life-ontology/entities/"+result.Entity.ID, nil)
	recorder = httptest.NewRecorder()
	bob.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-owner read status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestContextSuggestionEndpointRemainsAdvisory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(nil, func() time.Time { return fixedNow() })
	request := entityRequest(EntityRisk, "Upcoming deadline")
	request.OwnerIdentity = "alice"
	if _, err := service.RecordEntity(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	router := testLifeOntologyRouter(t, service, "alice")
	body := []byte(`{"asOf":"2026-07-31T12:00:00Z","allowLocalOnly":true,"limit":10}`)
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/life-ontology/context/suggest", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httpRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result ContextSuggestionResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		t.Fatalf("context endpoint crossed authority boundary: %#v", result)
	}
}

func TestContactReviewEndpointCreatesOwnerScopedDecisionWithoutAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(nil, func() time.Time { return fixedNow() })
	candidate := mustContactCandidate(t, service, "Candidate Joyce", "handler-contact")
	router := testLifeOntologyRouter(t, service, "owner-1")
	payload := map[string]any{
		"action": "promote", "reason": "Robert confirmed this contact identity",
		"idempotencyKey": "handler-contact-review",
	}
	encoded, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/life-ontology/contact-candidates/"+candidate.ID+"/decisions", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("contact review status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result ContactReviewDecisionResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Decision.OwnerIdentity != "owner-1" || result.Decision.CanExecute || result.Decision.GrantsAuthority || result.CanonicalEntity == nil {
		t.Fatalf("contact review response crossed authority boundary: %#v", result)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/life-ontology/contact-review-decisions?limit=10", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(result.Decision.ID)) {
		t.Fatalf("contact review history status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func testLifeOntologyRouter(t *testing.T, service *Service, owner string) *gin.Engine {
	t.Helper()
	router := gin.New()
	ownerGuard := func(c *gin.Context) { c.Set(identity.ContextSubjectKey, owner); c.Next() }
	pass := func(c *gin.Context) { c.Next() }
	err := RegisterRoutes(router.Group("/api/v1"), NewHandler(service), RouteGuards{
		AuthenticatedOwner: ownerGuard, RecognizedRole: pass, Read: pass, Write: pass, Govern: pass,
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}
