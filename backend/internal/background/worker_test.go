package background

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"automation-hub-backend/internal/accountfeed"
	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/operations"

	"github.com/google/uuid"
)

func buildWorker(t *testing.T, mode autonomypolicy.Mode, feedJSON string) (*Worker, *operations.Service, string) {
	t.Helper()
	feedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(feedDir, "feed.json"), []byte(feedJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	feed := accountfeed.Feed{
		ID:           uuid.New(),
		Name:         "inbox",
		Provider:     "local",
		AccountLabel: "primary",
		SourceType:   accountfeed.SourceLocalJSONFile,
		Path:         "feed.json",
		OwnerUserID:  "user-1",
		WorkspaceID:  "local",
		Enabled:      true,
	}
	reader, err := accountfeed.NewLocalFileReader(feed, feedDir)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	svc := operations.NewService(operations.NewMemoryRepository())
	workspace := t.TempDir()
	broker := executionbroker.NewBroker(workspace)
	w := New(svc, broker, []accountfeed.Reader{reader}, Options{
		OwnerUserID: "user-1", WorkspaceID: "local", Mode: mode,
	})
	return w, svc, workspace
}

const twoItemFeed = `[
  {"externalId":"a1","title":"Organize workspace notes","body":"Consolidate personal notes into a local file"},
  {"externalId":"a2","title":"Pay invoice to landlord","body":"Send payment for the rent invoice"}
]`

func TestRunOnceVerticalSlice(t *testing.T) {
	w, svc, workspace := buildWorker(t, autonomypolicy.ModeAutonomousSafe, twoItemFeed)
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if rep.OperationsCreated != 2 {
		t.Fatalf("want 2 operations created, got %d (errors: %v)", rep.OperationsCreated, rep.Errors)
	}
	if rep.AutoExecuted != 1 || rep.Verified != 1 {
		t.Fatalf("want one auto-executed+verified operation, got auto=%d verified=%d", rep.AutoExecuted, rep.Verified)
	}
	if rep.AwaitingApproval != 1 {
		t.Fatalf("want the high-risk operation awaiting approval, got %d", rep.AwaitingApproval)
	}

	// The safe worker must have actually written an artifact into the workspace.
	entries, _ := os.ReadDir(workspace)
	if len(entries) == 0 {
		t.Fatalf("safe worker did not create any artifact in the workspace")
	}

	// The completed operation must be verification-passed.
	completed, err := svc.List(operations.Filter{OwnerUserID: "user-1", WorkspaceID: "local", Status: operations.StatusCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 {
		t.Fatalf("want 1 completed operation, got %d", len(completed))
	}
	if completed[0].VerificationStatus != string(operations.VerificationPassed) {
		t.Fatalf("completed operation must be verification-passed, got %q", completed[0].VerificationStatus)
	}
	if completed[0].RuntimeID != executionbroker.LocalSafeWorkerID {
		t.Fatalf("completed operation must record the runtime, got %q", completed[0].RuntimeID)
	}

	// Audit trail exists.
	events, _ := svc.Events(completed[0].ID)
	if len(events) < 2 {
		t.Fatalf("completed operation must have an audit trail, got %d events", len(events))
	}
}

func TestFastTriageLaneStampsOperations(t *testing.T) {
	w, svc, _ := buildWorker(t, autonomypolicy.ModeAutonomousSafe, twoItemFeed)
	mi := modelintelligence.NewService(modelintelligence.NewRegistryFromEnv())
	w.WithModelIntelligence(mi)

	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Triaged != 2 {
		t.Fatalf("both operations must be triaged by the fast-triage lane, got %d", rep.Triaged)
	}
	// The lane must have stamped the model provider on the operation record.
	ops, _ := svc.List(operations.Filter{OwnerUserID: "user-1", WorkspaceID: "local"})
	for _, op := range ops {
		if op.ModelProviderID != modelintelligence.ProviderTestFastTriage {
			t.Fatalf("operation %s missing triage model provider, got %q", op.ID, op.ModelProviderID)
		}
	}
	// The lane must have produced real telemetry surfaced by model intelligence.
	if len(mi.Telemetry()) < 2 {
		t.Fatalf("fast-triage lane must record telemetry, got %d rows", len(mi.Telemetry()))
	}
	if len(mi.LaneWinners()) == 0 {
		t.Fatalf("triage runs must yield a fast-triage lane winner")
	}
}

func TestRunOnceIsIdempotent(t *testing.T) {
	w, _, _ := buildWorker(t, autonomypolicy.ModeAutonomousSafe, twoItemFeed)
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OperationsCreated != 0 {
		t.Fatalf("second pass must not create duplicate operations, created %d", rep.OperationsCreated)
	}
}

func TestEmergencyStopProcessesNothing(t *testing.T) {
	w, svc, _ := buildWorker(t, autonomypolicy.ModeAutonomousSafe, twoItemFeed)
	w.opts.EmergencyStop = true
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OperationsCreated != 2 {
		t.Fatalf("emergency stop still records ingested items, got %d", rep.OperationsCreated)
	}
	if rep.AutoExecuted != 0 || rep.Classified != 0 {
		t.Fatalf("emergency stop must not process operations, auto=%d classified=%d", rep.AutoExecuted, rep.Classified)
	}
	// Everything stays in `new`.
	news, _ := svc.List(operations.Filter{OwnerUserID: "user-1", WorkspaceID: "local", Status: operations.StatusNew})
	if len(news) != 2 {
		t.Fatalf("emergency stop must leave operations in `new`, got %d", len(news))
	}
}

func TestReadOnlyModeObservesOnly(t *testing.T) {
	w, svc, _ := buildWorker(t, autonomypolicy.ModeReadOnly, twoItemFeed)
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.AutoExecuted != 0 {
		t.Fatalf("read-only mode must not execute, got auto=%d", rep.AutoExecuted)
	}
	// Low-risk item is observed (stays classified), high-risk still needs approval.
	classified, _ := svc.List(operations.Filter{OwnerUserID: "user-1", WorkspaceID: "local", Status: operations.StatusClassified})
	if len(classified) == 0 {
		t.Fatalf("read-only mode should classify+observe low-risk items")
	}
}
