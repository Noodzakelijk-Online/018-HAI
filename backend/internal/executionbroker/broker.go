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
	issuer     AuthorizationIssuer
}

// NewBroker builds a broker with a local safe worker confined to workspaceRoot.
func NewBroker(workspaceRoot string) *Broker {
	return &Broker{safeWorker: NewLocalSafeWorker(workspaceRoot)}
}

// NewAuthorizedBroker builds the production local-safe-worker path. The
// bridge derives authorization server-side and consumes it at the final
// filesystem boundary.
func NewAuthorizedBroker(
	workspaceRoot string,
	owner string,
	workspaceID string,
	service ExecutionAuthorizationService,
) (*Broker, error) {
	bridge, err := NewDurableAuthorizationBridge(service, owner, workspaceID)
	if err != nil {
		return nil, err
	}
	return &Broker{
		safeWorker: newProductionLocalSafeWorker(workspaceRoot, bridge, bridge),
		issuer:     bridge,
	}, nil
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
	if b == nil || b.safeWorker == nil || b.safeWorker.verifier == nil {
		return ExecutionResult{RuntimeID: LocalSafeWorkerID}, ErrAuthorizationRequired
	}
	health := b.safeWorker.HealthCheck(ctx)
	if !health.Status.CanExecute() {
		return ExecutionResult{RuntimeID: LocalSafeWorkerID}, fmt.Errorf("runtime %s not executable: status=%s", LocalSafeWorkerID, health.Status)
	}
	if b.issuer != nil {
		var err error
		in, err = b.issuer.Issue(ctx, b.safeWorker.WorkspaceRoot, in)
		if err != nil {
			return ExecutionResult{RuntimeID: LocalSafeWorkerID}, err
		}
	}
	out, err := b.safeWorker.Run(ctx, in)
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
