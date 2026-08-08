package controlledlearning

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const ledgerPromoterID = "hai-controlled-learning-ledger-v1"

// LedgerPromoter activates a reviewed change in HAI's durable learning ledger.
// A target-specific promoter can replace it when deployment outside HAI is
// required.
type LedgerPromoter struct {
	now func() time.Time
}

func NewLedgerPromoter(now func() time.Time) *LedgerPromoter {
	return &LedgerPromoter{now: now}
}

func (promoter *LedgerPromoter) ID() string {
	return ledgerPromoterID
}

func (promoter *LedgerPromoter) Apply(
	ctx context.Context,
	request PromotionRequest,
) (PromotionResult, error) {
	if err := ctx.Err(); err != nil {
		return PromotionResult{}, err
	}
	digest, err := digestValue(request)
	if err != nil {
		return PromotionResult{}, err
	}
	return PromotionResult{
		AppliedVersion: request.ProposedVersion,
		RollbackToken: fmt.Sprintf(
			"ledger:%s:%s",
			request.ApplicationID,
			request.CurrentVersion,
		),
		Evidence: []ApplicationEvidence{promoter.evidence(
			request.ApplicationID,
			"ledger_application",
			digest,
		)},
	}, nil
}

func (promoter *LedgerPromoter) HandoffProtected(
	ctx context.Context,
	request ProtectedHandoffRequest,
) (ProtectedHandoffResult, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedHandoffResult{}, err
	}
	digest, err := digestValue(request)
	if err != nil {
		return ProtectedHandoffResult{}, err
	}
	return ProtectedHandoffResult{
		HandoffReference: "ledger-governance:" + request.ApplicationID,
		Evidence: []ApplicationEvidence{promoter.evidence(
			request.ApplicationID,
			"governance_handoff",
			digest,
		)},
	}, nil
}

func (promoter *LedgerPromoter) Rollback(
	ctx context.Context,
	request PromotionRollbackRequest,
) (PromotionRollbackResult, error) {
	if err := ctx.Err(); err != nil {
		return PromotionRollbackResult{}, err
	}
	digest, err := digestValue(request)
	if err != nil {
		return PromotionRollbackResult{}, err
	}
	return PromotionRollbackResult{
		RestoredVersion: request.RestoreVersion,
		Evidence: []ApplicationEvidence{promoter.evidence(
			request.ApplicationID,
			"ledger_rollback",
			digest,
		)},
	}, nil
}

func (promoter *LedgerPromoter) evidence(
	applicationID string,
	kind string,
	digest string,
) ApplicationEvidence {
	recordedAt := time.Now().UTC()
	if promoter != nil && promoter.now != nil {
		recordedAt = promoter.now().UTC()
	}
	return ApplicationEvidence{
		ID:         kind + "-receipt",
		Kind:       kind,
		URI:        "hai://controlled-learning/applications/" + strings.TrimSpace(applicationID),
		Digest:     digest,
		RecordedAt: recordedAt,
	}
}
