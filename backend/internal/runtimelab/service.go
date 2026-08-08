package runtimelab

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/background"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/idempotency"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/operations"
)

// RuntimeAttempt is an auditable record of a runtime execution/self-test attempt
// (§10.9). It is only ever created from a real attempt — a setup_required record
// truthfully means nothing executed.
type RuntimeAttempt struct {
	ID                 string               `json:"id"`
	RuntimeID          string               `json:"runtimeId"`
	OperationID        string               `json:"operationId,omitempty"`
	Status             RuntimeAttemptStatus `json:"status"`
	IdempotencyKey     string               `json:"idempotencyKey,omitempty"`
	Detail             string               `json:"detail"`
	BoundedOutput      string               `json:"boundedOutput,omitempty"`
	VerificationPassed bool                 `json:"verificationPassed"`
	CreatedAt          time.Time            `json:"createdAt"`
}

// RuntimeSummary is a runtime's truthful status for the overview.
type RuntimeSummary struct {
	Info              RuntimeInfo                   `json:"info"`
	Status            executionbroker.RuntimeStatus `json:"status"`
	ClaimLevel        executionbroker.ClaimLevel    `json:"claimLevel"`
	CanExecute        bool                          `json:"canExecute"`
	Capabilities      []string                      `json:"capabilities"`
	SetupRequirements []SetupRequirement            `json:"setupRequirements,omitempty"`
	LastAttempt       *RuntimeAttempt               `json:"lastAttempt,omitempty"`
}

// Service is the Runtime Lab: it probes runtimes, self-tests them (safely, via
// the Operation Ledger for the local safe worker), and records attempts. It
// never claims execution for a runtime that did not actually run.
type Service struct {
	reg    *Registry
	broker *executionbroker.Broker
	ops    *operations.Service // optional; enables ledger-backed self-test
	owner  string
	space  string
	now    func() time.Time

	mu       sync.Mutex
	attempts []RuntimeAttempt
	seq      int
}

// NewService builds a runtime lab service.
func NewService(broker *executionbroker.Broker, ops *operations.Service, ownerUserID, workspaceID string) *Service {
	return &Service{
		reg:    NewRegistry(broker),
		broker: broker,
		ops:    ops,
		owner:  ownerUserID,
		space:  workspaceID,
		now:    time.Now,
	}
}

// Overview returns every runtime's truthful status + last attempt.
func (s *Service) Overview(ctx context.Context) []RuntimeSummary {
	out := make([]RuntimeSummary, 0, len(s.reg.Adapters()))
	for _, a := range s.reg.Adapters() {
		h := a.HealthCheck(ctx)
		out = append(out, RuntimeSummary{
			Info:              a.Info(),
			Status:            h.Status,
			ClaimLevel:        h.Claim,
			CanExecute:        h.Status.CanExecute(),
			Capabilities:      a.Capabilities(),
			SetupRequirements: h.SetupRequirements,
			LastAttempt:       s.lastAttempt(a.Info().ID),
		})
	}
	return out
}

// FeatureParity returns the source-reviewed feature/disposition inventory.
// Reading it never probes, installs, configures, or executes a runtime.
func (s *Service) FeatureParity() (RuntimeParityOverview, error) {
	return RuntimeFeatureParity(s.now().UTC())
}

// RuntimeFeatureParity returns one runtime inventory without broadening its
// declared readiness ceiling.
func (s *Service) RuntimeFeatureParity(runtimeID string) (RuntimeParityInventory, bool, error) {
	overview, err := s.FeatureParity()
	if err != nil {
		return RuntimeParityInventory{}, false, err
	}
	runtimeID = normalizeRuntimeID(runtimeID)
	for _, inventory := range overview.Inventories {
		if inventory.RuntimeID == runtimeID {
			return inventory, true, nil
		}
	}
	return RuntimeParityInventory{}, false, nil
}

// Probe probes a runtime truthfully.
func (s *Service) Probe(ctx context.Context, runtimeID string) (ProbeResult, bool) {
	a, ok := s.reg.Adapter(runtimeID)
	if !ok {
		return ProbeResult{}, false
	}
	return a.Probe(ctx, s.now().UTC()), true
}

// SelfTest runs a safe self-test. For the local safe worker it executes a real
// bounded task through the Operation Ledger and verifies it. For every other
// runtime it records a truthful setup_required attempt — no fake execution.
func (s *Service) SelfTest(ctx context.Context, runtimeID string) (RuntimeAttempt, bool) {
	a, ok := s.reg.Adapter(runtimeID)
	if !ok {
		return RuntimeAttempt{}, false
	}
	now := s.now().UTC()

	if runtimeID == executionbroker.LocalSafeWorkerID && s.ops != nil {
		return s.selfTestSafeWorker(ctx, a, now), true
	}

	// Non-safe runtime: never fake execution. Report the health-derived status.
	h := a.HealthCheck(ctx)
	status := AttemptSetupRequired
	if h.Status.CanExecute() {
		// Reachable but no operator-verified integration -> inconclusive, not success.
		status = AttemptInconclusive
	}
	return s.record(RuntimeAttempt{
		RuntimeID: runtimeID,
		Status:    status,
		Detail:    h.Detail,
		CreatedAt: now,
	}), true
}

func (s *Service) selfTestSafeWorker(ctx context.Context, a Adapter, now time.Time) RuntimeAttempt {
	// Create a real Operation for the self-test so it flows through the ledger.
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	in := operations.NewOperationInput{
		OwnerUserID:   s.owner,
		WorkspaceID:   s.space,
		Title:         "Runtime self-test: local safe worker",
		Description:   "Safe workspace-confined write/read-back/hash self-test through the Operation Ledger.",
		OperationType: "runtime_self_test",
		SourceType:    "runtime_lab",
		DedupeKey:     idempotency.OperationDedupeKey(s.space, "runtime_self_test", "runtime_lab", executionbroker.LocalSafeWorkerID, itoa(seq)),
	}
	ingest, err := s.ops.Ingest(in)
	if err != nil {
		return s.record(RuntimeAttempt{RuntimeID: executionbroker.LocalSafeWorkerID, Status: AttemptFailed, Detail: "ingest: " + err.Error(), CreatedAt: now})
	}
	op := ingest.Operation
	op.CurrentDecision = string(operations.DecisionRunSafeLocalWorker)
	classified, err := s.ops.Transition(op, operations.StatusClassified, "hai", "", "runtime self-test classified")
	if err != nil {
		return s.record(RuntimeAttempt{RuntimeID: executionbroker.LocalSafeWorkerID, OperationID: op.ID.String(), Status: AttemptFailed, Detail: "classify: " + err.Error(), CreatedAt: now})
	}

	outcome, err := background.ExecuteSafeOperation(ctx, s.ops, s.broker, *classified, now)
	if err != nil {
		return s.record(RuntimeAttempt{RuntimeID: executionbroker.LocalSafeWorkerID, OperationID: op.ID.String(), Status: AttemptFailed, Detail: err.Error(), CreatedAt: now})
	}

	attempt := RuntimeAttempt{
		RuntimeID:          executionbroker.LocalSafeWorkerID,
		OperationID:        op.ID.String(),
		IdempotencyKey:     idempotency.RuntimeAttemptIdempotencyKey(op.ID.String(), "self-test", executionbroker.LocalSafeWorkerID, ""),
		VerificationPassed: outcome.Verified,
		CreatedAt:          now,
	}
	if outcome.Verified {
		attempt.Status = AttemptSucceeded
		attempt.Detail = "safe worker executed and verified through the Operation Ledger"
		if outcome.Operation != nil {
			attempt.BoundedOutput = outcome.Operation.ResultSummary
		}
	} else {
		attempt.Status = AttemptFailed
		attempt.Detail = "safe worker did not pass verification"
	}
	return s.record(attempt)
}

// Attempts returns the recorded attempts for a runtime (newest first).
func (s *Service) Attempts(runtimeID string) []RuntimeAttempt {
	runtimeID = normalizeRuntimeID(runtimeID)
	s.mu.Lock()
	out := make([]RuntimeAttempt, 0, len(s.attempts))
	for i := len(s.attempts) - 1; i >= 0; i-- {
		if s.attempts[i].RuntimeID == runtimeID {
			out = append(out, s.attempts[i])
		}
	}
	s.mu.Unlock()

	if runtimeID == executionbroker.LocalSafeWorkerID {
		out = mergeRuntimeAttempts(out, s.safeWorkerLedgerAttempts())
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *Service) record(a RuntimeAttempt) RuntimeAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	a.ID = "rta-" + itoa(s.seq)
	s.attempts = append(s.attempts, a)
	return a
}

func (s *Service) lastAttempt(runtimeID string) *RuntimeAttempt {
	attempts := s.Attempts(runtimeID)
	if len(attempts) > 0 {
		attempt := attempts[0]
		return &attempt
	}
	return nil
}

func (s *Service) safeWorkerLedgerAttempts() []RuntimeAttempt {
	if s.ops == nil {
		return nil
	}
	ledger, err := s.ops.List(operations.Filter{
		OwnerUserID: s.owner,
		WorkspaceID: s.space,
		Limit:       200,
	})
	if err != nil {
		return nil
	}
	attempts := make([]RuntimeAttempt, 0, len(ledger))
	for _, operation := range ledger {
		if operation.OperationType != "runtime_self_test" ||
			operation.SourceType != "runtime_lab" ||
			operation.RuntimeID != executionbroker.LocalSafeWorkerID {
			continue
		}
		attempts = append(attempts, runtimeAttemptFromOperation(operation))
	}
	return attempts
}

func runtimeAttemptFromOperation(operation models.Operation) RuntimeAttempt {
	createdAt := operation.UpdatedAt
	if operation.CompletedAt != nil {
		createdAt = *operation.CompletedAt
	}
	status := AttemptPending
	switch operations.OperationStatus(operation.Status) {
	case operations.StatusCompleted:
		if operations.VerificationStatus(operation.VerificationStatus) == operations.VerificationPassed {
			status = AttemptSucceeded
		} else {
			status = AttemptFailed
		}
	case operations.StatusFailed:
		status = AttemptFailed
	case operations.StatusBlocked:
		status = AttemptBlocked
	case operations.StatusRunning, operations.StatusVerifying:
		status = AttemptRunning
	}
	detail := operation.LastError
	if detail == "" {
		detail = "safe worker execution recovered from the durable Operation Ledger"
	}
	return RuntimeAttempt{
		ID:                 "rta-operation-" + operation.ID.String(),
		RuntimeID:          executionbroker.LocalSafeWorkerID,
		OperationID:        operation.ID.String(),
		Status:             status,
		IdempotencyKey:     operation.DedupeKey,
		Detail:             detail,
		BoundedOutput:      operation.ResultSummary,
		VerificationPassed: operations.VerificationStatus(operation.VerificationStatus) == operations.VerificationPassed,
		CreatedAt:          createdAt,
	}
}

func mergeRuntimeAttempts(primary, recovered []RuntimeAttempt) []RuntimeAttempt {
	seenOperations := make(map[string]struct{}, len(primary))
	out := append([]RuntimeAttempt(nil), primary...)
	for _, attempt := range primary {
		if attempt.OperationID != "" {
			seenOperations[attempt.OperationID] = struct{}{}
		}
	}
	for _, attempt := range recovered {
		if _, exists := seenOperations[attempt.OperationID]; exists {
			continue
		}
		out = append(out, attempt)
	}
	return out
}

func normalizeRuntimeID(runtimeID string) string {
	return strings.ToLower(strings.TrimSpace(runtimeID))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
