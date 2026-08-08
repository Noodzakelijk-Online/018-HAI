package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/lifeontology"
)

func TestDurablePlanProjectsIntoLifeGraphButPreviewDoesNot(t *testing.T) {
	ontology := lifeontology.NewService(nil, nil)
	configured, err := WithLifeOntologyProjection(
		NewService(&fakeMemoryService{}, newTaskTestLLMService(t)),
		ontology,
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, ok := configured.(PreviewService)
	if !ok {
		t.Fatal("configured service lost preview boundary")
	}
	request := IntakeRequest{
		OwnerIdentity: "owner-1", ProjectKey: "018-hai",
		Request: "Review the HAI engine and define verified completion criteria",
	}
	draft, err := preview.Preview(request)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if draft.LifeGraphProjection != nil || draft.LifeGraphProjectionError != "" {
		t.Fatalf("preview performed a durable projection: %#v", draft)
	}
	entities, err := ontology.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{AllowLocalOnly: true})
	if err != nil || len(entities) != 0 {
		t.Fatalf("preview wrote life graph state: %#v %v", entities, err)
	}

	plan, err := configured.Plan(request)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.LifeGraphProjection == nil || plan.LifeGraphProjectionError != "" {
		t.Fatalf("durable plan was not projected: %#v", plan)
	}
	if !plan.LifeGraphProjection.AdvisoryOnly || plan.LifeGraphProjection.CanExecute || plan.LifeGraphProjection.GrantsAuthority {
		t.Fatalf("projection crossed authority boundary: %#v", plan.LifeGraphProjection)
	}
	if len(plan.LifeGraphProjection.LinkedEntities) != 1 || plan.LifeGraphProjection.LinkedEntities[0].Type != lifeontology.EntityProject {
		t.Fatalf("project relation missing: %#v", plan.LifeGraphProjection)
	}
	entities, err = ontology.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil || len(entities) != 2 {
		t.Fatalf("durable projection graph mismatch: count=%d err=%v", len(entities), err)
	}
}

func TestProjectionFailureIsVisibleWithoutFailingDurablePlanning(t *testing.T) {
	configured, err := WithLifeOntologyProjection(
		NewService(&fakeMemoryService{}, newTaskTestLLMService(t)),
		failingLifeProjectionRecorder{},
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := configured.Plan(IntakeRequest{OwnerIdentity: "owner-1", Request: "Prepare a bounded internal plan"})
	if err != nil {
		t.Fatalf("advisory projection failure blocked planning: %v", err)
	}
	if !strings.Contains(plan.LifeGraphProjectionError, "ledger unavailable") || plan.LifeGraphProjection != nil {
		t.Fatalf("projection failure was hidden: %#v", plan)
	}
	logs := configured.Logs()
	if len(logs) != 1 || logs[0].LifeGraphProjectionError == "" {
		t.Fatalf("projection failure was not persisted in task history: %#v", logs)
	}
}

func TestWithLifeOntologyProjectionValidatesBoundary(t *testing.T) {
	if _, err := WithLifeOntologyProjection(nil, failingLifeProjectionRecorder{}); err == nil {
		t.Fatal("non-built-in task service accepted")
	}
	if _, err := WithLifeOntologyProjection(NewService(&fakeMemoryService{}, newTaskTestLLMService(t)), nil); err == nil {
		t.Fatal("nil projection recorder accepted")
	}
}

type failingLifeProjectionRecorder struct{}

func (failingLifeProjectionRecorder) ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
	return lifeontology.OperationalProjectionResult{}, errors.New("life ontology ledger unavailable")
}
