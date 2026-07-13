package modelintelligence

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"gorm.io/gorm"
)

// TelemetryRepository persists model-run telemetry durably (§18: "records
// durable model telemetry"). Telemetry survives restart.
type TelemetryRepository interface {
	Save(t ModelRunTelemetry) error
	LoadAll() ([]ModelRunTelemetry, error)
}

// GormTelemetryRepository is the Postgres-backed telemetry repository.
type GormTelemetryRepository struct{ DB *gorm.DB }

// NewGormTelemetryRepository builds a repository over db.
func NewGormTelemetryRepository(db *gorm.DB) *GormTelemetryRepository {
	return &GormTelemetryRepository{DB: db}
}

// DefaultTelemetryRepository builds a repository over the default DB, or returns
// nil (in-memory only) if no DB is available.
func DefaultTelemetryRepository() TelemetryRepository {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil
	}
	return NewGormTelemetryRepository(db)
}

func (r *GormTelemetryRepository) Save(t ModelRunTelemetry) error {
	return r.DB.Create(&models.ModelRunTelemetry{
		ID:              t.ID,
		ProviderID:      t.ProviderID,
		ModelID:         t.ModelID,
		Lane:            string(t.Lane),
		OperationID:     t.OperationID,
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		DurationMs:      t.DurationMs,
		TokensPerSecond: t.TokensPerSecond,
		OK:              t.OK,
		CacheHit:        t.CacheHit,
		CreatedAt:       t.CreatedAt,
	}).Error
}

func (r *GormTelemetryRepository) LoadAll() ([]ModelRunTelemetry, error) {
	var rows []models.ModelRunTelemetry
	if err := r.DB.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ModelRunTelemetry, 0, len(rows))
	for _, row := range rows {
		out = append(out, ModelRunTelemetry{
			ID:              row.ID,
			ProviderID:      row.ProviderID,
			ModelID:         row.ModelID,
			Lane:            RoutingLane(row.Lane),
			OperationID:     row.OperationID,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			DurationMs:      row.DurationMs,
			TokensPerSecond: row.TokensPerSecond,
			OK:              row.OK,
			CacheHit:        row.CacheHit,
			CreatedAt:       row.CreatedAt,
		})
	}
	return out, nil
}
