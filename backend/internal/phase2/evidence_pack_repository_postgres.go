package phase2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type evidencePackRow struct {
	ID                 uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	OwnerIdentity      string     `gorm:"column:owner_identity"`
	WorkspaceID        string     `gorm:"column:workspace_id"`
	OperationID        uuid.UUID  `gorm:"column:operation_id;type:uuid"`
	Title              string     `gorm:"column:title"`
	Markdown           string     `gorm:"column:markdown"`
	SourceType         string     `gorm:"column:source_type"`
	SourceID           *uuid.UUID `gorm:"column:source_id;type:uuid"`
	SourceURI          string     `gorm:"column:source_uri"`
	SourceReceivedAt   *time.Time `gorm:"column:source_received_at"`
	SourceRevisionHash string     `gorm:"column:source_revision_hash"`
	DedupeKey          string     `gorm:"column:dedupe_key"`
	ContentDigest      string     `gorm:"column:content_digest"`
	GeneratedAt        time.Time  `gorm:"column:generated_at"`
}

func (evidencePackRow) TableName() string { return "evidence_packs" }

// GormEvidencePackRepository persists immutable evidence packs in Postgres.
type GormEvidencePackRepository struct {
	db *gorm.DB
}

func NewGormEvidencePackRepository(db *gorm.DB) *GormEvidencePackRepository {
	return &GormEvidencePackRepository{db: db}
}

// DefaultEvidencePackRepository opens the migrated application database.
// Errors are returned to the module and never replaced with volatile storage.
func DefaultEvidencePackRepository() (EvidencePackRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEvidencePackRepositoryUnavailable, err)
	}
	return NewGormEvidencePackRepository(db), nil
}

func (repository *GormEvidencePackRepository) Create(
	ctx context.Context,
	pack EvidencePack,
) (EvidencePack, error) {
	if repository == nil || repository.db == nil {
		return EvidencePack{}, ErrEvidencePackRepositoryUnavailable
	}
	normalized, err := normalizeEvidencePack(pack)
	if err != nil {
		return EvidencePack{}, err
	}
	row := evidencePackToRow(normalized)
	if err := repository.db.WithContext(ctx).Create(&row).Error; err != nil {
		return EvidencePack{}, fmt.Errorf("persist evidence pack: %w", err)
	}
	return evidencePackFromRow(row), nil
}

func (repository *GormEvidencePackRepository) Get(
	ctx context.Context,
	ownerIdentity string,
	workspaceID string,
	id uuid.UUID,
) (EvidencePack, error) {
	if repository == nil || repository.db == nil {
		return EvidencePack{}, ErrEvidencePackRepositoryUnavailable
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	if ownerIdentity == "" || workspaceID == "" || id == uuid.Nil {
		return EvidencePack{}, ErrEvidencePackNotFound
	}
	var row evidencePackRow
	err := repository.db.WithContext(ctx).
		Where("id = ? AND owner_identity = ? AND workspace_id = ?", id, ownerIdentity, workspaceID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EvidencePack{}, ErrEvidencePackNotFound
	}
	if err != nil {
		return EvidencePack{}, fmt.Errorf("load evidence pack: %w", err)
	}
	pack := evidencePackFromRow(row)
	persistedDigest := pack.ContentDigest
	normalized, err := normalizeEvidencePack(pack)
	if err != nil ||
		normalized.ID != pack.ID ||
		normalized.OwnerIdentity != pack.OwnerIdentity ||
		normalized.WorkspaceID != pack.WorkspaceID ||
		normalized.Title != pack.Title ||
		normalized.ContentDigest != persistedDigest {
		return EvidencePack{}, ErrEvidencePackIntegrityViolation
	}
	return pack, nil
}

func evidencePackToRow(pack EvidencePack) evidencePackRow {
	return evidencePackRow{
		ID:                 pack.ID,
		OwnerIdentity:      pack.OwnerIdentity,
		WorkspaceID:        pack.WorkspaceID,
		OperationID:        pack.OperationID,
		Title:              pack.Title,
		Markdown:           pack.Markdown,
		SourceType:         pack.Provenance.SourceType,
		SourceID:           pack.Provenance.SourceID,
		SourceURI:          pack.Provenance.SourceURI,
		SourceReceivedAt:   pack.Provenance.SourceReceivedAt,
		SourceRevisionHash: pack.Provenance.SourceRevisionHash,
		DedupeKey:          pack.Provenance.DedupeKey,
		ContentDigest:      pack.ContentDigest,
		GeneratedAt:        pack.GeneratedAt,
	}
}

func evidencePackFromRow(row evidencePackRow) EvidencePack {
	return EvidencePack{
		ID:            row.ID,
		OwnerIdentity: row.OwnerIdentity,
		WorkspaceID:   row.WorkspaceID,
		OperationID:   row.OperationID,
		Title:         row.Title,
		Markdown:      row.Markdown,
		Provenance: EvidenceProvenance{
			SourceType:         row.SourceType,
			SourceID:           row.SourceID,
			SourceURI:          row.SourceURI,
			SourceReceivedAt:   row.SourceReceivedAt,
			SourceRevisionHash: row.SourceRevisionHash,
			DedupeKey:          row.DedupeKey,
		},
		ContentDigest: row.ContentDigest,
		GeneratedAt:   row.GeneratedAt.UTC(),
	}
}
