package controlledlearning

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

var controlledLearningHandlerNow = time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

func TestControlledLearningHandlerRequiresOwnerAndRejectsIdentitySpoofing(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)

	response := performControlledLearningRequest(router, http.MethodGet, "/outcomes", "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, body=%s", response.Code, response.Body.String())
	}

	body := validOutcomeHTTPBody("outcome-spoof")
	body["ownerIdentity"] = "bob"
	response = performControlledLearningJSON(t, router, http.MethodPost, "/outcomes", "alice", body)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "bob") {
		t.Fatalf("spoofed owner response = %d %s", response.Code, response.Body.String())
	}

	body = validOutcomeHTTPBody("actor-spoof")
	body["actorIdentity"] = "mallory"
	response = performControlledLearningJSON(t, router, http.MethodPost, "/outcomes", "alice", body)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "mallory") {
		t.Fatalf("spoofed actor response = %d %s", response.Code, response.Body.String())
	}
}

func TestControlledLearningHandlerOutcomeRoundTripFiltersAndOwnerIsolation(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)

	first := createControlledLearningOutcome(t, router, "alice", "operation-a")
	_ = createControlledLearningOutcome(t, router, "alice", "operation-b")
	_ = createControlledLearningOutcome(t, router, "bob", "operation-a")

	response := performControlledLearningRequest(
		router,
		http.MethodGet,
		"/outcomes?operationId=operation-a&limit=1",
		"alice",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("list outcomes status = %d, body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Outcomes []OutcomeRecord `json:"outcomes"`
	}
	decodeControlledLearningResponse(t, response, &listed)
	if len(listed.Outcomes) != 1 ||
		listed.Outcomes[0].OwnerIdentity != "alice" ||
		listed.Outcomes[0].OperationID != "operation-a" {
		t.Fatalf("filtered outcomes = %#v", listed.Outcomes)
	}

	response = performControlledLearningRequest(
		router,
		http.MethodGet,
		"/outcomes/"+first.ID,
		"bob",
		"",
	)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "alice") {
		t.Fatalf("cross-owner get = %d %s", response.Code, response.Body.String())
	}

	response = performControlledLearningRequest(router, http.MethodGet, "/outcomes?limit=0", "alice", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestControlledLearningHandlerStrictBoundedJSON(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)

	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "malformed", body: `{"operationId":`, want: http.StatusBadRequest},
		{name: "trailing object", body: `{}` + `{}`, want: http.StatusBadRequest},
		{name: "empty", body: ``, want: http.StatusBadRequest},
		{
			name: "oversized",
			body: `{"summary":"` + strings.Repeat("s", maxControlledLearningRequestBytes) + `"}`,
			want: http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performControlledLearningRequest(
				router,
				http.MethodPost,
				"/outcomes",
				"alice",
				test.body,
			)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.want, response.Body.String())
			}
			if strings.Contains(response.Body.String(), strings.Repeat("s", 32)) {
				t.Fatalf("request payload leaked: %s", response.Body.String())
			}
		})
	}
}

func TestControlledLearningHandlerUsesAuthenticatedHumanForCorrectionsAndDecisions(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)

	correctionBody := validOutcomeHTTPBody("human-correction")
	correctionBody["basis"] = EvidenceHumanCorrection
	correctionBody["status"] = OutcomeCorrected
	correctionBody["verification"] = VerificationHumanApproved
	correctionBody["humanConfirmed"] = true
	correctionBody["correction"] = "Use the corrected project deadline."
	delete(correctionBody, "sources")
	response := performControlledLearningJSON(
		t,
		router,
		http.MethodPost,
		"/outcomes",
		"alice",
		correctionBody,
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("correction status = %d, body=%s", response.Code, response.Body.String())
	}
	var correction OutcomeRecord
	decodeControlledLearningResponse(t, response, &correction)
	if correction.ActorIdentity != "alice" || !correction.HumanConfirmed {
		t.Fatalf("correction actor = %#v", correction)
	}

	evidence := createControlledLearningOutcome(t, router, "alice", "decision-evidence")
	proposal := createControlledLearningProposal(
		t,
		router,
		"alice",
		"decision-proposal",
		TargetRecommendation,
		evidence.ID,
	)

	response = performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "alice", map[string]any{
		"expectedRevision": 1,
		"kind":             DecisionApprove,
		"humanConfirmed":   false,
		"rationale":        "Not explicitly confirmed.",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed decision status = %d, body=%s", response.Code, response.Body.String())
	}

	response = performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "alice", map[string]any{
		"expectedRevision": 1,
		"kind":             DecisionApprove,
		"humanConfirmed":   true,
		"rationale":        "Evidence supports this recommendation update.",
		"actorIdentity":    "mallory",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("spoofed decision actor status = %d, body=%s", response.Code, response.Body.String())
	}

	response = performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "alice", map[string]any{
		"expectedRevision": 1,
		"kind":             DecisionApprove,
		"humanConfirmed":   true,
		"rationale":        "Evidence supports this recommendation update.",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body=%s", response.Code, response.Body.String())
	}

	response = performControlledLearningRequest(
		router,
		http.MethodGet,
		"/proposals/"+proposal.ID+"/decisions?kind=approve&limit=1",
		"alice",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("list decisions status = %d, body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Decisions []ReviewDecision `json:"decisions"`
	}
	decodeControlledLearningResponse(t, response, &listed)
	if len(listed.Decisions) != 1 ||
		listed.Decisions[0].ActorIdentity != "alice" ||
		!listed.Decisions[0].HumanConfirmed {
		t.Fatalf("decision attribution = %#v", listed.Decisions)
	}

	response = performControlledLearningRequest(
		router,
		http.MethodGet,
		"/proposals/"+proposal.ID+"/decisions/"+listed.Decisions[0].ID,
		"alice",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("get decision status = %d, body=%s", response.Code, response.Body.String())
	}
	var decision ReviewDecision
	decodeControlledLearningResponse(t, response, &decision)
	if decision.ID != listed.Decisions[0].ID || decision.ActorIdentity != "alice" {
		t.Fatalf("decision = %#v", decision)
	}

	response = performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "bob", map[string]any{
		"expectedRevision": 1,
		"kind":             DecisionReject,
		"humanConfirmed":   true,
		"rationale":        "Cross-owner attempt.",
	})
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "alice") {
		t.Fatalf("cross-owner decision = %d %s", response.Code, response.Body.String())
	}
}

func TestControlledLearningHandlerProtectedTargetRequiresGovernance(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)
	evidence := createControlledLearningOutcome(t, router, "alice", "governance-evidence")
	proposal := createControlledLearningProposal(
		t,
		router,
		"alice",
		"governance-proposal",
		TargetConstitution,
		evidence.ID,
	)
	if proposal.Status != ProposalGovernanceRequired || !proposal.ProtectedTarget {
		t.Fatalf("protected proposal = %#v", proposal)
	}

	response := performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "alice", map[string]any{
		"expectedRevision": 1,
		"kind":             DecisionApprove,
		"humanConfirmed":   true,
		"rationale":        "Attempt ordinary approval.",
	})
	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "governance") {
		t.Fatalf("protected approval = %d %s", response.Code, response.Body.String())
	}

	response = performControlledLearningJSON(t, router, http.MethodPost, "/proposals/"+proposal.ID+"/decisions", "alice", map[string]any{
		"expectedRevision":    1,
		"kind":                DecisionEscalateGovernance,
		"humanConfirmed":      true,
		"rationale":           "Escalate for separate policy review.",
		"governanceReference": "governance-review-42",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("governance escalation = %d %s", response.Code, response.Body.String())
	}
	var updated DecisionResult
	decodeControlledLearningResponse(t, response, &updated)
	if updated.Proposal.Status != ProposalGovernanceReview || updated.Application == nil {
		t.Fatalf("governance decision = %#v", updated)
	}
}

func TestControlledLearningHandlerProposalListsUseOwnerScopedFilters(t *testing.T) {
	handler := newControlledLearningTestHandler(t, NewMemoryRepository())
	router := newControlledLearningTestRouter(handler)
	evidence := createControlledLearningOutcome(t, router, "alice", "filter-evidence")
	_ = createControlledLearningProposal(
		t,
		router,
		"alice",
		"ordinary-filter-proposal",
		TargetRecommendation,
		evidence.ID,
	)
	protected := createControlledLearningProposal(
		t,
		router,
		"alice",
		"protected-filter-proposal",
		TargetSafetyBoundary,
		evidence.ID,
	)
	_ = createControlledLearningProposal(
		t,
		router,
		"bob",
		"bob-filter-proposal",
		TargetRecommendation,
		createControlledLearningOutcome(t, router, "bob", "bob-filter-evidence").ID,
	)

	response := performControlledLearningRequest(
		router,
		http.MethodGet,
		"/proposals?status=governance_required&limit=1",
		"alice",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("filtered proposal list = %d %s", response.Code, response.Body.String())
	}
	var listed struct {
		Proposals []LearningProposal `json:"proposals"`
	}
	decodeControlledLearningResponse(t, response, &listed)
	if len(listed.Proposals) != 1 ||
		listed.Proposals[0].ID != protected.ID ||
		listed.Proposals[0].OwnerIdentity != "alice" {
		t.Fatalf("filtered proposals = %#v", listed.Proposals)
	}

	response = performControlledLearningRequest(
		router,
		http.MethodGet,
		"/proposals/"+protected.ID,
		"bob",
		"",
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner proposal get = %d %s", response.Code, response.Body.String())
	}

	response = performControlledLearningRequest(
		router,
		http.MethodGet,
		"/proposals?status=made_up",
		"alice",
		"",
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid proposal filter = %d %s", response.Code, response.Body.String())
	}
}

func TestControlledLearningHandlerRedactsRepositoryFailures(t *testing.T) {
	handler := newControlledLearningTestHandler(t, failingControlledLearningRepository{
		err: errors.New("postgres dsn password=top-secret source_payload=private"),
	})
	router := newControlledLearningTestRouter(handler)

	response := performControlledLearningRequest(router, http.MethodGet, "/outcomes", "alice", "")
	if response.Code != http.StatusInternalServerError ||
		!strings.Contains(response.Body.String(), `"errorId"`) ||
		strings.Contains(response.Body.String(), "top-secret") ||
		strings.Contains(response.Body.String(), "postgres") ||
		strings.Contains(response.Body.String(), "source_payload") {
		t.Fatalf("redacted failure = %d %s", response.Code, response.Body.String())
	}
}

func newControlledLearningTestHandler(t *testing.T, repository Repository) *Handler {
	t.Helper()
	var sequence atomic.Uint64
	service, err := NewService(
		repository,
		func() time.Time { return controlledLearningHandlerNow },
		func() string {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", sequence.Add(1))
		},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	service, _ = configureTestPromoter(t, service, controlledLearningHandlerNow)
	return NewHandler(service)
}

func newControlledLearningTestRouter(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if owner := strings.TrimSpace(c.GetHeader("X-Test-Owner")); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
		c.Next()
	})
	router.POST("/outcomes", handler.RecordOutcome)
	router.GET("/outcomes", handler.ListOutcomes)
	router.GET("/outcomes/:id", handler.GetOutcome)
	router.POST("/proposals", handler.Propose)
	router.GET("/proposals", handler.ListProposals)
	router.GET("/proposals/:id", handler.GetProposal)
	router.POST("/proposals/:id/decisions", handler.Decide)
	router.GET("/proposals/:id/decisions", handler.ListDecisions)
	router.GET("/proposals/:id/decisions/:decisionId", handler.GetDecision)
	return router
}

func createControlledLearningOutcome(
	t *testing.T,
	router http.Handler,
	owner, operationID string,
) OutcomeRecord {
	t.Helper()
	response := performControlledLearningJSON(
		t,
		router,
		http.MethodPost,
		"/outcomes",
		owner,
		validOutcomeHTTPBody(operationID),
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("create outcome = %d %s", response.Code, response.Body.String())
	}
	var record OutcomeRecord
	decodeControlledLearningResponse(t, response, &record)
	return record
}

func createControlledLearningProposal(
	t *testing.T,
	router http.Handler,
	owner, idempotencyKey string,
	target TargetKind,
	evidenceID string,
) LearningProposal {
	t.Helper()
	response := performControlledLearningJSON(t, router, http.MethodPost, "/proposals", owner, map[string]any{
		"idempotencyKey":  idempotencyKey,
		"method":          MethodReflection,
		"target":          target,
		"title":           "Improve the next controlled run",
		"hypothesis":      "The evidence supports a bounded improvement.",
		"proposedChange":  "Use the verified lesson in the next matching task.",
		"currentVersion":  "1.0.0",
		"proposedVersion": "1.0.1",
		"rollbackPlan":    "Restore version 1.0.0.",
		"evaluationPlan":  "Compare the next verified outcome.",
		"evidenceIds":     []string{evidenceID},
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create proposal = %d %s", response.Code, response.Body.String())
	}
	var proposal LearningProposal
	decodeControlledLearningResponse(t, response, &proposal)
	return proposal
}

func validOutcomeHTTPBody(operationID string) map[string]any {
	return map[string]any{
		"idempotencyKey": operationID,
		"operationId":    operationID,
		"basis":          EvidenceVerifiedOutcome,
		"status":         OutcomeSucceeded,
		"summary":        "The operation completed and the source confirms the result.",
		"humanConfirmed": false,
		"verification":   VerificationSourceSupported,
		"sources": []map[string]any{{
			"id":          "source-" + operationID,
			"kind":        "test",
			"uri":         "https://example.test/evidence/" + operationID,
			"retrievedAt": controlledLearningHandlerNow.Add(-time.Minute).Format(time.RFC3339),
			"contentHash": "sha256:test",
		}},
		"occurredAt": controlledLearningHandlerNow.Add(-2 * time.Minute).Format(time.RFC3339),
	}
}

func performControlledLearningJSON(
	t *testing.T,
	router http.Handler,
	method, path, owner string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return performControlledLearningRequest(router, method, path, owner, string(encoded))
}

func performControlledLearningRequest(
	router http.Handler,
	method, path, owner, body string,
) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if owner != "" {
		request.Header.Set("X-Test-Owner", owner)
	}
	router.ServeHTTP(response, request)
	return response
}

func decodeControlledLearningResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}

type failingControlledLearningRepository struct {
	err error
}

func (repository failingControlledLearningRepository) CreateOutcome(
	context.Context,
	OutcomeRecord,
) (OutcomeRecord, error) {
	return OutcomeRecord{}, repository.err
}

func (repository failingControlledLearningRepository) GetOutcome(
	context.Context,
	string,
	string,
) (OutcomeRecord, error) {
	return OutcomeRecord{}, repository.err
}

func (repository failingControlledLearningRepository) ListOutcomes(
	context.Context,
	OutcomeQuery,
) ([]OutcomeRecord, error) {
	return nil, repository.err
}

func (repository failingControlledLearningRepository) CreateProposal(
	context.Context,
	LearningProposal,
) (LearningProposal, error) {
	return LearningProposal{}, repository.err
}

func (repository failingControlledLearningRepository) GetProposal(
	context.Context,
	string,
	string,
) (LearningProposal, error) {
	return LearningProposal{}, repository.err
}

func (repository failingControlledLearningRepository) ListProposals(
	context.Context,
	ProposalQuery,
) ([]LearningProposal, error) {
	return nil, repository.err
}

func (repository failingControlledLearningRepository) DecideProposal(
	context.Context,
	string,
	string,
	int64,
	ReviewDecision,
	ProposalStatus,
) (LearningProposal, error) {
	return LearningProposal{}, repository.err
}

func (repository failingControlledLearningRepository) ListDecisions(
	context.Context,
	string,
	string,
) ([]ReviewDecision, error) {
	return nil, repository.err
}
