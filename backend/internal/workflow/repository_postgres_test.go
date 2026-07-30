package workflow

import (
	"os"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFrameworkSelectionProvenanceSurvivesPostgresRepositoryRoundTrip(t *testing.T) {
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres repository round-trip test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
	})

	repo := NewGormRepository(tx)
	item, err := repo.CreateItem(&models.WorkflowItem{
		ID:            uuid.New(),
		OwnerIdentity: "postgres-owner",
		Title:         "Framework selection provenance round trip",
		Description:   "Verify durable workflow observability.",
		CurrentState:  StateReady,
		TaskType:      "administrative",
		RiskLevel:     "low",
		AutonomyLevel: "manual",
	})
	if err != nil {
		t.Fatalf("create workflow item: %v", err)
	}
	selection := testFrameworkSelection("postgres-round-trip-plan")
	runResult := &TaskRunResult{
		PlanID:             selection.TaskPlanID,
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Passed:             true,
		FrameworkSelection: &selection,
	}
	engine := NewService(repo)
	implementation, ok := engine.(*service)
	if !ok {
		t.Fatalf("unexpected workflow service implementation %T", engine)
	}
	if err := implementation.storeTaskFrameworkSelection(item.ID, runResult); err != nil {
		t.Fatalf("store framework selection: %v", err)
	}

	decisions, err := repo.FindDecisions(item.ID)
	if err != nil {
		t.Fatalf("find decisions: %v", err)
	}
	decoded := frameworkSelectionsFromDecisions(decisions)
	if len(decoded) != 1 || decoded[0] != selection {
		t.Fatalf("Postgres decision round trip = %#v, want %#v", decoded, selection)
	}
	events, err := repo.FindEvents(item.ID)
	if err != nil {
		t.Fatalf("find events: %v", err)
	}
	foundEvent := false
	for _, event := range events {
		if event.EventType == frameworkSelectionEventType &&
			event.SourceURI == "framework-selection://"+selection.SelectionDecisionID {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Fatalf("Postgres framework selection event missing: %#v", events)
	}
	detail, err := engine.GetForOwner("postgres-owner", item.ID)
	if err != nil {
		t.Fatalf("get owner workflow: %v", err)
	}
	if len(detail.FrameworkSelections) != 1 || detail.FrameworkSelections[0] != selection {
		t.Fatalf("owner API detail provenance = %#v", detail.FrameworkSelections)
	}
	if _, err := engine.GetForOwner("foreign-owner", item.ID); err == nil {
		t.Fatalf("foreign owner could retrieve Postgres selection provenance")
	}
}
