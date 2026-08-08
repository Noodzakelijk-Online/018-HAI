package knowledgegraph

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestClaimRoutesExposeOwnerScopedLifecycleAndAssessment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := claimTestService(NewMemoryRepository())
	router := gin.New()
	allow := func(c *gin.Context) { c.Next() }
	authenticate := func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "robert")
		c.Next()
	}
	err := RegisterClaimRoutes(router.Group("/api/v1"), NewClaimHandler(service), ClaimRouteGuards{
		AuthenticatedOwner: authenticate,
		RecognizedRole:     allow,
		Read:               allow,
		Write:              allow,
		Approve:            allow,
	})
	if err != nil {
		t.Fatalf("RegisterClaimRoutes: %v", err)
	}

	request := claimTestRequest("ready")
	request.VerificationStatus = VerificationUnverified
	body, err := json.Marshal(recordClaimBody{
		WorkspaceID: request.WorkspaceID, Subject: request.Subject, Predicate: request.Predicate,
		Object: request.Object, EffectiveFrom: request.EffectiveFrom, ObservedAt: request.ObservedAt,
		VerificationStatus: request.VerificationStatus, Provenance: request.Provenance,
		Sensitivity: request.Sensitivity,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performClaimRequest(router, http.MethodPost, "/api/v1/knowledge/claims", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("record status = %d body=%s", response.Code, response.Body.String())
	}
	var created Claim
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("decode created claim: %#v err=%v", created, err)
	}

	for _, path := range []string{
		"/api/v1/knowledge/claims?workspaceId=hai",
		"/api/v1/knowledge/claims/review-queue?workspaceId=hai",
		"/api/v1/knowledge/claims/" + created.ID + "?workspaceId=hai",
		"/api/v1/knowledge/claims/" + created.ID + "/lifecycle?workspaceId=hai",
		"/api/v1/knowledge/claims/" + created.ID + "/assessment?workspaceId=hai",
	} {
		response = performClaimRequest(router, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body=%s", path, response.Code, response.Body.String())
		}
	}
	var assessment ClaimAssessment
	if err := json.Unmarshal(response.Body.Bytes(), &assessment); err != nil || assessment.Status != ClaimAssessmentNeedsReview {
		t.Fatalf("assessment = %#v err=%v", assessment, err)
	}
}

func TestClaimRoutesRejectMissingScopeUnknownFieldsAndBadTimes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := claimTestService(NewMemoryRepository())
	router := gin.New()
	allow := func(c *gin.Context) { c.Next() }
	authenticate := func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "robert")
		c.Next()
	}
	if err := RegisterClaimRoutes(router.Group("/api/v1"), NewClaimHandler(service), ClaimRouteGuards{
		AuthenticatedOwner: authenticate, RecognizedRole: allow, Read: allow, Write: allow, Approve: allow,
	}); err != nil {
		t.Fatal(err)
	}

	response := performClaimRequest(router, http.MethodGet, "/api/v1/knowledge/claims", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d", response.Code)
	}
	response = performClaimRequest(router, http.MethodPost, "/api/v1/knowledge/claims", []byte(`{"workspaceId":"hai","ownerIdentity":"other"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("caller-supplied owner status = %d", response.Code)
	}
	privileged, err := json.Marshal(recordClaimBody{
		WorkspaceID: "hai", Subject: "HAI", Predicate: "status", Object: "ready",
		EffectiveFrom: claimTestNow.Add(-time.Hour), ObservedAt: claimTestNow.Add(-time.Minute),
		VerificationStatus: VerificationVerified,
		Provenance:         []ClaimProvenance{{ReferenceID: "manual", ContentDigest: claimTestDigest("ready"), CapturedAt: claimTestNow.Add(-time.Minute)}},
		Sensitivity:        SensitivityInternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	response = performClaimRequest(router, http.MethodPost, "/api/v1/knowledge/claims", privileged)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "trusted verification") {
		t.Fatalf("privileged status = %d body=%s", response.Code, response.Body.String())
	}
	response = performClaimRequest(router, http.MethodGet, "/api/v1/knowledge/claims/id/assessment?workspaceId=hai&observedBy=tomorrow", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad timestamp status = %d", response.Code)
	}
}

func TestClaimCorrectionRequiresApprovalRouteAndCreatesImmutableSuccessor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := claimTestService(NewMemoryRepository())
	router := gin.New()
	allow := func(c *gin.Context) { c.Next() }
	authenticate := func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "robert")
		c.Next()
	}
	if err := RegisterClaimRoutes(router.Group("/api/v1"), NewClaimHandler(service), ClaimRouteGuards{
		AuthenticatedOwner: authenticate, RecognizedRole: allow, Read: allow, Write: allow, Approve: allow,
	}); err != nil {
		t.Fatal(err)
	}
	targetRequest := claimTestRequest("old value")
	targetRequest.VerificationStatus = VerificationNeedsReview
	target := recordClaim(t, service, targetRequest)
	body, err := json.Marshal(correctClaimBody{
		WorkspaceID: "hai", RequestID: "ui-correction-1",
		CorrectedObject: "corrected value", Reason: "Robert confirmed the corrected value.",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performClaimRequest(router, http.MethodPost, "/api/v1/knowledge/claims/"+target.ID+"/corrections", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("correction status = %d body=%s", response.Code, response.Body.String())
	}
	var corrected Claim
	if err := json.Unmarshal(response.Body.Bytes(), &corrected); err != nil {
		t.Fatal(err)
	}
	if corrected.Object != "corrected value" || corrected.VerificationStatus != VerificationHumanApproved ||
		!containsString(corrected.SupersedesClaimIDs, target.ID) || len(corrected.Provenance) != 1 ||
		corrected.Provenance[0].SourceNodeID == "" || !corrected.LocalOnly {
		t.Fatalf("unexpected corrected claim: %#v", corrected)
	}
	retry := performClaimRequest(router, http.MethodPost, "/api/v1/knowledge/claims/"+target.ID+"/corrections", body)
	if retry.Code != http.StatusCreated {
		t.Fatalf("idempotent retry status = %d body=%s", retry.Code, retry.Body.String())
	}
	var retried Claim
	if err := json.Unmarshal(retry.Body.Bytes(), &retried); err != nil || retried.ID != corrected.ID {
		t.Fatalf("idempotent correction = %#v err=%v", retried, err)
	}
	lifecycle, err := service.GetClaimLifecycle(context.Background(), "robert", "hai", target.ID)
	if err != nil || len(lifecycle.SupersededBy) != 1 || lifecycle.SupersededBy[0].ID != corrected.ID {
		t.Fatalf("correction lifecycle = %#v err=%v", lifecycle, err)
	}
}

func TestRegisterClaimRoutesRequiresAllGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := claimTestService(NewMemoryRepository())
	if err := RegisterClaimRoutes(gin.New().Group("/api/v1"), NewClaimHandler(service), ClaimRouteGuards{}); err == nil {
		t.Fatal("claim routes registered without security guards")
	}
}

func performClaimRequest(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
