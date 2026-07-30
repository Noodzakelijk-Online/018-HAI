package standingmandate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestMandatePersistenceRoundTrip(t *testing.T) {
	now := time.Date(2026, time.July, 30, 13, 0, 0, 123000000, time.UTC)
	expires := now.Add(8 * time.Hour)
	value := StandingMandate{
		ID:              uuid.New(),
		OwnerIdentity:   "robert",
		Name:            "Bounded local administration",
		Purpose:         "Execute one reviewed task class.",
		Status:          StatusDraft,
		Version:         "2.1.0",
		Revision:        1,
		AutonomyCeiling: 6,
		Scopes: []Scope{{
			ID:      "tasks",
			Actions: []string{"task.update"},
			Resources: []ResourceScope{{
				Type: "task",
				IDs:  []string{"task-1"},
			}},
			Projects:    []string{"hai"},
			Domains:     []string{"work"},
			Tools:       []string{"local-worker"},
			MaximumRisk: RiskMedium,
		}},
		ApprovalPolicy: ApprovalPolicy{
			Mode:                      ApprovalAtOrAboveAutonomy,
			AutonomyThreshold:         5,
			ApproverRoles:             []string{"owner"},
			MaximumEvidenceAgeSeconds: 300,
		},
		StopConditions: []StopCondition{{
			ID:            "stop",
			Description:   "Stop flag is active.",
			FactKey:       "stop",
			Operator:      StopEquals,
			ExpectedValue: "true",
			Required:      true,
			Effect:        StopDeny,
		}},
		SourceReferences: []string{"policy:v2"},
		CreatedBy:        "robert",
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        &expires,
	}

	row, err := mandateToModel(value)
	if err != nil {
		t.Fatalf("mandateToModel: %v", err)
	}
	roundTrip, err := mandateFromModel(row)
	if err != nil {
		t.Fatalf("mandateFromModel: %v", err)
	}
	if roundTrip.ID != value.ID ||
		roundTrip.OwnerIdentity != value.OwnerIdentity ||
		roundTrip.Version != value.Version ||
		roundTrip.AutonomyCeiling != value.AutonomyCeiling ||
		len(roundTrip.Scopes) != 1 ||
		roundTrip.Scopes[0].Resources[0].IDs[0] != "task-1" ||
		roundTrip.ApprovalPolicy.Mode != ApprovalAtOrAboveAutonomy ||
		len(roundTrip.StopConditions) != 1 ||
		len(roundTrip.SourceReferences) != 1 ||
		roundTrip.ExpiresAt == nil ||
		!roundTrip.ExpiresAt.Equal(expires) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", roundTrip, value)
	}
}

func TestPersistenceConversionRejectsInvalidJSONAndMismatchedDecisionEvidence(t *testing.T) {
	row := models.StandingMandate{
		ID:                   uuid.New(),
		OwnerIdentity:        "robert",
		Status:               string(StatusDraft),
		Revision:             1,
		AutonomyCeiling:      1,
		ScopesJSON:           `{}`,
		ApprovalPolicyJSON:   `{}`,
		StopConditionsJSON:   `[]`,
		SourceReferencesJSON: `[]`,
	}
	if _, err := mandateFromModel(row); err == nil ||
		!strings.Contains(err.Error(), "JSON array") {
		t.Fatalf("invalid scope JSON error = %v", err)
	}

	decision := AuthorizationDecision{
		ID:                uuid.New(),
		MandateID:         uuid.New(),
		OwnerIdentity:     "robert",
		ActorIdentity:     "worker",
		Action:            "task.update",
		Outcome:           DecisionDenied,
		Reason:            "test",
		EffectiveAutonomy: 1,
		EvaluatedAt:       time.Now().UTC(),
		Evidence: DecisionEvidence{
			RequestDigest:   strings.Repeat("a", 64),
			MandateDigest:   strings.Repeat("b", 64),
			DecisionDigest:  strings.Repeat("c", 64),
			MandateRevision: 1,
			Trace:           []DecisionTrace{},
		},
	}
	decisionRow, err := decisionToModel(decision)
	if err != nil {
		t.Fatalf("decisionToModel: %v", err)
	}
	decisionRow.RequestDigest = strings.Repeat("d", 64)
	if _, err := decisionFromModel(decisionRow); err == nil ||
		!strings.Contains(err.Error(), "do not match") {
		t.Fatalf("mismatched evidence error = %v", err)
	}
}

func TestAuthorizationDecisionIsDurableAndOwnerScopedInRepositoryContract(t *testing.T) {
	repository := NewMemoryRepository()
	now := time.Date(2026, time.July, 30, 14, 0, 0, 0, time.UTC)
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	mandate := activateTestMandate(t, service, now)

	decision, err := service.Authorize(context.Background(), mandate.ID, validAction(now))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	stored, err := service.GetDecision(context.Background(), "robert", decision.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if stored.Evidence.DecisionDigest != decision.Evidence.DecisionDigest ||
		stored.MandateID != mandate.ID {
		t.Fatalf("stored decision = %#v, want %#v", stored, decision)
	}
	if _, err := service.GetDecision(
		context.Background(),
		"other-owner",
		decision.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner decision read error = %v, want ErrNotFound", err)
	}
	decisions, err := service.ListDecisions(
		context.Background(),
		"robert",
		&mandate.ID,
		10,
	)
	if err != nil || len(decisions) != 1 {
		t.Fatalf("ListDecisions = %#v, %v", decisions, err)
	}
	decisions[0].Evidence.Trace[0].Message = "mutated"
	again, err := service.GetDecision(context.Background(), "robert", decision.ID)
	if err != nil {
		t.Fatalf("GetDecision again: %v", err)
	}
	if again.Evidence.Trace[0].Message == "mutated" {
		t.Fatal("repository returned mutable decision evidence")
	}
}
