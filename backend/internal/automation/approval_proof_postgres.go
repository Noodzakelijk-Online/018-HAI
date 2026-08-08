package automation

import (
	"context"
	"fmt"

	"automation-hub-backend/internal/infra"

	"gorm.io/gorm"
)

// PostgresApprovalProofConsumptionStore is the production replay boundary for
// short-lived approval capabilities. The database primary key makes claiming a
// proof atomic across goroutines, restarts, and backend instances.
type PostgresApprovalProofConsumptionStore struct {
	DB *gorm.DB
}

var _ ApprovalProofConsumptionStore = (*PostgresApprovalProofConsumptionStore)(nil)

func NewPostgresApprovalProofConsumptionStore(db *gorm.DB) *PostgresApprovalProofConsumptionStore {
	return &PostgresApprovalProofConsumptionStore{DB: db}
}

func DefaultDurableApprovalProofService(
	secret []byte,
) (ApprovalProofService, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("open approval proof consumption database: %w", err)
	}
	return NewApprovalProofService(
		secret,
		NewPostgresApprovalProofConsumptionStore(db),
		nil,
	)
}

func (s *PostgresApprovalProofConsumptionStore) Consume(
	ctx context.Context,
	value ApprovalProofConsumption,
) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("approval proof consumption database is required")
	}
	if ctx == nil {
		return fmt.Errorf("approval proof consumption context is required")
	}
	if err := validateApprovalProofConsumption(value); err != nil {
		return err
	}
	result := s.DB.WithContext(ctx).Exec(`
		INSERT INTO public.automation_approval_proof_consumptions (
			contract_version, owner_identity, proof_id, automation_id,
			action_digest, scope, approval_source_id, nonce_digest,
			signature_digest, record_digest, issued_at, expires_at,
			consumed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (owner_identity, proof_id) DO NOTHING`,
		value.ContractVersion,
		value.OwnerIdentity,
		value.ProofID,
		value.AutomationID,
		value.ActionDigest,
		string(value.Scope),
		value.ApprovalSourceID,
		value.NonceDigest,
		value.SignatureDigest,
		value.RecordDigest,
		value.IssuedAt.UTC(),
		value.ExpiresAt.UTC(),
		value.ConsumedAt.UTC(),
	)
	if result.Error != nil {
		return fmt.Errorf("consume approval proof: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrApprovalProofConsumed
	}
	return nil
}
