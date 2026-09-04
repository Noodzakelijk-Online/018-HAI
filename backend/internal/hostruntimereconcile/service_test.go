package hostruntimereconcile

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/hostruntime"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestReconcileCompletedAppendsIdempotentTerminalEvent(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	automationID := uuid.New()
	jobID := uuid.New()
	exitCode := 0
	host := &fakeHostJobs{jobs: []hostruntime.Job{{
		ID: jobID, OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness",
		TaskID: "automation:" + automationID.String() + ":intent:" + uuid.NewString(), Status: hostruntime.StatusCompleted,
		Output: "completed without exposed secrets", ExitCode: &exitCode, CreatedAt: now.Add(-time.Minute), CompletedAt: &now,
	}}}
	ledger := &fakeLedger{launch: &models.AutomationLaunchEvent{
		AutomationID: automationID, RuntimeType: "deepseek-harness", RuntimeTaskID: host.jobs[0].TaskID,
		ExecutionReference: jobID.String(), Status: "queued",
	}, automation: &models.Automation{ID: automationID}}
	service := NewService(host, ledger)
	service.now = func() time.Time { return now }

	count, err := service.ReconcileCompleted(10)
	if err != nil || count != 1 {
		t.Fatalf("ReconcileCompleted = %d, %v", count, err)
	}
	if len(ledger.events) != 1 || ledger.events[0].Status != "completed" || ledger.events[0].EventKey != completionEventPrefix+jobID.String() {
		t.Fatalf("terminal ledger event = %#v", ledger.events)
	}
	if ledger.automation.LastSuccessAt == nil || !ledger.automation.LastSuccessAt.Equal(now) || !host.reconciled[jobID] {
		t.Fatalf("completion projection was not finalized: automation=%#v reconciled=%#v", ledger.automation, host.reconciled)
	}

	if count, err := service.ReconcileCompleted(10); err != nil || count != 0 || len(ledger.events) != 1 {
		t.Fatalf("reconciliation was not idempotent: count=%d err=%v events=%#v", count, err, ledger.events)
	}
}

func TestReconcileCompletedKeepsUnlinkedHostJobVisible(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	jobID := uuid.New()
	exitCode := 0
	host := &fakeHostJobs{jobs: []hostruntime.Job{{ID: jobID, RuntimeID: "deepseek-harness", Status: hostruntime.StatusCompleted, ExitCode: &exitCode, CompletedAt: &now}}}
	ledger := &fakeLedger{findErr: errTest("queued launch missing")}
	service := NewService(host, ledger)
	if _, err := service.ReconcileCompleted(1); err == nil || !strings.Contains(err.Error(), "queued launch missing") {
		t.Fatalf("unlinked job error = %v", err)
	}
	if host.reconciled[jobID] {
		t.Fatal("unlinked host job was silently marked reconciled")
	}
}

func TestReconcileCompletedProjectsFailureWithoutLeakingSecrets(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	automationID := uuid.New()
	jobID := uuid.New()
	exitCode := 23
	taskID := "automation:" + automationID.String() + ":intent:" + uuid.NewString()
	host := &fakeHostJobs{jobs: []hostruntime.Job{{
		ID: jobID, OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: taskID,
		Status: hostruntime.StatusCompleted, ExitCode: &exitCode, Error: "Authorization: Bearer secret-value", CompletedAt: &now,
	}}}
	ledger := &fakeLedger{launch: &models.AutomationLaunchEvent{
		AutomationID: automationID, RuntimeType: "deepseek-harness", RuntimeTaskID: taskID, ExecutionReference: jobID.String(),
	}, automation: &models.Automation{ID: automationID}}
	service := NewService(host, ledger)
	if count, err := service.ReconcileCompleted(1); err != nil || count != 1 {
		t.Fatalf("ReconcileCompleted = %d, %v", count, err)
	}
	if ledger.automation.LastFailureAt == nil || strings.Contains(ledger.automation.LastFailureReason, "secret-value") {
		t.Fatalf("failure projection was not safely recorded: %#v", ledger.automation)
	}
	if len(ledger.events) != 1 || ledger.events[0].Status != "failed" || strings.Contains(ledger.events[0].Message, "secret-value") {
		t.Fatalf("failed event leaked or had wrong status: %#v", ledger.events)
	}
}

func TestReconcileCompletedDoesNotLetAnUnlinkedJobBlockLaterCompletion(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	automationID := uuid.New()
	missingID, linkedID := uuid.New(), uuid.New()
	exitCode := 0
	linkedTaskID := "automation:" + automationID.String() + ":intent:" + uuid.NewString()
	host := &fakeHostJobs{jobs: []hostruntime.Job{
		{ID: missingID, RuntimeID: "deepseek-harness", Status: hostruntime.StatusCompleted, ExitCode: &exitCode, CompletedAt: &now},
		{ID: linkedID, OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: linkedTaskID, Status: hostruntime.StatusCompleted, ExitCode: &exitCode, CompletedAt: &now},
	}}
	ledger := &multiLaunchLedger{launches: map[string]*models.AutomationLaunchEvent{
		linkedID.String(): {AutomationID: automationID, RuntimeType: "deepseek-harness", RuntimeTaskID: linkedTaskID, ExecutionReference: linkedID.String()},
	}, automation: &models.Automation{ID: automationID}}
	service := NewService(host, ledger)
	if count, err := service.ReconcileCompleted(10); count != 1 || err == nil {
		t.Fatalf("ReconcileCompleted = %d, %v; want one success plus retained error", count, err)
	}
	if host.reconciled[missingID] || !host.reconciled[linkedID] || len(ledger.events) != 1 {
		t.Fatalf("batch continuation failed: reconciled=%#v events=%#v", host.reconciled, ledger.events)
	}
}

type fakeHostJobs struct {
	jobs       []hostruntime.Job
	reconciled map[uuid.UUID]bool
}

func (f *fakeHostJobs) CompletedUnreconciled(limit int) ([]hostruntime.Job, error) {
	if f.reconciled == nil {
		f.reconciled = map[uuid.UUID]bool{}
	}
	jobs := make([]hostruntime.Job, 0)
	for _, job := range f.jobs {
		if !f.reconciled[job.ID] {
			jobs = append(jobs, job)
		}
		if limit > 0 && len(jobs) >= limit {
			break
		}
	}
	return jobs, nil
}

func (f *fakeHostJobs) MarkReconciled(id uuid.UUID) (bool, error) {
	if f.reconciled == nil {
		f.reconciled = map[uuid.UUID]bool{}
	}
	if f.reconciled[id] {
		return false, nil
	}
	f.reconciled[id] = true
	return true, nil
}

type fakeLedger struct {
	launch     *models.AutomationLaunchEvent
	automation *models.Automation
	events     []models.AutomationLaunchEvent
	findErr    error
}

func (f *fakeLedger) FindLaunchEventByExecutionReference(string) (*models.AutomationLaunchEvent, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.launch, nil
}

func (f *fakeLedger) FindByID(uuid.UUID) (*models.Automation, error) { return f.automation, nil }
func (f *fakeLedger) Update(automation *models.Automation) (*models.Automation, error) {
	f.automation = automation
	return automation, nil
}
func (f *fakeLedger) SaveLaunchEvent(event *models.AutomationLaunchEvent) error {
	for _, existing := range f.events {
		if existing.EventKey == event.EventKey {
			return nil
		}
	}
	f.events = append(f.events, *event)
	return nil
}

type multiLaunchLedger struct {
	launches   map[string]*models.AutomationLaunchEvent
	automation *models.Automation
	events     []models.AutomationLaunchEvent
}

func (f *multiLaunchLedger) FindLaunchEventByExecutionReference(reference string) (*models.AutomationLaunchEvent, error) {
	launch, ok := f.launches[reference]
	if !ok {
		return nil, errTest("queued launch missing")
	}
	return launch, nil
}
func (f *multiLaunchLedger) FindByID(uuid.UUID) (*models.Automation, error) { return f.automation, nil }
func (f *multiLaunchLedger) Update(automation *models.Automation) (*models.Automation, error) {
	f.automation = automation
	return automation, nil
}
func (f *multiLaunchLedger) SaveLaunchEvent(event *models.AutomationLaunchEvent) error {
	f.events = append(f.events, *event)
	return nil
}

type errTest string

func (e errTest) Error() string { return string(e) }
