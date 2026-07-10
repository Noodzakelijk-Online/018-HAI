package executionbroker

import (
	"context"
	"fmt"
)

// Broker selects an executor, validates runtime health, runs the action, bounds
// output, and verifies. For Phase 2A only the local safe worker executes; other
// runtimes are contract-defined and refused until real. The broker never
// executes when the runtime status is not ready (§10.14).
type Broker struct {
	safeWorker *LocalSafeWorker
}

// NewBroker builds a broker with a local safe worker confined to workspaceRoot.
func NewBroker(workspaceRoot string) *Broker {
	return &Broker{safeWorker: NewLocalSafeWorker(workspaceRoot)}
}

// SafeWorker exposes the underlying safe worker (e.g. for health reporting).
func (b *Broker) SafeWorker() *LocalSafeWorker { return b.safeWorker }

// ExecutionResult is the bounded, verified outcome of a safe execution.
type ExecutionResult struct {
	RuntimeID    string                 `json:"runtimeId"`
	OK           bool                   `json:"ok"`
	Output       SafeWorkerOutput       `json:"output"`
	Verification SafeWorkerVerification `json:"verification"`
}

// ExecuteLocalSafeWorker runs the safe worker for a bounded workspace task and
// verifies its postconditions. It refuses to execute when the runtime is not
// ready.
func (b *Broker) ExecuteLocalSafeWorker(ctx context.Context, in SafeWorkerInput) (ExecutionResult, error) {
	health := b.safeWorker.HealthCheck(ctx)
	if !health.Status.CanExecute() {
		return ExecutionResult{RuntimeID: LocalSafeWorkerID}, fmt.Errorf("runtime %s not executable: status=%s", LocalSafeWorkerID, health.Status)
	}
	out, err := b.safeWorker.Run(in)
	if err != nil {
		return ExecutionResult{RuntimeID: LocalSafeWorkerID}, err
	}
	v := b.safeWorker.Verify(in, out)
	return ExecutionResult{
		RuntimeID:    LocalSafeWorkerID,
		OK:           v.Passed,
		Output:       out,
		Verification: v,
	}, nil
}
