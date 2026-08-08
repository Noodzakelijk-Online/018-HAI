package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/frameworkregistry"
	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestControlledLearningAndTaxonomyRoutesRequireAuthenticatedOwner(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	for _, path := range []string{
		"/api/v1/controlled-learning/outcomes",
		"/api/v1/controlled-learning/proposals",
		"/api/v1/framework-registry/family-taxonomy",
	} {
		response := performControlledLearningRouteRequest(
			engine,
			http.MethodGet,
			path,
			"",
			"",
			"owner",
		)
		if response.Code != http.StatusUnauthorized {
			t.Errorf(
				"%s status = %d, want %d: %s",
				path,
				response.Code,
				http.StatusUnauthorized,
				response.Body.String(),
			)
		}
	}
}

func TestControlledLearningRoutePermissionsOwnerScopeAndLifecycle(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	now := time.Now().UTC().Truncate(time.Second)
	outcomeBody := controlledLearningOutcomeRouteBody(now, "route-outcome-1")

	viewerCreate := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/outcomes",
		outcomeBody,
		"alice",
		"viewer",
	)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer outcome create = %d: %s", viewerCreate.Code, viewerCreate.Body.String())
	}
	operatorCreate := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/outcomes",
		outcomeBody,
		"alice",
		"operator",
	)
	if operatorCreate.Code != http.StatusForbidden {
		t.Fatalf("operator outcome create = %d: %s", operatorCreate.Code, operatorCreate.Body.String())
	}

	createdOutcome := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/outcomes",
		outcomeBody,
		"alice",
		"owner",
	)
	if createdOutcome.Code != http.StatusCreated {
		t.Fatalf("owner outcome create = %d: %s", createdOutcome.Code, createdOutcome.Body.String())
	}
	var outcome controlledlearning.OutcomeRecord
	decodeControlledLearningRouteResponse(t, createdOutcome, &outcome)
	if outcome.OwnerIdentity != "alice" || outcome.ID == "" {
		t.Fatalf("created outcome = %#v", outcome)
	}
	directOutcome := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/outcomes/"+outcome.ID,
		"",
		"alice",
		"viewer",
	)
	if directOutcome.Code != http.StatusOK ||
		!strings.Contains(directOutcome.Body.String(), outcome.ID) {
		t.Fatalf("direct outcome read = %d: %s", directOutcome.Code, directOutcome.Body.String())
	}

	aliceRead := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/outcomes?limit=1",
		"",
		"alice",
		"viewer",
	)
	if aliceRead.Code != http.StatusOK ||
		!strings.Contains(aliceRead.Body.String(), outcome.ID) {
		t.Fatalf("Alice outcome list = %d: %s", aliceRead.Code, aliceRead.Body.String())
	}
	bobRead := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/outcomes?ownerIdentity=alice&limit=1",
		"",
		"bob",
		"viewer",
	)
	if bobRead.Code != http.StatusOK ||
		strings.Contains(bobRead.Body.String(), outcome.ID) ||
		strings.Contains(bobRead.Body.String(), "alice") {
		t.Fatalf("cross-owner outcome list = %d: %s", bobRead.Code, bobRead.Body.String())
	}

	proposalBody := controlledLearningProposalRouteBody(outcome.ID)
	viewerProposal := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals",
		proposalBody,
		"alice",
		"viewer",
	)
	if viewerProposal.Code != http.StatusForbidden {
		t.Fatalf("viewer proposal create = %d: %s", viewerProposal.Code, viewerProposal.Body.String())
	}
	createdProposal := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals",
		proposalBody,
		"alice",
		"operator",
	)
	if createdProposal.Code != http.StatusCreated {
		t.Fatalf("operator proposal create = %d: %s", createdProposal.Code, createdProposal.Body.String())
	}
	var proposal controlledlearning.LearningProposal
	decodeControlledLearningRouteResponse(t, createdProposal, &proposal)
	if proposal.OwnerIdentity != "alice" || proposal.Status != controlledlearning.ProposalReviewRequired {
		t.Fatalf("created proposal = %#v", proposal)
	}
	proposalList := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/proposals?status=review_required&limit=1",
		"",
		"alice",
		"viewer",
	)
	if proposalList.Code != http.StatusOK ||
		!strings.Contains(proposalList.Body.String(), proposal.ID) {
		t.Fatalf("proposal list = %d: %s", proposalList.Code, proposalList.Body.String())
	}

	decisionBody := `{
		"expectedRevision":1,
		"kind":"approve",
		"humanConfirmed":true,
		"rationale":"The verified evidence supports this bounded update."
	}`
	operatorDecision := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals/"+proposal.ID+"/decisions",
		decisionBody,
		"alice",
		"operator",
	)
	if operatorDecision.Code != http.StatusForbidden {
		t.Fatalf("operator decision = %d: %s", operatorDecision.Code, operatorDecision.Body.String())
	}
	ownerDecision := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals/"+proposal.ID+"/decisions",
		decisionBody,
		"alice",
		"owner",
	)
	if ownerDecision.Code != http.StatusOK {
		t.Fatalf("owner decision = %d: %s", ownerDecision.Code, ownerDecision.Body.String())
	}

	decisions := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/proposals/"+proposal.ID+"/decisions?limit=1",
		"",
		"alice",
		"viewer",
	)
	if decisions.Code != http.StatusOK ||
		!strings.Contains(decisions.Body.String(), `"kind":"approve"`) {
		t.Fatalf("decision list = %d: %s", decisions.Code, decisions.Body.String())
	}
	var decisionList struct {
		Decisions []controlledlearning.ReviewDecision `json:"decisions"`
	}
	decodeControlledLearningRouteResponse(t, decisions, &decisionList)
	if len(decisionList.Decisions) != 1 {
		t.Fatalf("decision list = %#v", decisionList.Decisions)
	}
	directDecision := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/proposals/"+proposal.ID+
			"/decisions/"+decisionList.Decisions[0].ID,
		"",
		"alice",
		"viewer",
	)
	if directDecision.Code != http.StatusOK ||
		!strings.Contains(directDecision.Body.String(), decisionList.Decisions[0].ID) {
		t.Fatalf("direct decision read = %d: %s", directDecision.Code, directDecision.Body.String())
	}
	bobProposal := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/proposals/"+proposal.ID,
		"",
		"bob",
		"viewer",
	)
	if bobProposal.Code != http.StatusNotFound ||
		strings.Contains(bobProposal.Body.String(), "alice") {
		t.Fatalf("cross-owner proposal read = %d: %s", bobProposal.Code, bobProposal.Body.String())
	}
}

func TestControlledLearningRoutesRejectSpoofingUnknownJSONAndInvalidLimits(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	now := time.Now().UTC().Truncate(time.Second)
	spoofedBody := strings.TrimSuffix(
		controlledLearningOutcomeRouteBody(now, "spoofed-owner"),
		"}",
	) + `,"ownerIdentity":"bob"}`
	spoofed := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/outcomes",
		spoofedBody,
		"alice",
		"owner",
	)
	if spoofed.Code != http.StatusBadRequest ||
		strings.Contains(spoofed.Body.String(), "bob") {
		t.Fatalf("spoofed owner response = %d: %s", spoofed.Code, spoofed.Body.String())
	}

	for _, path := range []string{
		"/api/v1/controlled-learning/outcomes?limit=0",
		"/api/v1/controlled-learning/outcomes?limit=501",
		"/api/v1/controlled-learning/outcomes?limit=1&limit=2",
		"/api/v1/controlled-learning/proposals?limit=invalid",
	} {
		response := performControlledLearningRouteRequest(
			engine,
			http.MethodGet,
			path,
			"",
			"alice",
			"viewer",
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf(
				"%s status = %d, want %d: %s",
				path,
				response.Code,
				http.StatusBadRequest,
				response.Body.String(),
			)
		}
	}
}

func TestFrameworkTaxonomyProductionRouteReadPermissionAndStableMetadata(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	response := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/framework-registry/family-taxonomy",
		"",
		"alice",
		"viewer",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("taxonomy status = %d: %s", response.Code, response.Body.String())
	}
	var taxonomy frameworkregistry.FrameworkFamilyTaxonomy
	decodeControlledLearningRouteResponse(t, response, &taxonomy)
	if taxonomy.Version != frameworkregistry.FrameworkFamilyTaxonomyVersion ||
		taxonomy.Digest == "" ||
		len(taxonomy.Families) != 55 {
		t.Fatalf("taxonomy = %#v", taxonomy)
	}
	if err := frameworkregistry.ValidateFamilyTaxonomy(taxonomy); err != nil {
		t.Fatalf("ValidateFamilyTaxonomy: %v", err)
	}

	unknownRole := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/framework-registry/family-taxonomy",
		"",
		"alice",
		"root",
	)
	if unknownRole.Code != http.StatusOK {
		t.Fatalf(
			"unknown authenticated role did not receive least-privilege read: %d %s",
			unknownRole.Code,
			unknownRole.Body.String(),
		)
	}
}

func newControlledLearningRouteTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var sequence atomic.Uint64
	service, err := controlledlearning.NewService(
		controlledlearning.NewMemoryRepository(),
		time.Now,
		func() string {
			return fmt.Sprintf(
				"00000000-0000-4000-8000-%012d",
				sequence.Add(1),
			)
		},
	)
	if err != nil {
		t.Fatalf("controlledlearning.NewService: %v", err)
	}
	frameworkService, err := frameworkregistry.NewService(
		frameworkregistry.NewMemoryRepository(),
	)
	if err != nil {
		t.Fatalf("frameworkregistry.NewService: %v", err)
	}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if subject := strings.TrimSpace(c.GetHeader("X-Test-Verified-Subject")); subject != "" {
			c.Set(identity.ContextSubjectKey, subject)
		}
		if role := strings.TrimSpace(c.GetHeader("X-Test-Verified-Role")); role != "" {
			c.Set(identity.ContextRoleKey, role)
		}
		c.Next()
	})
	v1 := engine.Group("/api/v1")
	initializeControlledLearningRoutes(v1, controlledlearning.NewHandler(service))
	initializeFrameworkRegistryRoutes(v1, frameworkregistry.NewHandler(frameworkService))
	return engine
}

func controlledLearningOutcomeRouteBody(now time.Time, key string) string {
	payload := map[string]any{
		"idempotencyKey": key,
		"operationId":    key,
		"basis":          controlledlearning.EvidenceVerifiedOutcome,
		"status":         controlledlearning.OutcomeSucceeded,
		"summary":        "The source-backed operation completed successfully.",
		"humanConfirmed": false,
		"verification":   controlledlearning.VerificationSourceSupported,
		"sources": []map[string]any{{
			"id":          "source-" + key,
			"kind":        "test",
			"uri":         "https://example.test/evidence/" + key,
			"retrievedAt": now.Add(-time.Minute),
			"contentHash": "sha256:test",
		}},
		"occurredAt": now.Add(-2 * time.Minute),
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func controlledLearningProposalRouteBody(evidenceID string) string {
	payload := map[string]any{
		"idempotencyKey":  "route-proposal-1",
		"method":          controlledlearning.MethodReflection,
		"target":          controlledlearning.TargetRecommendation,
		"title":           "Reuse the verified lesson",
		"hypothesis":      "The source-backed outcome supports this bounded change.",
		"proposedChange":  "Apply the lesson to matching future recommendations.",
		"currentVersion":  "1.0.0",
		"proposedVersion": "1.0.1",
		"rollbackPlan":    "Restore version 1.0.0.",
		"evaluationPlan":  "Compare the next verified matching outcome.",
		"evidenceIds":     []string{evidenceID},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func performControlledLearningRouteRequest(
	engine *gin.Engine,
	method string,
	path string,
	body string,
	subject string,
	role string,
) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if subject != "" {
		request.Header.Set("X-Test-Verified-Subject", subject)
	}
	if role != "" {
		request.Header.Set("X-Test-Verified-Role", role)
	}
	engine.ServeHTTP(response, request)
	return response
}

func decodeControlledLearningRouteResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}
