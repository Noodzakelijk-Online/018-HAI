package ambientmonitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

type schedulerStub struct {
	scopes               []Scope
	recovered            int
	compositionRecovered int
	processed            int
	processError         error
}

func (s *schedulerStub) DueScopes(context.Context, time.Time, int) ([]Scope, error) {
	return append([]Scope(nil), s.scopes...), nil
}
func (s *schedulerStub) PendingCompositionScopes(context.Context, time.Time, int) ([]Scope, error) {
	return nil, nil
}
func (s *schedulerStub) RecoverExpiredLeases(context.Context, Scope, time.Time) (int, error) {
	s.recovered++
	return 0, nil
}
func (s *schedulerStub) RecoverExpiredCompositionLeases(context.Context, Scope, time.Time) (int, error) {
	s.compositionRecovered++
	return 0, nil
}
func (s *schedulerStub) ProcessDue(context.Context, ProcessDueRequest) (ProcessDueResult, error) {
	s.processed++
	return ProcessDueResult{Authority: advisoryAuthority()}, s.processError
}

func TestRunMonitorSweepProcessesEveryDueScope(t *testing.T) {
	stub := &schedulerStub{scopes: []Scope{{OwnerID: "owner-a", WorkspaceID: "workspace-a"}, {OwnerID: "owner-b", WorkspaceID: "workspace-b"}}}
	if err := runMonitorSweep(t.Context(), stub, time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if stub.recovered != 2 || stub.compositionRecovered != 2 || stub.processed != 2 {
		t.Fatalf("recovered=%d compositionRecovered=%d processed=%d, want 2/2/2", stub.recovered, stub.compositionRecovered, stub.processed)
	}
}

func TestRunMonitorSweepReturnsSanitizedAggregateFailure(t *testing.T) {
	stub := &schedulerStub{scopes: []Scope{{OwnerID: "owner-a", WorkspaceID: "workspace-a"}}, processError: errors.New("Authorization: Bearer secret")}
	err := runMonitorSweep(t.Context(), stub, time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC), func() bool { return true })
	if err == nil || err.Error() != "ambient outcome sweep failed for 1 scoped batch(es)" {
		t.Fatalf("error = %v", err)
	}
}

func TestRunMonitorSweepStopsWhenPermissionIsWithdrawnMidSweep(t *testing.T) {
	stub := &schedulerStub{scopes: []Scope{
		{OwnerID: "owner-a", WorkspaceID: "workspace-a"},
		{OwnerID: "owner-b", WorkspaceID: "workspace-b"},
	}}
	allowedChecks := 0
	allowed := func() bool {
		allowedChecks++
		return stub.processed == 0
	}
	if err := runMonitorSweep(t.Context(), stub, time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC), allowed); err != nil {
		t.Fatal(err)
	}
	if stub.processed != 1 || stub.recovered != 1 || stub.compositionRecovered != 1 {
		t.Fatalf("processed=%d recovered=%d compositionRecovered=%d, want 1/1/1 after permission withdrawal", stub.processed, stub.recovered, stub.compositionRecovered)
	}
	if allowedChecks < 3 {
		t.Fatalf("allowed checks = %d, want checks before and during scoped work", allowedChecks)
	}
}

func TestMonitorSchedulerEnvironmentBounds(t *testing.T) {
	t.Setenv("OUTCOME_MONITOR_BATCH_LIMIT", "999")
	t.Setenv("OUTCOME_MONITOR_POLL_SECONDS", "0")
	t.Setenv("OUTCOME_MONITOR_SCHEDULER_ENABLED", "off")
	if monitorBatchLimit() != 20 || monitorPollInterval() != 15*time.Second || DurableSchedulerEnabled() {
		t.Fatal("invalid scheduler environment did not fall back safely")
	}
}
