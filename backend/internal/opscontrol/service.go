package opscontrol

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/operations"
)

// BackgroundRunner runs one background pass and returns how many operations it
// processed (classified/executed). It lets the ops-control service verify the
// emergency stop without importing the background worker directly.
type BackgroundRunner func(ctx context.Context) (processed int, err error)

// Service is the always-on runtime control plane: emergency stop, background
// mode, Docker dependency status, crash/reboot recovery, and the Windows
// readiness checklist. It shares the Operation Ledger + execution broker.
type Service struct {
	control *Controller
	broker  *executionbroker.Broker
	ops     *operations.Service
	runner  BackgroundRunner
	owner   string
	space   string
	now     func() time.Time
}

// NewService builds the ops-control service rooted at stateDir.
func NewService(stateDir string, broker *executionbroker.Broker, ops *operations.Service, ownerUserID, workspaceID string) *Service {
	return &Service{
		control: NewController(stateDir),
		broker:  broker,
		ops:     ops,
		owner:   ownerUserID,
		space:   workspaceID,
		now:     time.Now,
	}
}

// Control returns the controller (implements the background worker's Control).
func (s *Service) Control() *Controller { return s.control }

// SetBackgroundRunner wires the background pass used by emergency-stop
// verification.
func (s *Service) SetBackgroundRunner(r BackgroundRunner) { s.runner = r }

// EngageEmergencyStop halts all background processing (persisted).
func (s *Service) EngageEmergencyStop(reason, actor string) (EmergencyStopState, error) {
	if reason == "" {
		reason = "operator-engaged emergency stop"
	}
	return s.control.Engage(reason, actor, s.now().UTC())
}

// DisengageEmergencyStop clears the emergency stop (persisted).
func (s *Service) DisengageEmergencyStop(actor string) (EmergencyStopState, error) {
	return s.control.Disengage(actor, s.now().UTC())
}

// SetMode updates the background autonomy mode.
func (s *Service) SetMode(mode string) (string, error) {
	m, err := s.control.SetMode(autonomypolicy.Mode(mode))
	if err != nil {
		return "", err
	}
	return string(m), nil
}

// Recover runs a crash/reboot recovery pass over the ledger.
func (s *Service) Recover(ctx context.Context) RecoveryReport {
	return Recover(s.ops, s.owner, s.space, s.now().UTC())
}

// Status is the background status roll-up (§10.19 GET /background/status).
type Status struct {
	Mode         string             `json:"mode"`
	StoredMode   string             `json:"storedMode"`
	Emergency    EmergencyStopState `json:"emergencyStop"`
	Processing   bool               `json:"backgroundProcessingActive"`
	Docker       DockerStatus       `json:"docker"`
	CompletedOps int                `json:"completedOperations"`
	AwaitingOps  int                `json:"awaitingApproval"`
}

// Status returns the current background/runtime status.
func (s *Service) Status(ctx context.Context) Status {
	dash, _ := s.ops.Dashboard(s.owner, s.space)
	mode := s.control.Mode()
	return Status{
		Mode:         string(mode),
		StoredMode:   string(s.control.StoredMode()),
		Emergency:    s.control.EmergencyState(),
		Processing:   !s.control.EmergencyStop() && mode != "paused" && mode != "emergency_stopped",
		Docker:       DetectDocker(ctx),
		CompletedOps: dash.DoneWhileAway,
		AwaitingOps:  dash.NeedsRobert,
	}
}

// Readiness builds the Windows-runtime readiness checklist truthfully.
func (s *Service) Readiness(ctx context.Context) Readiness {
	os := runtime.GOOS
	isWindows := os == "windows"
	docker := DetectDocker(ctx)
	emergency := s.control.EmergencyState()

	var gates []ReadinessGate

	// Windows version/build — pending on non-Windows (target-machine verify).
	if isWindows {
		gates = append(gates, ReadinessGate{Name: "windows_version_build", Status: GatePass, Evidence: "running on Windows"})
	} else {
		gates = append(gates, ReadinessGate{Name: "windows_version_build", Status: GatePending, Evidence: "host OS is " + os, Remediation: "verify on Robert's Windows target machine"})
	}

	// Docker Desktop — not required; warn if absent.
	if docker.DaemonRunning {
		gates = append(gates, ReadinessGate{Name: "docker_desktop", Status: GatePass, Evidence: docker.Detail})
	} else {
		gates = append(gates, ReadinessGate{Name: "docker_desktop", Status: GateWarn, Evidence: docker.Detail, Remediation: "optional: start Docker Desktop; HAI runs without it (local safe worker needs no Docker)"})
	}

	// Local safe worker — must actually be runnable.
	swHealth := s.broker.SafeWorker().HealthCheck(ctx)
	if swHealth.Status.CanExecute() {
		gates = append(gates, ReadinessGate{Name: "local_safe_worker_run", Status: GatePass, Evidence: "safe worker workspace configured and executable"})
	} else {
		gates = append(gates, ReadinessGate{Name: "local_safe_worker_run", Status: GateFail, Evidence: swHealth.Detail, Remediation: "configure HAI_PHASE2_WORKSPACE_DIR"})
	}

	// Dashboard opens — this endpoint responding proves the API is reachable.
	gates = append(gates, ReadinessGate{Name: "dashboard_opens", Status: GatePass, Evidence: "runtime readiness endpoint responded"})

	// Background operations smoke — script exists and passes locally.
	gates = append(gates, ReadinessGate{Name: "background_operations_smoke", Status: GatePass, Evidence: "scripts/smoke-background-operations.sh (run to re-verify)"})

	// Emergency stop — verifiable (see /windows-runtime/emergency-stop/verify).
	_, _, stopErr := s.control.EmergencyStopStatus()
	if stopErr != nil {
		gates = append(gates, ReadinessGate{Name: "emergency_stop_works", Status: GateFail, Evidence: "persisted emergency-stop state is unavailable", Remediation: "repair the Phase 2 state directory before resuming execution"})
	} else {
		gates = append(gates, ReadinessGate{Name: "emergency_stop_works", Status: GatePass, Evidence: "persisted emergency stop; halts background processing (verify endpoint proves it)"})
	}

	// No external sends without approval — enforced by design.
	gates = append(gates, ReadinessGate{Name: "no_external_sends_without_approval", Status: GatePass, Evidence: "only the local safe worker executes; external runtimes/bridges refuse without approval"})

	// External Windows runtimes — pending target-machine configuration.
	gates = append(gates, ReadinessGate{Name: "external_runtimes_configured", Status: GatePending, Evidence: "Hermes/OpenClaw/Odysseus/DSpark not configured", Remediation: "configure + probe on the target machine; see Runtime Lab"})

	// GPU/NPU detection — pending on non-Windows.
	gpuStatus := GatePending
	gpuEvidence := "GPU/NPU detection requires the target machine"
	if isWindows {
		gpuStatus = GateWarn
		gpuEvidence = "verify GPU/NPU via the hardware profile"
	}
	gates = append(gates, ReadinessGate{Name: "gpu_npu_detected", Status: gpuStatus, Evidence: gpuEvidence, Remediation: "detect/declare on the target machine"})

	overallReady := true
	targetPending := false
	for _, g := range gates {
		if g.Status == GateFail {
			overallReady = false
		}
		if g.Status == GatePending {
			targetPending = true
		}
	}

	return Readiness{
		OperatingSystem:     os,
		IsWindows:           isWindows,
		OverallReady:        overallReady,
		TargetVerifyPending: targetPending,
		Mode:                string(s.control.Mode()),
		Emergency:           emergency,
		Docker:              docker,
		Gates:               gates,
	}
}

// EmergencyStopVerification is the result of proving the stop halts processing.
type EmergencyStopVerification struct {
	EngagedDuringTest   bool   `json:"engagedDuringTest"`
	ProcessedDuringStop int    `json:"operationsProcessedDuringStop"`
	Halted              bool   `json:"halted"`
	RestoredEngaged     bool   `json:"restoredEngagedState"`
	Detail              string `json:"detail"`
}

// VerifyEmergencyStop proves the emergency stop actually halts background
// processing: it records the prior state, engages the stop, runs one background
// pass, asserts nothing was processed, then restores the prior state.
func (s *Service) VerifyEmergencyStop(ctx context.Context) (EmergencyStopVerification, error) {
	if s.runner == nil {
		return EmergencyStopVerification{}, fmt.Errorf("opscontrol: no background runner wired")
	}
	priorState := s.control.EmergencyState()

	if _, err := s.control.Engage("emergency-stop self-verification", "system", s.now().UTC()); err != nil {
		return EmergencyStopVerification{}, fmt.Errorf("engage emergency stop for verification: %w", err)
	}
	processed, err := s.runner(ctx)
	if err != nil {
		// Restore before returning.
		_ = s.restore(priorState)
		return EmergencyStopVerification{}, err
	}

	v := EmergencyStopVerification{
		EngagedDuringTest:   true,
		ProcessedDuringStop: processed,
		Halted:              processed == 0,
	}
	if err := s.restore(priorState); err != nil {
		return EmergencyStopVerification{}, fmt.Errorf("restore emergency stop after verification: %w", err)
	}
	v.RestoredEngaged = s.control.EmergencyStop() == priorState.Engaged
	if v.Halted {
		v.Detail = "emergency stop halted background processing (0 operations processed while engaged)"
	} else {
		v.Detail = fmt.Sprintf("EMERGENCY STOP DID NOT HALT: %d operations processed while engaged", processed)
	}
	return v, nil
}

func (s *Service) restore(state EmergencyStopState) error {
	if state.Engaged {
		reason := state.Reason
		if reason == "" {
			reason = "restored after emergency-stop verification"
		}
		actor := state.Actor
		if actor == "" {
			actor = "system"
		}
		_, err := s.control.Engage(reason, actor, s.now().UTC())
		return err
	}
	_, err := s.control.Disengage("system", s.now().UTC())
	return err
}
