package outcomeevaluation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

type projectionFunc func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)

func (function projectionFunc) ProjectOperationalRecord(ctx context.Context, request lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
	return function(ctx, request)
}

func TestOutcomeAndEvaluationProjectIntoOneOwnerScopedLifeGraph(t *testing.T) {
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	graph := lifeontology.NewService(nil, func() time.Time { return now })
	service, err := WithLifeOntologyProjection(
		newService(NewMemoryRepository(), func() time.Time { return now }),
		graph,
	)
	if err != nil {
		t.Fatal(err)
	}

	definition, created, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "project-definition", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil || !created {
		t.Fatalf("StoreOutcome() created=%v err=%v", created, err)
	}
	if definition.LifeGraphProjection == nil || definition.LifeGraphProjectionWarning != "" {
		t.Fatalf("definition projection = %#v warning=%q", definition.LifeGraphProjection, definition.LifeGraphProjectionWarning)
	}
	if !definition.LifeGraphProjection.AdvisoryOnly || definition.LifeGraphProjection.CanExecute || definition.LifeGraphProjection.GrantsAuthority {
		t.Fatalf("definition projection crossed authority boundary: %#v", definition.LifeGraphProjection)
	}
	if definition.LifeGraphProjection.Primary.Domain != lifeontology.DomainPersonalAdmin ||
		definition.LifeGraphProjection.Primary.Attributes["domain_assignment"] != "explicit_outcome_definition" {
		t.Fatalf("definition domain projection = %#v", definition.LifeGraphProjection.Primary)
	}
	if err := VerifyOutcomeRevisionDigest(definition); err != nil {
		t.Fatalf("projection metadata changed definition digest: %v", err)
	}

	retry, created, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "project-definition", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil || created || retry.LifeGraphProjection == nil || !retry.LifeGraphProjection.AlreadyExisted {
		t.Fatalf("idempotent projection repair = created=%v record=%#v err=%v", created, retry, err)
	}

	evaluation, created, err := service.CreateEvaluation(context.Background(), "owner-1", "workspace-1", "outcome-1", CreateEvaluationRequest{
		IdempotencyKey: "project-evaluation", OutcomeRevision: 1,
		Observations: []Observation{
			observation("obs-1", 12, testStart.Add(5*24*time.Hour)),
			observation("obs-2", 16, testStart.Add(15*24*time.Hour)),
		},
		AsOf: testAsOf,
	})
	if err != nil || !created {
		t.Fatalf("CreateEvaluation() created=%v err=%v", created, err)
	}
	if evaluation.LifeGraphProjection == nil || evaluation.LifeGraphProjectionWarning != "" {
		t.Fatalf("evaluation projection = %#v warning=%q", evaluation.LifeGraphProjection, evaluation.LifeGraphProjectionWarning)
	}
	if evaluation.LifeGraphProjection.Primary.Domain != lifeontology.DomainPersonalAdmin ||
		evaluation.LifeGraphProjection.Primary.Attributes["domain_assignment"] != "explicit_outcome_definition" {
		t.Fatalf("evaluation domain projection = %#v", evaluation.LifeGraphProjection.Primary)
	}
	if err := VerifyEvaluationRecordDigest(evaluation); err != nil {
		t.Fatalf("projection metadata changed evaluation digest: %v", err)
	}

	entities, err := graph.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) != 4 {
		t.Fatalf("life graph entity count = %d, want 4: %#v", len(entities), entities)
	}
	relations, err := graph.QueryRelations(context.Background(), "owner-1", lifeontology.RelationQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 4 {
		t.Fatalf("life graph relation count = %d, want 4: %#v", len(relations), relations)
	}
	otherOwner, err := graph.QueryEntities(context.Background(), "other-owner", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil || len(otherOwner) != 0 {
		t.Fatalf("cross-owner graph leak = %#v err=%v", otherOwner, err)
	}
}

func TestOutcomeProjectionFailureIsVisibleButCannotRollbackLedger(t *testing.T) {
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	service, err := WithLifeOntologyProjection(
		newService(NewMemoryRepository(), func() time.Time { return now }),
		projectionFunc(func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
			return lifeontology.OperationalProjectionResult{}, errors.New("projection failed api_key=do-not-expose-this")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	definition, created, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "projection-failure", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil || !created || definition.LifeGraphProjection != nil {
		t.Fatalf("authoritative write was rolled back: created=%v definition=%#v err=%v", created, definition, err)
	}
	if definition.LifeGraphProjectionWarning == "" || strings.Contains(definition.LifeGraphProjectionWarning, "do-not-expose-this") {
		t.Fatalf("projection warning is absent or leaked a secret: %q", definition.LifeGraphProjectionWarning)
	}
	stored, err := service.GetOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1")
	if err != nil || stored.Revision != 1 || stored.LifeGraphProjectionWarning != "" {
		t.Fatalf("authoritative ledger did not retain clean revision: %#v err=%v", stored, err)
	}
}

func TestEvaluationRepairsDefinitionThatPredatesLifeGraphProjection(t *testing.T) {
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	service := newService(repository, func() time.Time { return now })
	if _, _, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "pre-graph-definition", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	}); err != nil {
		t.Fatal(err)
	}
	graph := lifeontology.NewService(nil, func() time.Time { return now })
	service, err := WithLifeOntologyProjection(service, graph)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, created, err := service.CreateEvaluation(context.Background(), "owner-1", "workspace-1", "outcome-1", CreateEvaluationRequest{
		IdempotencyKey: "post-graph-evaluation", OutcomeRevision: 1,
		Observations: []Observation{
			observation("repair-obs-1", 12, testStart.Add(5*24*time.Hour)),
			observation("repair-obs-2", 16, testStart.Add(15*24*time.Hour)),
		},
		AsOf: testAsOf,
	})
	if err != nil || !created || evaluation.LifeGraphProjection == nil || evaluation.LifeGraphProjectionWarning != "" {
		t.Fatalf("evaluation did not repair definition projection: created=%v record=%#v err=%v", created, evaluation, err)
	}
	entities, err := graph.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 100})
	if err != nil || len(entities) != 4 {
		t.Fatalf("repaired graph entities = %d err=%v: %#v", len(entities), err, entities)
	}
}

func TestOutcomeProjectionRejectsAuthorityBearingProjectorResponse(t *testing.T) {
	service, err := WithLifeOntologyProjection(
		newService(NewMemoryRepository(), func() time.Time { return time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC) }),
		projectionFunc(func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
			return lifeontology.OperationalProjectionResult{AdvisoryOnly: false, CanExecute: true, GrantsAuthority: true}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	definition, created, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "projection-authority", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil || !created || definition.LifeGraphProjection != nil || !strings.Contains(definition.LifeGraphProjectionWarning, "authority boundary") {
		t.Fatalf("authority-bearing projection was accepted: created=%v definition=%#v err=%v", created, definition, err)
	}
}

func TestWithLifeOntologyProjectionValidatesDependencies(t *testing.T) {
	if _, err := WithLifeOntologyProjection(nil, projectionFunc(func(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
		return lifeontology.OperationalProjectionResult{}, nil
	})); err == nil {
		t.Fatal("nil outcome service was accepted")
	}
	if _, err := WithLifeOntologyProjection(NewService(NewMemoryRepository()), nil); err == nil {
		t.Fatal("nil projector was accepted")
	}
}
