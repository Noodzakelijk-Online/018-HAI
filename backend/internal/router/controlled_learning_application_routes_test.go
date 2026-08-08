package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/controlledlearning"

	"github.com/gin-gonic/gin"
)

func TestControlledLearningApplicationRoutesAreOwnerScopedAndFailClosed(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	application := createAppliedLearningApplication(t, engine)

	unauthenticated := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications",
		"",
		"",
		"owner",
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}

	unknownRole := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications",
		"",
		"alice",
		"root",
	)
	if unknownRole.Code != http.StatusForbidden {
		t.Fatalf("unknown-role list = %d: %s", unknownRole.Code, unknownRole.Body.String())
	}

	list := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications?status=applied&proposalId="+
			application.ProposalID+"&limit=1",
		"",
		"alice",
		"viewer",
	)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), application.ID) {
		t.Fatalf("owner list = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "rollbackToken") {
		t.Fatalf("owner list exposed rollback capability: %s", list.Body.String())
	}

	direct := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications/"+application.ID,
		"",
		"alice",
		"viewer",
	)
	if direct.Code != http.StatusOK || !strings.Contains(direct.Body.String(), application.ID) {
		t.Fatalf("owner get = %d: %s", direct.Code, direct.Body.String())
	}
	if strings.Contains(direct.Body.String(), "rollbackToken") {
		t.Fatalf("owner get exposed rollback capability: %s", direct.Body.String())
	}

	crossOwnerList := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications?proposalId="+application.ProposalID,
		"",
		"bob",
		"viewer",
	)
	if crossOwnerList.Code != http.StatusOK ||
		strings.Contains(crossOwnerList.Body.String(), application.ID) ||
		strings.Contains(crossOwnerList.Body.String(), "alice") {
		t.Fatalf("cross-owner list = %d: %s", crossOwnerList.Code, crossOwnerList.Body.String())
	}

	for _, suffix := range []string{"", "/events"} {
		crossOwnerRead := performControlledLearningRouteRequest(
			engine,
			http.MethodGet,
			"/api/v1/controlled-learning/applications/"+application.ID+suffix,
			"",
			"bob",
			"viewer",
		)
		if crossOwnerRead.Code != http.StatusNotFound ||
			strings.Contains(crossOwnerRead.Body.String(), "alice") {
			t.Fatalf("cross-owner %q = %d: %s", suffix, crossOwnerRead.Code, crossOwnerRead.Body.String())
		}
	}

	for _, path := range []string{
		"/api/v1/controlled-learning/applications?status=unknown",
		"/api/v1/controlled-learning/applications?limit=0",
		"/api/v1/controlled-learning/applications/" + application.ID + "/events?limit=501",
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
			t.Errorf("invalid read %s = %d: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestControlledLearningApplicationRollbackRequiresOwnerConfirmationAndIsIdempotent(t *testing.T) {
	engine := newControlledLearningRouteTestEngine(t)
	application := createAppliedLearningApplication(t, engine)
	path := "/api/v1/controlled-learning/applications/" + application.ID + "/rollback"

	rollbackBody := `{
		"idempotencyKey":"rollback-route-1",
		"expectedVersion":"1.0.1",
		"humanConfirmed":true,
		"rationale":"Robert deliberately restores the prior verified recommendation."
	}`
	for _, role := range []string{"viewer", "operator"} {
		response := performControlledLearningRouteRequest(
			engine,
			http.MethodPost,
			path,
			rollbackBody,
			"alice",
			role,
		)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s rollback = %d: %s", role, response.Code, response.Body.String())
		}
	}

	for name, body := range map[string]string{
		"confirmation": `{
			"idempotencyKey":"rollback-route-1",
			"expectedVersion":"1.0.1",
			"humanConfirmed":false,
			"rationale":"Robert deliberately restores the prior verified recommendation."
		}`,
		"idempotency": `{
			"expectedVersion":"1.0.1",
			"humanConfirmed":true,
			"rationale":"Robert deliberately restores the prior verified recommendation."
		}`,
		"unknown field": `{
			"idempotencyKey":"rollback-route-1",
			"expectedVersion":"1.0.1",
			"humanConfirmed":true,
			"rationale":"Robert deliberately restores the prior verified recommendation.",
			"ownerIdentity":"bob"
		}`,
	} {
		response := performControlledLearningRouteRequest(
			engine,
			http.MethodPost,
			path,
			body,
			"alice",
			"owner",
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid %s rollback = %d: %s", name, response.Code, response.Body.String())
		}
	}

	wrongVersion := strings.Replace(rollbackBody, "1.0.1", "9.9.9", 1)
	versionConflict := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		path,
		wrongVersion,
		"alice",
		"owner",
	)
	if versionConflict.Code != http.StatusConflict {
		t.Fatalf("wrong-version rollback = %d: %s", versionConflict.Code, versionConflict.Body.String())
	}

	rolledBack := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		path,
		rollbackBody,
		"alice",
		"owner",
	)
	if rolledBack.Code != http.StatusOK {
		t.Fatalf("owner rollback = %d: %s", rolledBack.Code, rolledBack.Body.String())
	}
	var record controlledlearning.ApplicationRecord
	decodeControlledLearningRouteResponse(t, rolledBack, &record)
	if record.ID != application.ID ||
		record.Status != controlledlearning.ApplicationRolledBack ||
		record.RestoredVersion != application.CurrentVersion ||
		record.RollbackToken != "" {
		t.Fatalf("rollback result = %#v", record)
	}

	replayed := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		path,
		rollbackBody,
		"alice",
		"owner",
	)
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), application.ID) {
		t.Fatalf("idempotent rollback replay = %d: %s", replayed.Code, replayed.Body.String())
	}

	conflictingBody := strings.Replace(
		rollbackBody,
		"prior verified recommendation",
		"different rollback intent",
		1,
	)
	conflict := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		path,
		conflictingBody,
		"alice",
		"owner",
	)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict = %d: %s", conflict.Code, conflict.Body.String())
	}

	events := performControlledLearningRouteRequest(
		engine,
		http.MethodGet,
		"/api/v1/controlled-learning/applications/"+application.ID+"/events?limit=2",
		"",
		"alice",
		"viewer",
	)
	if events.Code != http.StatusOK {
		t.Fatalf("events = %d: %s", events.Code, events.Body.String())
	}
	var eventList struct {
		Events []controlledlearning.ApplicationEvent `json:"events"`
	}
	decodeControlledLearningRouteResponse(t, events, &eventList)
	if len(eventList.Events) != 2 ||
		eventList.Events[0].Kind != controlledlearning.ApplicationEventRollbackStarted ||
		eventList.Events[1].Kind != controlledlearning.ApplicationEventRolledBack {
		t.Fatalf("latest rollback events = %#v", eventList.Events)
	}
}

func createAppliedLearningApplication(
	t *testing.T,
	engine *gin.Engine,
) controlledlearning.ApplicationRecord {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	outcomeResponse := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/outcomes",
		controlledLearningOutcomeRouteBody(now, "application-route-outcome"),
		"alice",
		"owner",
	)
	if outcomeResponse.Code != http.StatusCreated {
		t.Fatalf("create outcome = %d: %s", outcomeResponse.Code, outcomeResponse.Body.String())
	}
	var outcome controlledlearning.OutcomeRecord
	decodeControlledLearningRouteResponse(t, outcomeResponse, &outcome)

	proposalResponse := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals",
		controlledLearningProposalRouteBody(outcome.ID),
		"alice",
		"operator",
	)
	if proposalResponse.Code != http.StatusCreated {
		t.Fatalf("create proposal = %d: %s", proposalResponse.Code, proposalResponse.Body.String())
	}
	var proposal controlledlearning.LearningProposal
	decodeControlledLearningRouteResponse(t, proposalResponse, &proposal)

	decisionPayload, _ := json.Marshal(map[string]any{
		"idempotencyKey":   "application-route-approval",
		"expectedRevision": proposal.Revision,
		"kind":             controlledlearning.DecisionApprove,
		"humanConfirmed":   true,
		"rationale":        "Robert approves this source-backed, reversible recommendation update.",
	})
	decisionResponse := performControlledLearningRouteRequest(
		engine,
		http.MethodPost,
		"/api/v1/controlled-learning/proposals/"+proposal.ID+"/decisions",
		string(decisionPayload),
		"alice",
		"owner",
	)
	if decisionResponse.Code != http.StatusOK {
		t.Fatalf("approve proposal = %d: %s", decisionResponse.Code, decisionResponse.Body.String())
	}
	var decision controlledlearning.DecisionResult
	decodeControlledLearningRouteResponse(t, decisionResponse, &decision)
	if decision.Application == nil || decision.Application.Status != controlledlearning.ApplicationApplied {
		t.Fatalf("application decision = %#v", decision)
	}
	if decision.Application.RollbackToken != "" ||
		strings.Contains(decisionResponse.Body.String(), "rollbackToken") {
		t.Fatalf("application decision exposed rollback capability: %s", decisionResponse.Body.String())
	}
	return *decision.Application
}
