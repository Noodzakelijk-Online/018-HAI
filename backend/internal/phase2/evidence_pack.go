package phase2

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/privacyfilter"

	"github.com/google/uuid"
)

var (
	// ErrEvidencePackNotFound deliberately does not distinguish between a
	// missing pack and one belonging to a different owner or workspace.
	ErrEvidencePackNotFound = errors.New("phase2: evidence pack not found")
	// ErrEvidencePackRepositoryUnavailable is returned when durable storage was
	// not configured or could not be opened. Evidence packs never fall back to
	// process-local storage.
	ErrEvidencePackRepositoryUnavailable = errors.New("phase2: durable evidence pack repository unavailable")
	// ErrEvidencePackIntegrityViolation prevents a malformed or tampered
	// durable row from being returned as evidence.
	ErrEvidencePackIntegrityViolation = errors.New("phase2: evidence pack integrity violation")
)

// EvidenceProvenance preserves the operation source coordinates used to build
// a pack. It is persisted separately from the rendered Markdown so callers can
// inspect provenance without parsing presentation text.
type EvidenceProvenance struct {
	SourceType         string     `json:"sourceType"`
	SourceID           *uuid.UUID `json:"sourceId,omitempty"`
	SourceURI          string     `json:"sourceUri,omitempty"`
	SourceReceivedAt   *time.Time `json:"sourceReceivedAt,omitempty"`
	SourceRevisionHash string     `json:"sourceRevisionHash,omitempty"`
	DedupeKey          string     `json:"dedupeKey"`
}

// EvidencePack is a generated, auditable evidence pack for an Operation. It
// redacts secrets, includes source-revision hashes and timestamps, and never
// embeds raw sensitive content.
type EvidencePack struct {
	ID            uuid.UUID          `json:"id"`
	OwnerIdentity string             `json:"ownerIdentity"`
	WorkspaceID   string             `json:"workspaceId"`
	OperationID   uuid.UUID          `json:"operationId"`
	Title         string             `json:"title"`
	Markdown      string             `json:"markdown"`
	Provenance    EvidenceProvenance `json:"provenance"`
	ContentDigest string             `json:"contentDigest"`
	GeneratedAt   time.Time          `json:"generatedAt"`
}

// EvidencePackRepository is the durable, owner-scoped persistence boundary.
// Implementations must scope reads by all three coordinates so UUID knowledge
// alone never grants cross-owner access.
type EvidencePackRepository interface {
	Create(context.Context, EvidencePack) (EvidencePack, error)
	Get(context.Context, string, string, uuid.UUID) (EvidencePack, error)
}

func normalizeEvidencePack(pack EvidencePack) (EvidencePack, error) {
	pack.OwnerIdentity = strings.TrimSpace(pack.OwnerIdentity)
	pack.WorkspaceID = strings.TrimSpace(pack.WorkspaceID)
	pack.Title = strings.TrimSpace(pack.Title)
	if pack.ID == uuid.Nil {
		pack.ID = uuid.New()
	}
	if pack.OwnerIdentity == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack owner identity is required")
	}
	if pack.WorkspaceID == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack workspace id is required")
	}
	if pack.OperationID == uuid.Nil {
		return EvidencePack{}, fmt.Errorf("evidence pack operation id is required")
	}
	if pack.Title == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack title is required")
	}
	if strings.TrimSpace(pack.Markdown) == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack markdown is required")
	}
	if strings.TrimSpace(pack.Provenance.SourceType) == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack source type is required")
	}
	if strings.TrimSpace(pack.Provenance.DedupeKey) == "" {
		return EvidencePack{}, fmt.Errorf("evidence pack dedupe key is required")
	}
	if pack.GeneratedAt.IsZero() {
		return EvidencePack{}, fmt.Errorf("evidence pack generation time is required")
	}
	pack.GeneratedAt = pack.GeneratedAt.UTC()
	if pack.Provenance.SourceReceivedAt != nil {
		value := pack.Provenance.SourceReceivedAt.UTC()
		pack.Provenance.SourceReceivedAt = &value
	}
	digest, err := evidencePackDigest(pack)
	if err != nil {
		return EvidencePack{}, err
	}
	pack.ContentDigest = digest
	return pack, nil
}

func evidencePackDigest(pack EvidencePack) (string, error) {
	sourceID := ""
	if pack.Provenance.SourceID != nil {
		sourceID = pack.Provenance.SourceID.String()
	}
	sourceReceivedAt := ""
	if pack.Provenance.SourceReceivedAt != nil {
		sourceReceivedAt = pack.Provenance.SourceReceivedAt.UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(struct {
		ID                 string `json:"id"`
		OwnerIdentity      string `json:"ownerIdentity"`
		WorkspaceID        string `json:"workspaceId"`
		OperationID        string `json:"operationId"`
		Title              string `json:"title"`
		Markdown           string `json:"markdown"`
		SourceType         string `json:"sourceType"`
		SourceID           string `json:"sourceId"`
		SourceURI          string `json:"sourceUri"`
		SourceReceivedAt   string `json:"sourceReceivedAt"`
		SourceRevisionHash string `json:"sourceRevisionHash"`
		DedupeKey          string `json:"dedupeKey"`
		GeneratedAt        string `json:"generatedAt"`
	}{
		ID:                 pack.ID.String(),
		OwnerIdentity:      pack.OwnerIdentity,
		WorkspaceID:        pack.WorkspaceID,
		OperationID:        pack.OperationID.String(),
		Title:              pack.Title,
		Markdown:           pack.Markdown,
		SourceType:         pack.Provenance.SourceType,
		SourceID:           sourceID,
		SourceURI:          pack.Provenance.SourceURI,
		SourceReceivedAt:   sourceReceivedAt,
		SourceRevisionHash: pack.Provenance.SourceRevisionHash,
		DedupeKey:          pack.Provenance.DedupeKey,
		GeneratedAt:        pack.GeneratedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", fmt.Errorf("encode evidence pack digest: %w", err)
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

// buildEvidencePack assembles the Markdown evidence pack. Absent record types
// are honestly labelled "none recorded" rather than fabricated.
func buildEvidencePack(op models.Operation, events []models.OperationEvent, scan privacyfilter.ScanResult, telemetry []modelintelligence.ModelRunTelemetry, now time.Time) EvidencePack {
	var b strings.Builder
	w := func(format string, a ...any) { _, _ = fmt.Fprintf(&b, format, a...) }

	w("# Evidence Pack: %s\n\n", op.Title)

	w("## Operation\n")
	w("- id: %s\n- ownerIdentity: %s\n- workspaceId: %s\n- type: %s\n- status: %s\n- risk: %s\n- autonomy: %s\n- owner: %s\n- decision: %s\n- created: %s\n- updated: %s\n\n",
		op.ID, op.OwnerUserID, op.WorkspaceID, op.OperationType, op.Status, op.RiskLevel, op.AutonomyLevel, op.OwnerType, op.CurrentDecision,
		op.CreatedAt.UTC().Format(time.RFC3339), op.UpdatedAt.UTC().Format(time.RFC3339))

	w("## Source Evidence\n")
	w("- sourceType: %s\n- sourceUri: %s\n- sourceRevisionHash: %s\n- dedupeKey: %s\n\n",
		orNone(op.SourceType), orNone(op.SourceURI), orNone(op.SourceRevisionHash), orNone(op.DedupeKey))

	w("## Extracted Claims\n")
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
	w("- OperationEvents are the durable event log; disabled publishers do not change this pack's recorded provenance.\n")
	w("- Some sub-records are derived from operation fields and immutable operation events in this phase.\n")

	pack, err := normalizeEvidencePack(EvidencePack{
		ID:            uuid.New(),
		OwnerIdentity: op.OwnerUserID,
		WorkspaceID:   op.WorkspaceID,
		OperationID:   op.ID,
		Title:         op.Title,
		Markdown:      b.String(),
		Provenance: EvidenceProvenance{
			SourceType:         op.SourceType,
			SourceID:           op.SourceID,
			SourceURI:          op.SourceURI,
			SourceReceivedAt:   op.SourceReceivedAt,
			SourceRevisionHash: op.SourceRevisionHash,
			DedupeKey:          op.DedupeKey,
		},
		GeneratedAt: now,
	})
	if err != nil {
		// Persisted operations normally satisfy all required invariants. A
		// corrupt operation yields an invalid pack that repository validation
		// rejects rather than partially storing.
		return EvidencePack{}
	}
	return pack
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
