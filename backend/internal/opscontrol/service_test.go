package opscontrol

import (
	"context"
	"testing"

	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/operations"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	broker := executionbroker.NewBroker(t.TempDir())
	ops := operations.NewService(operations.NewMemoryRepository())
	return NewService(t.TempDir(), broker, ops, "local-operator", "local")
}

func TestEmergencyStopEngageDisengageAndControl(t *testing.T) {
	s := newTestService(t)
	if s.Control().EmergencyStop() {
		t.Fatalf("emergency stop must default to disengaged")
	}
	s.EngageEmergencyStop("test", "op")
	if !s.Control().EmergencyStop() {
		t.Fatalf("engage must set the stop")
	}
	// Engaged mode is emergency_stopped regardless of stored mode.
	if s.Control().Mode() != autonomypolicy.ModeEmergencyStopped {
		t.Fatalf("engaged control mode must be emergency_stopped, got %s", s.Control().Mode())
	}
	s.DisengageEmergencyStop("op")
	if s.Control().EmergencyStop() {
		t.Fatalf("disengage must clear the stop")
	}
}

func TestEmergencyStopSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	broker := executionbroker.NewBroker(t.TempDir())
	ops := operations.NewService(operations.NewMemoryRepository())
	s1 := NewService(dir, broker, ops, "u", "local")
	s1.EngageEmergencyStop("halt for maintenance", "op")

	// A fresh service rooted at the same dir must load the persisted stop.
	s2 := NewService(dir, broker, ops, "u", "local")
	if !s2.Control().EmergencyStop() {
		t.Fatalf("emergency stop must survive restart (persisted)")
	}
	if s2.Control().EmergencyState().Reason != "halt for maintenance" {
		t.Fatalf("persisted reason must survive restart")
	}
}

func TestVerifyEmergencyStopHaltsProcessing(t *testing.T) {
	s := newTestService(t)
	processedCalls := 0
	// A runner that would "process" 3 ops if it ran unguarded. The verify wraps
	// it with the emergency stop engaged; a correct system must report the
	// runner's honest processed count — which the caller's worker forces to 0.
	// Here we simulate a compliant worker: it checks the stop and returns 0.
	s.SetBackgroundRunner(func(ctx context.Context) (int, error) {
		processedCalls++
		if s.Control().EmergencyStop() {
			return 0, nil // compliant: halted
		}
		return 3, nil
	})
	v, err := s.VerifyEmergencyStop(context.Background())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !v.Halted || v.ProcessedDuringStop != 0 {
		t.Fatalf("emergency stop must halt processing, got halted=%v processed=%d", v.Halted, v.ProcessedDuringStop)
	}
	if processedCalls != 1 {
		t.Fatalf("verify must run the background pass exactly once")
	}
	// Prior state (disengaged) must be restored.
	if s.Control().EmergencyStop() {
		t.Fatalf("verify must restore the prior disengaged state")
	}
}

func TestVerifyDetectsNonHaltingStop(t *testing.T) {
	s := newTestService(t)
	// A broken worker that ignores the emergency stop.
	s.SetBackgroundRunner(func(ctx context.Context) (int, error) { return 5, nil })
	v, err := s.VerifyEmergencyStop(context.Background())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if v.Halted {
		t.Fatalf("a non-halting stop must be reported as NOT halted")
	}
}

func TestRecoveryReconcilesStuckOperations(t *testing.T) {
	ops := operations.NewService(operations.NewMemoryRepository())
	broker := executionbroker.NewBroker(t.TempDir())
	s := NewService(t.TempDir(), broker, ops, "u", "local")

	// Create an operation stuck in `running` (as if the process crashed).
	in := operations.NewOperationInput{OwnerUserID: "u", WorkspaceID: "local", Title: "stuck", OperationType: "t", SourceType: "runtime_lab", DedupeKey: "d1"}
	ing, _ := ops.Ingest(in)
	op := ing.Operation
	op.CurrentDecision = string(operations.DecisionRunSafeLocalWorker)
	cl, _ := ops.Transition(op, operations.StatusClassified, "hai", "", "")
	rdy, _ := ops.Transition(*cl, operations.StatusReady, "hai", "", "")
	run, _ := ops.Transition(*rdy, operations.StatusRunning, "hai", "", "")
	_ = run

	rep := s.Recover(context.Background())
	if rep.ScannedRunning != 1 || rep.Recovered != 1 {
		t.Fatalf("recovery must move the stuck running op to interrupted, got %+v", rep)
	}
	interrupted, _ := ops.List(operations.Filter{OwnerUserID: "u", WorkspaceID: "local", Status: operations.StatusInterrupted})
	if len(interrupted) != 1 {
		t.Fatalf("stuck op must be recovered to interrupted")
	}
}

func TestReadinessIsTruthfulOffWindows(t *testing.T) {
	s := newTestService(t)
	r := s.Readiness(context.Background())
	if r.IsWindows {
		if r.OperatingSystem != "windows" {
			t.Fatalf("isWindows must match OS")
		}
	} else {
		// Windows gate must be pending, not passed, off-Windows.
		var winGate *ReadinessGate
		for i := range r.Gates {
			if r.Gates[i].Name == "windows_version_build" {
				winGate = &r.Gates[i]
			}
		}
		if winGate == nil || winGate.Status != GatePending {
			t.Fatalf("windows gate must be pending off-Windows")
		}
		if !r.TargetVerifyPending {
			t.Fatalf("off-Windows readiness must flag target-machine verification pending")
		}
	}
	// Docker is never required.
	if r.Docker.Required {
		t.Fatalf("docker must not be marked required")
	}
	// The safe worker gate should pass (broker has a workspace).
	for _, g := range r.Gates {
		if g.Name == "local_safe_worker_run" && g.Status != GatePass {
			t.Fatalf("safe worker gate should pass with a configured workspace")
		}
	}
}

func TestSetModeValidates(t *testing.T) {
	s := newTestService(t)
	if _, err := s.SetMode("not_a_mode"); err == nil {
		t.Fatalf("invalid mode must be rejected")
	}
	if m, err := s.SetMode(string(autonomypolicy.ModeReadOnly)); err != nil || m != string(autonomypolicy.ModeReadOnly) {
		t.Fatalf("valid mode must be accepted, got %s err=%v", m, err)
	}
}
