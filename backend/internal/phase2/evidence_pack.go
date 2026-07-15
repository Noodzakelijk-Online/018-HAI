package phase2

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/privacyfilter"
)

// EvidencePack is a generated, auditable evidence pack for an Operation (§10.18).
// It redacts secrets, includes source-revision hashes + timestamps, and never
// embeds raw sensitive content.
type EvidencePack struct {
	ID          string    `json:"id"`
	OperationID string    `json:"operationId"`
	Title       string    `json:"title"`
	Markdown    string    `json:"markdown"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// EvidencePackStore holds generated packs for retrieval.
type EvidencePackStore struct {
	mu    sync.Mutex
	packs map[string]EvidencePack
	seq   int
}

// NewEvidencePackStore builds an empty store.
func NewEvidencePackStore() *EvidencePackStore {
	return &EvidencePackStore{packs: map[string]EvidencePack{}}
}

func (s *EvidencePackStore) put(p EvidencePack) EvidencePack {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	p.ID = fmt.Sprintf("evp-%d", s.seq)
	s.packs[p.ID] = p
	return p
}

// Get returns a stored pack by id.
func (s *EvidencePackStore) Get(id string) (EvidencePack, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.packs[id]
	return p, ok
}

// buildEvidencePack assembles the markdown evidence pack (§10.18). Absent record
// types are honestly labelled "none recorded" rather than fabricated.
func buildEvidencePack(op models.Operation, events []models.OperationEvent, scan privacyfilter.ScanResult, telemetry []modelintelligence.ModelRunTelemetry, now time.Time) EvidencePack {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# Evidence Pack: %s\n\n", op.Title)

	w("## Operation\n")
	w("- id: %s\n- type: %s\n- status: %s\n- risk: %s\n- autonomy: %s\n- owner: %s\n- decision: %s\n- created: %s\n- updated: %s\n\n",
		op.ID, op.OperationType, op.Status, op.RiskLevel, op.AutonomyLevel, op.OwnerType, op.CurrentDecision,
		op.CreatedAt.UTC().Format(time.RFC3339), op.UpdatedAt.UTC().Format(time.RFC3339))

	w("## Source Evidence\n")
	w("- sourceType: %s\n- sourceUri: %s\n- sourceRevisionHash: %s\n- dedupeKey: %s\n\n",
		orNone(op.SourceType), orNone(op.SourceURI), orNone(op.SourceRevisionHash), orNone(op.DedupeKey))

	w("## Extracted Claims\n")
	// Redact sensitive content: when the scan flags secrets/redaction, show the
	// bounded redacted preview instead of the raw description (§10.18).
	claims := op.Description
	if scan.RedactionApplied || !scan.SafeForCloudModel {
		claims = scan.RedactedPreview
	}
	w("- %s\n\n", orNone(claims))

	w("## Privacy Scan\n")
	w("- privacyRiskLevel: %s\n- sensitiveFields: %s\n- safeForCloudModel: %t\n- redactionApplied: %t\n- redactedPreview: %s\n\n",
		scan.PrivacyRiskLevel, joinOrNone(scan.SensitiveFields), scan.SafeForCloudModel, scan.RedactionApplied, orNone(scan.RedactedPreview))

	w("## Policy Decision\n")
	w("- currentDecision: %s\n- requiresApproval: %t\n- recommendedAction: %s\n\n",
		op.CurrentDecision, op.RequiresApproval, orNone(op.RecommendedAction))

	w("## Action Plan\n")
	w("- runtime: %s\n- modelProvider/model: %s / %s\n\n", orNone(op.RuntimeID), orNone(op.ModelProviderID), orNone(op.ModelID))

	w("## Approval\n")
	approvals := filterEvents(events, func(e models.OperationEvent) bool {
		return e.AfterStatus == string(operations.StatusAwaitingApproval) ||
			e.AfterStatus == string(operations.StatusApproved) ||
			e.AfterStatus == string(operations.StatusDismissed) ||
			e.EventType == "postponed"
	})
	if len(approvals) == 0 {
		w("- none recorded\n\n")
	} else {
		for _, e := range approvals {
			w("- [%s] %s: %s\n", e.CreatedAt.UTC().Format(time.RFC3339), e.EventType, e.Message)
		}
		w("\n")
	}

	w("## Runtime / Model Attempts\n")
	if op.RuntimeID == "" && len(telemetry) == 0 {
		w("- none recorded\n\n")
	} else {
		if op.RuntimeID != "" {
			w("- runtime %s: verification=%s result=%s\n", op.RuntimeID, op.VerificationStatus, orNone(op.ResultSummary))
		}
		for _, t := range telemetry {
			w("- model %s/%s lane=%s tokens(in/out)=%d/%d durationMs=%d ok=%t\n",
				t.ProviderID, t.ModelID, t.Lane, t.InputTokens, t.OutputTokens, t.DurationMs, t.OK)
		}
		w("\n")
	}

	w("## Verification\n")
	w("- verificationStatus: %s\n\n", op.VerificationStatus)

	w("## Failure Modes\n")
	if strings.TrimSpace(op.LastError) == "" {
		w("- none recorded\n\n")
	} else {
		w("- lastError: %s\n\n", op.LastError)
	}

	w("## Audit Timeline\n")
	if len(events) == 0 {
		w("- none recorded\n\n")
	} else {
		for _, e := range events {
			w("- [%s] %s %s->%s: %s\n", e.CreatedAt.UTC().Format(time.RFC3339), e.EventType, orNone(e.BeforeStatus), e.AfterStatus, e.Message)
		}
		w("\n")
	}

	w("## Known Limits\n")
	w("- Secrets are redacted; raw sensitive content is never embedded.\n")
	w("- Outbox: OperationEvents are the durable event log (Postgres); the Kafka publisher is disabled in this deployment, so no events are lost.\n")
	w("- Some sub-records (ActionPlan/FailureMode as separate entities) are derived from operation fields + events in this phase.\n")

	return EvidencePack{OperationID: op.ID.String(), Title: op.Title, Markdown: b.String(), GeneratedAt: now}
}

func filterEvents(events []models.OperationEvent, keep func(models.OperationEvent) bool) []models.OperationEvent {
	var out []models.OperationEvent
	for _, e := range events {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
