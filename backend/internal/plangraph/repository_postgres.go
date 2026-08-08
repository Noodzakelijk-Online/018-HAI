package plangraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type planRevisionRow struct {
	RowID          uuid.UUID  `gorm:"column:row_id;type:uuid;primaryKey"`
	PlanID         uuid.UUID  `gorm:"column:plan_id;type:uuid;not null"`
	OwnerIdentity  string     `gorm:"column:owner_identity;type:varchar(255);not null"`
	Revision       uint64     `gorm:"column:revision;not null"`
	Status         string     `gorm:"column:status;type:varchar(24);not null"`
	Digest         string     `gorm:"column:digest;type:char(64);not null"`
	ParentRevision uint64     `gorm:"column:parent_revision;not null"`
	ParentDigest   string     `gorm:"column:parent_digest;type:varchar(64);not null"`
	IdempotencyKey string     `gorm:"column:idempotency_key;type:text;not null"`
	PayloadJSON    []byte     `gorm:"column:payload;type:jsonb;not null"`
	CreatedBy      string     `gorm:"column:created_by;type:varchar(255);not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	AcceptedAt     *time.Time `gorm:"column:accepted_at"`
}

func (planRevisionRow) TableName() string { return "plan_graph_revisions" }

type GormRepository struct{ db *gorm.DB }

var _ Repository = (*GormRepository)(nil)

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (repository *GormRepository) CreateRevision(ctx context.Context, plan Plan, expectedPreviousRevision uint64) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("plan graph database is required")
	}
	row, err := planToRow(plan)
	if err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latest planRevisionRow
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_identity = ? AND plan_id = ?", row.OwnerIdentity, row.PlanID).
			Order("revision DESC").First(&latest)
		currentRevision := uint64(0)
		if query.Error == nil {
			currentRevision = latest.Revision
		} else if !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return query.Error
		}
		if currentRevision != expectedPreviousRevision || row.Revision != currentRevision+1 {
			return ErrRevisionConflict
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			if isConstraintConflict(result.Error) {
				return classifyConstraintConflict(result.Error)
			}
			return result.Error
		}
		if result.RowsAffected != 1 {
			if row.IdempotencyKey != "" {
				return ErrIdempotencyConflict
			}
			return ErrRevisionConflict
		}
		return nil
	})
}

func (repository *GormRepository) GetLatest(ctx context.Context, ownerIdentity string, id uuid.UUID) (*Plan, error) {
	return repository.get(ctx, ownerIdentity, id, 0)
}

func (repository *GormRepository) GetRevision(ctx context.Context, ownerIdentity string, id uuid.UUID, revision uint64) (*Plan, error) {
	if revision == 0 {
		return nil, ErrNotFound
	}
	return repository.get(ctx, ownerIdentity, id, revision)
}

func (repository *GormRepository) get(ctx context.Context, ownerIdentity string, id uuid.UUID, revision uint64) (*Plan, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("plan graph database is required")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || id == uuid.Nil {
		return nil, ErrNotFound
	}
	query := repository.db.WithContext(ctx).
		Where("owner_identity = ? AND plan_id = ?", ownerIdentity, id)
	if revision > 0 {
		query = query.Where("revision = ?", revision)
	} else {
		query = query.Order("revision DESC")
	}
	var row planRevisionRow
	if err := query.First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	value, err := rowToPlan(row)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (repository *GormRepository) FindByIdempotencyKey(ctx context.Context, ownerIdentity, key string) (*Plan, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("plan graph database is required")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	key = strings.TrimSpace(key)
	if ownerIdentity == "" || key == "" {
		return nil, ErrNotFound
	}
	var row planRevisionRow
	err := repository.db.WithContext(ctx).
		Where("owner_identity = ? AND idempotency_key = ?", ownerIdentity, key).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value, err := rowToPlan(row)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (repository *GormRepository) ListLatest(ctx context.Context, ownerIdentity string) ([]Plan, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("plan graph database is required")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	var rows []planRevisionRow
	if err := repository.db.WithContext(ctx).
		Raw(`SELECT DISTINCT ON (plan_id) * FROM plan_graph_revisions
			WHERE owner_identity = ? ORDER BY plan_id, revision DESC`, ownerIdentity).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Plan, 0, len(rows))
	for _, row := range rows {
		value, err := rowToPlan(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func planToRow(plan Plan) (planRevisionRow, error) {
	plan = normalizePlan(plan)
	if err := validateStoredPlan(plan); err != nil {
		return planRevisionRow{}, err
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return planRevisionRow{}, fmt.Errorf("encode plan graph payload: %w", err)
	}
	return planRevisionRow{
		RowID: uuid.New(), PlanID: plan.ID, OwnerIdentity: plan.OwnerIdentity,
		Revision: plan.Revision, Status: string(plan.Status), Digest: plan.Digest,
		ParentRevision: plan.ParentRevision, ParentDigest: plan.ParentDigest,
		IdempotencyKey: plan.IdempotencyKey, PayloadJSON: payload,
		CreatedBy: plan.CreatedBy, CreatedAt: plan.CreatedAt, AcceptedAt: plan.AcceptedAt,
	}, nil
}

func rowToPlan(row planRevisionRow) (Plan, error) {
	var value Plan
	if err := json.Unmarshal(row.PayloadJSON, &value); err != nil {
		return Plan{}, fmt.Errorf("decode plan graph payload: %w", err)
	}
	if value.CanExecute {
		return Plan{}, fmt.Errorf("persisted plan graph cannot grant execution authority")
	}
	value.OwnerIdentity = row.OwnerIdentity
	value = normalizePlan(value)
	if err := validateRowMetadata(row, value); err != nil {
		return Plan{}, err
	}
	return value, validatePersistedPlan(value)
}

func validateRowMetadata(row planRevisionRow, value Plan) error {
	rowDigest := strings.ToLower(strings.TrimSpace(row.Digest))
	rowParentDigest := strings.ToLower(strings.TrimSpace(row.ParentDigest))
	if value.ID != row.PlanID ||
		value.Revision != row.Revision ||
		value.Status != Status(strings.TrimSpace(row.Status)) ||
		value.Digest != rowDigest ||
		value.ParentRevision != row.ParentRevision ||
		value.ParentDigest != rowParentDigest ||
		value.IdempotencyKey != strings.TrimSpace(row.IdempotencyKey) ||
		value.CreatedBy != strings.TrimSpace(row.CreatedBy) ||
		!value.CreatedAt.Equal(canonicalTime(row.CreatedAt)) ||
		!equalCanonicalTimePointers(value.AcceptedAt, row.AcceptedAt) {
		return fmt.Errorf("persisted plan graph metadata mismatch")
	}
	return nil
}

func equalCanonicalTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return canonicalTime(*left).Equal(canonicalTime(*right))
}

func validatePersistedPlan(plan Plan) error {
	if err := validateStoredPlan(normalizePlan(plan)); err != nil {
		return fmt.Errorf("persisted plan graph is invalid: %w", err)
	}
	return nil
}

func validateStoredPlan(plan Plan) error {
	if err := validatePlan(normalizePlan(plan)); err != nil {
		return err
	}
	if !validDigest(plan.Digest) {
		return fmt.Errorf("plan graph digest must be a lowercase SHA-256 digest")
	}
	digest, err := computeDigest(plan)
	if err != nil {
		return err
	}
	if digest != plan.Digest {
		return fmt.Errorf("plan graph digest mismatch")
	}
	return nil
}

func isConstraintConflict(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func classifyConstraintConflict(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "idempotency") {
		return ErrIdempotencyConflict
	}
	return ErrRevisionConflict
}
