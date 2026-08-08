package workflow

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresReminderCandidatesAreOwnerScopedAndExcludeClosedWork(t *testing.T) {
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres reminder query test")
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
	t.Cleanup(func() { _ = tx.Rollback().Error })

	repo := NewGormRepository(tx)
	reminderAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	create := func(owner, state, label string) uuid.UUID {
		item, createErr := repo.CreateItem(&models.WorkflowItem{
			ID:            uuid.New(),
			OwnerIdentity: owner,
			Title:         label,
			CurrentState:  state,
			TaskType:      "administrative",
			RiskLevel:     "low",
			AutonomyLevel: "manual",
		})
		if createErr != nil {
			t.Fatalf("create workflow: %v", createErr)
		}
		if _, createErr = repo.CreateChecklistItem(&models.WorkflowChecklistItem{
			ID:         uuid.New(),
			WorkflowID: item.ID,
			Label:      label,
			Status:     "open",
			ReminderAt: &reminderAt,
		}); createErr != nil {
			t.Fatalf("create reminder: %v", createErr)
		}
		return item.ID
	}
	wantedID := create("reminder-owner", StateReady, "owner reminder")
	create("foreign-owner", StateReady, "foreign reminder")
	create("reminder-owner", StateCompleted, "completed reminder")

	candidates, err := repo.FindReminderCandidatesForOwner(
		"reminder-owner", reminderAt.Add(time.Hour), 100,
	)
	if err != nil {
		t.Fatalf("find reminder candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Workflow.ID != wantedID ||
		candidates[0].Reminder.WorkflowID != wantedID {
		t.Fatalf("reminder candidates = %#v, want only owner workflow %s", candidates, wantedID)
	}
}

func TestPostgresReminderActivationRepositoryIsOwnerScopedReplayableAndLinear(t *testing.T) {
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres reminder activation test")
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
	t.Cleanup(func() { _ = tx.Rollback().Error })

	repo := NewGormRepository(tx)
	activationRepo, ok := repo.(reminderActivationRepository)
	if !ok {
		t.Fatal("Postgres workflow repository does not expose durable reminder activation storage")
	}
	owner := "activation-owner-" + uuid.NewString() + "@example.com"
	workflow, err := repo.CreateItem(&models.WorkflowItem{
		ID: uuid.New(), OwnerIdentity: owner, Title: "Review internal reminder",
		CurrentState: StateReady, TaskType: "administrative", RiskLevel: "low",
		AutonomyLevel: "manual", RequiresApproval: true, ApprovalStatus: "pending",
	})
	if err != nil {
		t.Fatalf("create activation workflow: %v", err)
	}
	reminderAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	checklist, err := repo.CreateChecklistItem(&models.WorkflowChecklistItem{
		ID: uuid.New(), WorkflowID: workflow.ID, Label: "Review reminder internally",
		Status: "open", RequiresApproval: true, ReminderAt: &reminderAt,
	})
	if err != nil {
		t.Fatalf("create activation checklist: %v", err)
	}
	if foreign, loadErr := activationRepo.LoadReminderActivationSourceForOwner("foreign-"+owner, checklist.ID); loadErr != nil || foreign != nil {
		t.Fatalf("foreign owner activation source = %#v, %v", foreign, loadErr)
	}
	source, err := activationRepo.LoadReminderActivationSourceForOwner(owner, checklist.ID)
	if err != nil || source == nil {
		t.Fatalf("load owner activation source = %#v, %v", source, err)
	}
	digest, err := reminderEvidenceDigest(*source)
	if err != nil {
		t.Fatalf("digest activation source: %v", err)
	}

	activationService := NewService(repo).(ReminderActivationService)
	prepareRequest := ReminderActivationPrepareRequest{
		ExpectedReminderDigest: digest,
		IdempotencyKey:         "postgres:activation:" + checklist.ID.String(),
		ActivationKind:         ReminderActivationKindInternal,
		Confirmation:           ReminderActivationPrepareConfirmation,
	}
	prepared, err := activationService.PrepareReminderActivationForOwner(owner, owner, checklist.ID, prepareRequest)
	if err != nil {
		t.Fatalf("prepare activation: %v", err)
	}
	replayed, err := activationService.PrepareReminderActivationForOwner(owner, owner, checklist.ID, prepareRequest)
	if err != nil || !replayed.Replayed || replayed.Request.ID != prepared.Request.ID {
		t.Fatalf("activation replay = %#v, %v", replayed, err)
	}

	approveRequest := ReminderActivationDecisionRequest{
		Decision: ReminderActivationDecisionApproved, Reason: "Owner reviewed the internal reminder.",
		Confirmation:                    ReminderActivationApproveConfirmation,
		ExpectedActivationRequestDigest: prepared.Request.RecordDigest,
	}
	approved, err := activationService.DecideReminderActivationForOwner(owner, owner, prepared.Request.ID, approveRequest)
	if err != nil {
		t.Fatalf("approve activation preparation: %v", err)
	}
	approvedReplay, err := activationService.DecideReminderActivationForOwner(owner, owner, prepared.Request.ID, approveRequest)
	if err != nil || !approvedReplay.Replayed || approvedReplay.Decision.ID != approved.Decision.ID {
		t.Fatalf("activation decision replay = %#v, %v", approvedReplay, err)
	}
	revoked, err := activationService.DecideReminderActivationForOwner(owner, owner, prepared.Request.ID, ReminderActivationDecisionRequest{
		Decision: ReminderActivationDecisionRevoked, Reason: "Owner revoked the internal preparation.",
		Confirmation:                    ReminderActivationRevokeConfirmation,
		ExpectedActivationRequestDigest: prepared.Request.RecordDigest,
		ExpectedPreviousDecisionID:      approved.Decision.ID.String(),
	})
	if err != nil || revoked.CanExecute || revoked.Decision.PreviousDecisionID == nil ||
		*revoked.Decision.PreviousDecisionID != approved.Decision.ID {
		t.Fatalf("activation revocation = %#v, %v", revoked, err)
	}
}

func TestPostgresReminderDeliveryReplayUsesStableEvidenceNotGeneratedRecordIdentity(t *testing.T) {
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres reminder delivery replay test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	repo := NewGormRepository(tx)
	owner := "delivery-owner-" + uuid.NewString() + "@example.com"
	workflow, err := repo.CreateItem(&models.WorkflowItem{
		ID: uuid.New(), OwnerIdentity: owner, Title: "Replay one internal reminder",
		CurrentState: StateReady, TaskType: "administrative", RiskLevel: "low", AutonomyLevel: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	reminderAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	checklist, err := repo.CreateChecklistItem(&models.WorkflowChecklistItem{
		ID: uuid.New(), WorkflowID: workflow.ID, Label: "Review reminder replay", Status: "open", ReminderAt: &reminderAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := repo.(reminderActivationRepository).LoadReminderActivationSourceForOwner(owner, checklist.ID)
	if err != nil || source == nil {
		t.Fatalf("source=%#v err=%v", source, err)
	}
	digest, err := reminderEvidenceDigest(*source)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewService(repo)
	activation := engine.(ReminderActivationService)
	delivery := engine.(ReminderDeliveryService)
	prepared, err := activation.PrepareReminderActivationForOwner(owner, owner, checklist.ID, ReminderActivationPrepareRequest{
		ExpectedReminderDigest: digest, IdempotencyKey: "postgres:delivery:prepare:" + checklist.ID.String(),
		ActivationKind: ReminderActivationKindInternal, Confirmation: ReminderActivationPrepareConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := activation.DecideReminderActivationForOwner(owner, owner, prepared.Request.ID, ReminderActivationDecisionRequest{
		Decision: ReminderActivationDecisionApproved, Reason: "Authorize one internal reminder.",
		Confirmation: ReminderActivationApproveConfirmation, ExpectedActivationRequestDigest: prepared.Request.RecordDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	authorizeRequest := ReminderDeliveryAuthorizeRequest{
		ExpectedActivationRequestDigest: prepared.Request.RecordDigest, ExpectedActivationDecisionDigest: approved.Decision.RecordDigest,
		ExpectedReminderDigest: digest, IdempotencyKey: "postgres:delivery:authorize:" + checklist.ID.String(),
		Channel: ReminderDeliveryChannelInApp, Confirmation: ReminderDeliveryAuthorizeConfirmation,
	}
	authorized, err := delivery.AuthorizeReminderDeliveryForOwner(owner, owner, prepared.Request.ID, authorizeRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := delivery.AuthorizeReminderDeliveryForOwner(owner, owner, prepared.Request.ID, authorizeRequest)
	if err != nil || !replayed.Replayed || replayed.Authorization.ID != authorized.Authorization.ID {
		t.Fatalf("authorization replay=%#v err=%v", replayed, err)
	}
	changedAuthorization := authorizeRequest
	changedAuthorization.IdempotencyKey = "postgres:delivery:changed:" + checklist.ID.String()
	if _, err = delivery.AuthorizeReminderDeliveryForOwner(owner, owner, prepared.Request.ID, changedAuthorization); err == nil {
		t.Fatal("one approved preparation must not create a second delivery authorization")
	}

	deliveryRepo := repo.(reminderDeliveryRepository)
	attempt := &models.WorkflowReminderDeliveryAttempt{
		ID: uuid.New(), AuthorizationID: authorized.Authorization.ID, OwnerIdentity: owner,
		AttemptNumber: 1, Status: ReminderDeliveryStatusRetryableFailure, Reason: "transient internal sink failure",
		ReminderDigest: digest, AuthorizationDigest: authorized.Authorization.RecordDigest,
		Authority: ReminderDeliveryAttemptAuthority, AttemptedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	attempt.RecordDigest, err = digestReminderActivationPayload(attempt)
	if err != nil {
		t.Fatal(err)
	}
	stored, created, err := deliveryRepo.SaveReminderDeliveryAttempt(attempt)
	if err != nil || !created {
		t.Fatalf("first receipt=%#v created=%v err=%v", stored, created, err)
	}
	retry := *attempt
	retry.ID = uuid.New()
	retry.AttemptedAt = retry.AttemptedAt.Add(time.Second)
	retry.RecordDigest, _ = digestReminderActivationPayload(&retry)
	replayedAttempt, created, err := deliveryRepo.SaveReminderDeliveryAttempt(&retry)
	if err != nil || created || replayedAttempt.ID != stored.ID {
		t.Fatalf("receipt replay=%#v created=%v err=%v", replayedAttempt, created, err)
	}
	conflict := retry
	conflict.ID = uuid.New()
	conflict.Status = ReminderDeliveryStatusSuppressed
	conflict.Reason = "different terminal evidence"
	conflict.RecordDigest, _ = digestReminderActivationPayload(&conflict)
	if _, _, err = deliveryRepo.SaveReminderDeliveryAttempt(&conflict); err == nil {
		t.Fatal("attempt-number reuse with different evidence must fail")
	}
}

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

func TestPostgresWorkflowApprovalDecisionLookupIsDurableAndOwnerScoped(t *testing.T) {
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres approval lookup test")
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
	alice, err := repo.CreateItem(&models.WorkflowItem{
		ID:               uuid.New(),
		OwnerIdentity:    "alice",
		Title:            "Alice durable approval",
		CurrentState:     StateReady,
		TaskType:         "administrative",
		RiskLevel:        "high",
		AutonomyLevel:    "approve_before_execute",
		RequiresApproval: true,
		ApprovalStatus:   "approved",
	})
	if err != nil {
		t.Fatalf("create Alice workflow: %v", err)
	}
	bob, err := repo.CreateItem(&models.WorkflowItem{
		ID:               uuid.New(),
		OwnerIdentity:    "bob",
		Title:            "Bob durable approval",
		CurrentState:     StateReady,
		TaskType:         "administrative",
		RiskLevel:        "high",
		AutonomyLevel:    "approve_before_execute",
		RequiresApproval: true,
		ApprovalStatus:   "approved",
	})
	if err != nil {
		t.Fatalf("create Bob workflow: %v", err)
	}

	digest := strings.Repeat("d", 64)
	binding := "automation-action:automation.docker.start:" + digest
	aliceDecision, err := repo.CreateDecision(&models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   alice.ID,
		DecisionType: "approval",
		Decision:     "approved",
		Reason:       "Alice approved the exact Docker action",
		RuleApplied:  binding,
		Approved:     true,
		Actor:        "alice",
	})
	if err != nil {
		t.Fatalf("create Alice decision: %v", err)
	}
	bobDecision, err := repo.CreateDecision(&models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   bob.ID,
		DecisionType: "approval",
		Decision:     "approved",
		Reason:       "Bob approved his exact Docker action",
		RuleApplied:  binding,
		Approved:     true,
		Actor:        "bob",
	})
	if err != nil {
		t.Fatalf("create Bob decision: %v", err)
	}
	rejectedDecision, err := repo.CreateDecision(&models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   alice.ID,
		DecisionType: "approval",
		Decision:     "rejected",
		Reason:       "Alice rejected this action",
		RuleApplied:  binding,
		Approved:     false,
		Actor:        "alice",
	})
	if err != nil {
		t.Fatalf("create rejected decision: %v", err)
	}

	record, err := repo.FindApprovalDecisionForOwner(
		context.Background(),
		"alice",
		aliceDecision.ID.String(),
	)
	if err != nil {
		t.Fatalf("find Alice decision: %v", err)
	}
	if record.DecisionID != aliceDecision.ID.String() ||
		record.WorkflowID != alice.ID.String() ||
		record.OwnerIdentity != "alice" ||
		record.ActionBinding != binding ||
		record.Actor != "alice" ||
		!record.Approved {
		t.Fatalf("Postgres approval projection = %#v", record)
	}

	for _, test := range []struct {
		name       string
		owner      string
		decisionID string
	}{
		{name: "Alice cannot read Bob decision", owner: "alice", decisionID: bobDecision.ID.String()},
		{name: "Bob cannot read Alice decision", owner: "bob", decisionID: aliceDecision.ID.String()},
		{name: "invented decision", owner: "alice", decisionID: uuid.NewString()},
	} {
		t.Run(test.name, func(t *testing.T) {
			found, lookupErr := repo.FindApprovalDecisionForOwner(
				context.Background(),
				test.owner,
				test.decisionID,
			)
			if found != nil || !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				t.Fatalf("lookup = %#v, %v, want nil record-not-found", found, lookupErr)
			}
		})
	}

	rejected, err := repo.FindApprovalDecisionForOwner(
		context.Background(),
		"alice",
		rejectedDecision.ID.String(),
	)
	if err != nil {
		t.Fatalf("find rejected decision: %v", err)
	}
	if rejected.Approved || rejected.Decision != "rejected" {
		t.Fatalf("rejected Postgres projection = %#v", rejected)
	}

}
