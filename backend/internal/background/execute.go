package background

import (
	"context"
	"fmt"
	"time"

	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/operations"
)

// SafeOutcome reports the result of executing an Operation via the local safe
// worker.
type SafeOutcome struct {
	Operation *models.Operation
	Verified  bool
	Failed    bool
}

// ExecuteSafeOperation runs the local safe worker for an Operation and gates
// completion on passing verification (§8/§10.15). It drives the Operation
// classified/ready/failed -> ready -> running -> verifying -> completed|failed.
// It refuses any Operation whose decision is not run_safe_local_worker, so
// high-risk/external work is never fake-completed by the file-writing worker.
func ExecuteSafeOperation(ctx context.Context, svc *operations.Service, broker *executionbroker.Broker, op models.Operation, now time.Time) (SafeOutcome, error) {
	if operations.CurrentDecision(op.CurrentDecision) != operations.DecisionRunSafeLocalWorker {
		return SafeOutcome{}, fmt.Errorf("operation %s is not safe-executable (decision=%s); no real runtime is available in Phase 2A", op.ID, op.CurrentDecision)
	}

	// Bring the operation to `ready`. A fresh classified op or a retried failed
	// op both route through ready.
	switch operations.OperationStatus(op.Status) {
	case operations.StatusClassified, operations.StatusFailed:
		ready, err := svc.Transition(op, operations.StatusReady, "hai", "", "ready for safe local worker")
		if err != nil {
			return SafeOutcome{}, err
		}
		op = *ready
	case operations.StatusReady:
		// already ready
	default:
		return SafeOutcome{}, fmt.Errorf("operation %s cannot be safe-executed from status %s", op.ID, op.Status)
	}

	op.RuntimeID = executionbroker.LocalSafeWorkerID
	op.VerificationStatus = string(operations.VerificationPending)
	running, err := svc.Transition(op, operations.StatusRunning, "hai", "", "running safe local worker")
	if err != nil {
		return SafeOutcome{}, err
	}
	op = *running

	res, execErr := broker.ExecuteLocalSafeWorker(ctx, safePayload(op))
	if execErr != nil {
		op.VerificationStatus = string(operations.VerificationFailed)
		op.LastError = execErr.Error()
		failed, err := svc.Transition(op, operations.StatusFailed, "hai", "", "safe worker error: "+execErr.Error())
		if err != nil {
			return SafeOutcome{}, err
		}
		return SafeOutcome{Operation: failed, Failed: true}, nil
	}

	op.ResultSummary = res.Output.BoundedOutput
	verifying, err := svc.Transition(op, operations.StatusVerifying, "hai", "", "verifying safe worker result")
	if err != nil {
		return SafeOutcome{}, err
	}
	op = *verifying

	if res.OK && res.Verification.Passed {
		op.VerificationStatus = string(operations.VerificationPassed)
		completed, err := svc.Transition(op, operations.StatusCompleted, "hai", "", "verified and completed")
		if err != nil {
			return SafeOutcome{}, err
		}
		return SafeOutcome{Operation: completed, Verified: true}, nil
	}

	op.VerificationStatus = string(operations.VerificationFailed)
	op.LastError = "safe worker verification failed"
	failed, err := svc.Transition(op, operations.StatusFailed, "hai", "", "verification failed")
	if err != nil {
		return SafeOutcome{}, err
	}
	return SafeOutcome{Operation: failed, Failed: true}, nil
}
