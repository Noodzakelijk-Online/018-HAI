package planningoptimizer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/models"
)

type memoryRepository struct {
	records []models.OptimizationProposalRun
}

func (r *memoryRepository) Create(run *models.OptimizationProposalRun) (*models.OptimizationProposalRun, error) {
	r.records = append(r.records, *run)
	return run, nil
}
func (r *memoryRepository) List(owner string, _ int) ([]models.OptimizationProposalRun, error) {
	items := []models.OptimizationProposalRun{}
	for _, record := range r.records {
		if record.OwnerIdentity == owner {
			items = append(items, record)
		}
	}
	return items, nil
}

func TestProposeUsesBoundedLocalSolverAndPersistsAudit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/schedule" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected solver request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"optimal","solver":"or-tools-cp-sat 9.15.6755","scheduled":[{"id":"high","startMinute":540,"endMinute":600,"priority":90}],"deferred":["low"],"objectiveValue":900000,"assumptions":["one lane","proposal only"]}`))
	}))
	defer server.Close()
	repo := &memoryRepository{}
	service := NewService(repo, true, server.URL, 0)
	run, err := service.Propose(context.Background(), "owner-a", Request{DayStartMinute: 540, DayEndMinute: 600, Jobs: []Job{{ID: "high", DurationMinutes: 60, Priority: 90}, {ID: "low", DurationMinutes: 60, Priority: 10}}})
	if err != nil || run.Status != "optimal" || len(run.Result.Scheduled) != 1 || len(repo.records) != 1 {
		t.Fatalf("unexpected proposal: run=%#v err=%v records=%d", run, err, len(repo.records))
	}
	if runs, err := service.Runs("owner-a", 10); err != nil || len(runs) != 1 || runs[0].RequestDigest == "" {
		t.Fatalf("owner-scoped audit was not available: %#v %v", runs, err)
	}
}

func TestOptimizerFailsClosedForExternalOrDisabledService(t *testing.T) {
	repo := &memoryRepository{}
	unsafe := NewService(repo, true, "https://example.com", 0)
	if unsafe.Status().Configured || unsafe.Status().ConfigError == "" {
		t.Fatalf("external solver must be rejected: %#v", unsafe.Status())
	}
	disabled := NewService(repo, false, "http://127.0.0.1:8080", 0)
	run, err := disabled.Propose(context.Background(), "owner-a", Request{DayStartMinute: 540, DayEndMinute: 600, Jobs: []Job{{ID: "job", DurationMinutes: 60, Priority: 50}}})
	if err != ErrNotConfigured || run == nil || run.Status != "not_configured" {
		t.Fatalf("disabled optimizer must not contact a solver: run=%#v err=%v", run, err)
	}
}

func TestRequestValidationRejectsUnsafeIdentifiersAndWindows(t *testing.T) {
	if err := validateRequest(Request{DayStartMinute: 540, DayEndMinute: 600, Jobs: []Job{{ID: "email@example.test", DurationMinutes: 60, Priority: 50}}}); err == nil {
		t.Fatalf("non-opaque ID must be rejected")
	}
	if err := validateRequest(Request{DayStartMinute: 540, DayEndMinute: 600, Jobs: []Job{{ID: "late", DurationMinutes: 60, Priority: 50, EarliestMinute: ptr(550), LatestEndMinute: ptr(600)}}}); err == nil {
		t.Fatalf("non-fitting window must be rejected")
	}
}

func TestProposalValidationRejectsSolverOutputThatDoesNotMatchInput(t *testing.T) {
	request := Request{DayStartMinute: 540, DayEndMinute: 600, Jobs: []Job{{ID: "job", DurationMinutes: 60, Priority: 50}}}
	valid := Proposal{Status: "optimal", Solver: "or-tools", Scheduled: []ScheduledJob{{ID: "job", StartMinute: 540, EndMinute: 600, Priority: 50}}, Assumptions: []string{"proposal only"}}
	if !validProposal(valid, request) {
		t.Fatal("expected matching local solver output to be accepted")
	}
	valid.Scheduled[0].EndMinute = 599
	if validProposal(valid, request) {
		t.Fatal("mismatched duration must be rejected")
	}
	valid.Scheduled[0].EndMinute = 600
	valid.Scheduled[0].ID = "unknown"
	if validProposal(valid, request) {
		t.Fatal("unknown solver job must be rejected")
	}
}

func ptr(v int) *int { return &v }
