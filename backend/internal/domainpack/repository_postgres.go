package domainpack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormPreferenceRepository persists owner overlays while the canonical
// catalog remains code-owned.
type GormPreferenceRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewGormPreferenceRepository(db *gorm.DB, now func() time.Time) *GormPreferenceRepository {
	if now == nil {
		now = time.Now
	}
	return &GormPreferenceRepository{db: db, now: now}
}

func DefaultPreferenceRepository() (PreferenceRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormPreferenceRepository(db, time.Now), nil
}

func (repository *GormPreferenceRepository) Upsert(preference PackPreference) (PackPreference, error) {
	if repository == nil || repository.db == nil {
		return PackPreference{}, fmt.Errorf("domain pack preference database is required")
	}
	owner := strings.TrimSpace(preference.OwnerIdentity)
	packID := preference.PackID
	var stored models.DomainPackPreference
	err := repository.db.Transaction(func(tx *gorm.DB) error {
		var existingRow models.DomainPackPreference
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_identity = ? AND pack_id = ?", owner, packID).
			First(&existingRow)
		exists := query.Error == nil
		if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return query.Error
		}

		var existing PackPreference
		var err error
		if exists {
			existing, err = preferenceFromModel(existingRow)
			if err != nil {
				return err
			}
		}
		normalized, err := normalizePreference(preference, existing, exists, repository.now().UTC())
		if err != nil {
			return err
		}
		row, err := preferenceToModel(normalized)
		if err != nil {
			return err
		}
		if exists {
			row.ID = existingRow.ID
			if err := tx.Model(&models.DomainPackPreference{}).
				Where("id = ? AND owner_identity = ? AND revision = ?", row.ID, owner, existingRow.Revision).
				Updates(map[string]any{
					"catalog_version":      row.CatalogVersion,
					"revision":             row.Revision,
					"status":               row.Status,
					"enabled":              row.Enabled,
					"classification_boost": row.ClassificationBoost,
					"force_local_only":     row.ForceLocalOnly,
					"adaptations_json":     row.AdaptationsJSON,
					"updated_at":           row.UpdatedAt,
				}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&row).Error; err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return PackPreference{}, ErrPreferenceConflict
		}
		return PackPreference{}, err
	}
	return preferenceFromModel(stored)
}

func (repository *GormPreferenceRepository) Get(
	ownerIdentity string,
	packID PackID,
) (PackPreference, bool, error) {
	if repository == nil || repository.db == nil {
		return PackPreference{}, false, fmt.Errorf("domain pack preference database is required")
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return PackPreference{}, false, fmt.Errorf("owner identity is required")
	}
	var row models.DomainPackPreference
	err := repository.db.
		Where("owner_identity = ? AND pack_id = ?", owner, packID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PackPreference{}, false, nil
	}
	if err != nil {
		return PackPreference{}, false, err
	}
	value, err := preferenceFromModel(row)
	return value, err == nil, err
}

func (repository *GormPreferenceRepository) List(ownerIdentity string) ([]PackPreference, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("domain pack preference database is required")
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	var rows []models.DomainPackPreference
	if err := repository.db.
		Where("owner_identity = ?", owner).
		Order("pack_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PackPreference, 0, len(rows))
	for _, row := range rows {
		value, err := preferenceFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (repository *GormPreferenceRepository) Delete(ownerIdentity string, packID PackID) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("domain pack preference database is required")
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return fmt.Errorf("owner identity is required")
	}
	return repository.db.
		Where("owner_identity = ? AND pack_id = ?", owner, packID).
		Delete(&models.DomainPackPreference{}).Error
}

func preferenceToModel(value PackPreference) (models.DomainPackPreference, error) {
	adaptationJSON, err := json.Marshal(value.Adaptation)
	if err != nil {
		return models.DomainPackPreference{}, fmt.Errorf("encode domain pack adaptation: %w", err)
	}
	return models.DomainPackPreference{
		ID:                  uuid.New(),
		OwnerIdentity:       value.OwnerIdentity,
		PackID:              string(value.PackID),
		CatalogVersion:      value.CatalogVersion,
		Revision:            value.Revision,
		Status:              string(value.Status),
		Enabled:             cloneBool(value.Enabled),
		ClassificationBoost: value.ClassificationBoost,
		ForceLocalOnly:      value.ForceLocalOnly,
		AdaptationsJSON:     string(adaptationJSON),
		CreatedAt:           value.CreatedAt.UTC(),
		UpdatedAt:           value.UpdatedAt.UTC(),
	}, nil
}

func preferenceFromModel(row models.DomainPackPreference) (PackPreference, error) {
	var adaptation PackAdaptation
	if err := decodePersistedJSON(row.AdaptationsJSON, &adaptation); err != nil {
		return PackPreference{}, fmt.Errorf("decode domain pack adaptation: %w", err)
	}
	value := PackPreference{
		OwnerIdentity:       row.OwnerIdentity,
		PackID:              PackID(row.PackID),
		CatalogVersion:      row.CatalogVersion,
		Revision:            row.Revision,
		Status:              PreferenceStatus(row.Status),
		Enabled:             cloneBool(row.Enabled),
		ClassificationBoost: row.ClassificationBoost,
		ForceLocalOnly:      row.ForceLocalOnly,
		Adaptation:          adaptation,
		CreatedAt:           row.CreatedAt.UTC(),
		UpdatedAt:           row.UpdatedAt.UTC(),
	}
	if !validPreferenceStatus(value.Status) {
		return PackPreference{}, fmt.Errorf("persisted domain pack preference has invalid status %q", value.Status)
	}
	return value, nil
}

func decodePersistedJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("persisted JSON must contain one object")
	}
	return nil
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique constraint")
}
