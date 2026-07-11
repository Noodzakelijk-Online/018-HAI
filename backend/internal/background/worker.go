// Package background runs the autonomous back-office loop (§10.16). One RunOnce
// pass ingests account-feed items into the Operation Ledger, then for each new
// Operation runs the deterministic pipeline: privacy scan -> risk/autonomy
// decision -> route (auto-execute the safe local worker + verify, request
// approval, block, draft, or observe). Every step writes audit events and moves
// the Operation through its state machine. Nothing is faked: only the local
// safe worker actually executes, and completion requires passing verification.
package background

import (
	"context"
	"errors"
	"fmt"
	"time"

	"automation-hub-backend/internal/accountfeed"
	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/idempotency"
	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/privacyfilter"

	"github.com/google/uuid"
)

// ErrBusy is returned when a RunOnce is already in progress.
var ErrBusy = errors.New("background: run already in progress")

// Options configures a background worker.
type Options struct {
	OwnerUserID   string
	WorkspaceID   string
	Mode          autonomypolicy.Mode
	EmergencyStop bool
	MaxOps        int
}

// Worker orchestrates one background pass over feeds and the Operation Ledger.
type Worker struct {
	svc      *operations.Service
	broker   *executionbroker.Broker
	readers  []accountfeed.Reader
	modelInt *modelintelligence.Service // optional; drives the fast-triage lane
	opts     Options
	now      func() time.Time
	lease    *lease
}

// New builds a worker. If opts.MaxOps <= 0 a default of 50 is used.
func New(svc *operations.Service, broker *executionbroker.Broker, readers []accountfeed.Reader, opts Options) *Worker {
	if opts.WorkspaceID == "" {
		opts.WorkspaceID = "local"
	}
	if opts.MaxOps <= 0 {
		opts.MaxOps = 50
	}
	if opts.Mode == "" {
		opts.Mode = autonomypolicy.ModeAutonomousSafe
	}
	return &Worker{svc: svc, broker: broker, readers: readers, opts: opts, now: time.Now, lease: newLease()}
}

// WithModelIntelligence attaches a model-intelligence service so the fast-triage
// lane runs a real (bounded, local) model call per operation and records
// telemetry. Returns the worker for chaining.
func (w *Worker) WithModelIntelligence(mi *modelintelligence.Service) *Worker {
	w.modelInt = mi
	return w
}

// Report summarizes a RunOnce pass.
type Report struct {
	FeedsRead         int      `json:"feedsRead"`
	ItemsIngested     int      `json:"itemsIngested"`
	OperationsCreated int      `json:"operationsCreated"`
	Classified        int      `json:"classified"`
	Triaged           int      `json:"triaged"`
	AutoExecuted      int      `json:"autoExecuted"`
	Verified          int      `json:"verified"`
	Failed            int      `json:"failed"`
	AwaitingApproval  int      `json:"awaitingApproval"`
	Blocked           int      `json:"blocked"`
	Drafted           int      `json:"drafted"`
	Observed          int      `json:"observed"`
	Errors            []string `json:"errors,omitempty"`
}

// RunOnce performs a single background pass. It acquires the lease first; if a
// pass is already running it returns ErrBusy.
func (w *Worker) RunOnce(ctx context.Context) (Report, error) {
	if !w.lease.acquire() {
		return Report{}, ErrBusy
	}
	defer w.lease.release()

	var rep Report
	w.ingest(ctx, &rep)
	if w.opts.EmergencyStop {
		// Emergency stop still ingests for the record but processes nothing.
		return rep, nil
	}
	w.process(ctx, &rep)
	return rep, nil
}

// ingest reads every feed and creates/refreshes Operations for its items.
func (w *Worker) ingest(ctx context.Context, rep *Report) {
	for _, r := range w.readers {
		feed := r.Feed()
		if !feed.Enabled {
			continue
		}
		rep.FeedsRead++
		items, err := r.Read(ctx)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("feed %s: %v", feed.Name, err))
			continue
		}
		for _, it := range items {
			rep.ItemsIngested++
			in, err := feed.ToOperationInput(it)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("feed %s item %s: %v", feed.Name, it.ExternalID, err))
				continue
			}
			res, err := w.svc.Ingest(in)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("ingest %s: %v", it.ExternalID, err))
				continue
			}
			if res.Created {
				rep.OperationsCreated++
			}
		}
	}
}

// process classifies and routes every actionable Operation currently in `new`.
func (w *Worker) process(ctx context.Context, rep *Report) {
	due, err := w.svc.ListDue(w.opts.OwnerUserID, w.opts.WorkspaceID, w.opts.MaxOps)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("list due: %v", err))
		return
	}
	for _, op := range due {
		if op.Status != string(operations.StatusNew) {
			continue // Phase 2A processes freshly-ingested operations.
		}
		if err := w.processOne(ctx, op, rep); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("operation %s: %v", op.ID, err))
		}
	}
}

func (w *Worker) processOne(ctx context.Context, op models.Operation, rep *Report) error {
	now := w.now().UTC()
	scan := privacyfilter.Scan(op.Title+"\n"+op.Description, 280)
	decision := autonomypolicy.Decide(autonomypolicy.Input{
		Title:         op.Title,
		Content:       op.Description,
		OperationType: op.OperationType,
		Privacy:       scan,
		Mode:          w.opts.Mode,
		Reversible:    true, // feed items default reversible; risk classifier bumps dangerous ones to approval
		EmergencyStop: w.opts.EmergencyStop,
	}, now)

	applyDecision(&op, decision, scan)

	// Fast-triage lane (§16): if a model-intelligence service is attached, run a
	// real bounded local model call to categorize the item. The lane affects the
	// operation record (model provider/model fields) and records telemetry that
	// surfaces on the model-intelligence dashboard.
	classifyMsg := "classified: " + decision.Reason
	highRisk := decision.Risk == operations.RiskHigh
	if w.modelInt != nil {
		if tri, tErr := w.modelInt.Triage(ctx, op.OperationType, op.Title, op.Description, scan.SafeForCloudModel, highRisk, op.ID.String()); tErr == nil && tri.Routed {
			op.ModelProviderID = tri.ProviderID
			op.ModelID = tri.ModelID
			classifyMsg = "classified [" + tri.Category + "]: " + decision.Reason
			rep.Triaged++
		}
	}

	classified, err := w.svc.Transition(op, operations.StatusClassified, "hai", "", classifyMsg)
	if err != nil {
		return err
	}
	rep.Classified++
	op = *classified

	switch decision.Decision {
	case operations.DecisionRunSafeLocalWorker:
		return w.runSafe(ctx, op, rep)
	case operations.DecisionBlock:
		_, err := w.svc.Transition(op, operations.StatusBlocked, "hai", "", "blocked: "+decision.Reason)
		if err == nil {
			rep.Blocked++
		}
		return err
	case operations.DecisionCreateDraft:
		return w.runDraft(op, rep, decision)
	case operations.DecisionObserveOnly:
		rep.Observed++
		return nil
	default:
		// ask_robert and any approval-gated decision.
		if decision.RequiresApproval {
			_, err := w.svc.Transition(op, operations.StatusAwaitingApproval, "hai", "", "awaiting approval: "+decision.Reason)
			if err == nil {
				rep.AwaitingApproval++
			}
			return err
		}
		rep.Observed++
		return nil
	}
}

// runSafe executes the local safe worker for a low-risk reversible operation and
// gates completion on passing verification (§8/§10.15).
func (w *Worker) runSafe(ctx context.Context, op models.Operation, rep *Report) error {
	rep.AutoExecuted++
	outcome, err := ExecuteSafeOperation(ctx, w.svc, w.broker, op, w.now().UTC())
	if err != nil {
		return err
	}
	if outcome.Verified {
		rep.Verified++
	}
	if outcome.Failed {
		rep.Failed++
	}
	return nil
}

// runDraft records an internal draft for a draft-mode operation.
func (w *Worker) runDraft(op models.Operation, rep *Report, decision autonomypolicy.Decision) error {
	drafting, err := w.svc.Transition(op, operations.StatusDrafting, "hai", "", "preparing internal draft")
	if err != nil {
		return err
	}
	op = *drafting
	op.ResultSummary = decision.RecommendedAction
	if _, err := w.svc.Transition(op, operations.StatusDraftReady, "hai", "", "internal draft ready for review"); err != nil {
		return err
	}
	rep.Drafted++
	return nil
}

// applyDecision maps the policy decision + privacy scan onto Operation fields.
func applyDecision(op *models.Operation, d autonomypolicy.Decision, scan privacyfilter.ScanResult) {
	op.RiskLevel = string(d.Risk)
	op.AutonomyLevel = string(d.Autonomy)
	op.CurrentDecision = string(d.Decision)
	op.RequiresApproval = d.RequiresApproval
	op.OwnerType = string(d.Owner)
	op.RecommendedAction = d.RecommendedAction
	op.NextReviewAt = d.NextReviewAt
	if d.Decision == operations.DecisionRunSafeLocalWorker {
		op.VerificationStatus = string(operations.VerificationPending)
	}
	if wm, err := idempotency.CanonicalJSONString(map[string]any{
		"policyRule":       d.PolicyRule,
		"reason":           d.Reason,
		"privacyRiskLevel": string(scan.PrivacyRiskLevel),
		"sensitiveFields":  scan.SensitiveFields,
		"safeForCloud":     scan.SafeForCloudModel,
		"redactedPreview":  scan.RedactedPreview,
	}); err == nil {
		op.WorldModelStateJSON = wm
	}
}

// safePayload derives the deterministic, bounded safe-worker payload for an
// operation. artifactName is a basename only; the marker binds the artifact to
// the operation identity + source revision.
func safePayload(op models.Operation) executionbroker.SafeWorkerInput {
	name := "operation-" + shortID(op.ID) + ".txt"
	marker := "HAI-OP " + op.ID.String() + " rev " + op.SourceRevisionHash
	return executionbroker.SafeWorkerInput{ArtifactName: name, Marker: marker}
}

func shortID(id uuid.UUID) string {
	s := id.String()
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}
