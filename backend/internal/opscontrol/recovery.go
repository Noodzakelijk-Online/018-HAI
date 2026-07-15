package opscontrol

import (
	"fmt"
	"time"

	"automation-hub-backend/internal/operations"
)

// RecoveryReport summarizes a crash/reboot recovery pass.
type RecoveryReport struct {
	ScannedRunning   int       `json:"scannedRunning"`
	ScannedVerifying int       `json:"scannedVerifying"`
	Recovered        int       `json:"recovered"`
	Details          []string  `json:"details,omitempty"`
	RanAt            time.Time `json:"ranAt"`
}

// Recover reconciles operations left in a non-terminal executing state by a
// crash/reboot (§31 recovery). A `running` operation had an uncertain side
// effect, so it is moved to `interrupted` for review; a `verifying` operation
// is moved to `awaiting_approval` so a human confirms the outcome. Nothing is
// silently completed.
func Recover(svc *operations.Service, ownerUserID, workspaceID string, now time.Time) RecoveryReport {
	rep := RecoveryReport{RanAt: now.UTC()}

	running, _ := svc.List(operations.Filter{OwnerUserID: ownerUserID, WorkspaceID: workspaceID, Status: operations.StatusRunning, Limit: 200})
	rep.ScannedRunning = len(running)
	for _, op := range running {
		if _, err := svc.Transition(op, operations.StatusInterrupted, "recovery", "", "recovered after crash/reboot: run interrupted, side effect uncertain"); err != nil {
			rep.Details = append(rep.Details, fmt.Sprintf("op %s: %v", op.ID, err))
			continue
		}
		rep.Recovered++
		rep.Details = append(rep.Details, fmt.Sprintf("op %s: running -> interrupted", op.ID))
	}

	verifying, _ := svc.List(operations.Filter{OwnerUserID: ownerUserID, WorkspaceID: workspaceID, Status: operations.StatusVerifying, Limit: 200})
	rep.ScannedVerifying = len(verifying)
	for _, op := range verifying {
		if _, err := svc.Transition(op, operations.StatusAwaitingApproval, "recovery", "", "recovered after crash/reboot: verification incomplete, needs human confirmation"); err != nil {
			rep.Details = append(rep.Details, fmt.Sprintf("op %s: %v", op.ID, err))
			continue
		}
		rep.Recovered++
		rep.Details = append(rep.Details, fmt.Sprintf("op %s: verifying -> awaiting_approval", op.ID))
	}
	return rep
}
