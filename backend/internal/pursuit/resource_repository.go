package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pursuitResourceRepository interface {
	AppendResourceEvent(event *models.PursuitResourceEvent) (*models.PursuitResourceEvent, bool, error)
	FindResourceEventsForOwner(ownerIdentity string, pursuitID uuid.UUID, limit int) ([]models.PursuitResourceEvent, error)
	SummarizeResourceEventsForOwner(ownerIdentity string, pursuitID uuid.UUID) (PursuitResourceTotals, error)
}

type pursuitResourceReservationRepository interface {
	ReserveResource(reservation *models.PursuitResourceReservation, activity *models.PursuitActivity) (*models.PursuitResourceReservation, bool, error)
	FindResourceReservation(ownerIdentity string, pursuitID uuid.UUID, operationID string) (*models.PursuitResourceReservation, error)
	FindResourceReservationByID(ownerIdentity string, pursuitID, reservationID uuid.UUID) (*models.PursuitResourceReservation, error)
	FindActiveResourceReservations(ownerIdentity string, pursuitID uuid.UUID, limit int) ([]models.PursuitResourceReservation, error)
	SettleResourceReservation(settlement *models.PursuitResourceReservationSettlement, events []models.PursuitResourceEvent, activities []models.PursuitActivity) (*models.PursuitResourceReservationSettlement, bool, error)
}

type pursuitResourceReservationSummaryRepository interface {
	SummarizeActiveResourceReservations(ownerIdentity string, pursuitID uuid.UUID) (PursuitResourceReservationTotals, error)
}

func (r *GormRepository) AppendResourceEvent(event *models.PursuitResourceEvent) (*models.PursuitResourceEvent, bool, error) {
	result := r.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "owner_identity"},
			{Name: "pursuit_id"},
			{Name: "idempotency_key"},
		},
		DoNothing: true,
	}).Create(event)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected > 0 {
		return event, true, nil
	}

	var existing models.PursuitResourceEvent
	if err := r.DB.Where(
		"owner_identity = ? AND pursuit_id = ? AND idempotency_key = ?",
		event.OwnerIdentity,
		event.PursuitID,
		event.IdempotencyKey,
	).First(&existing).Error; err != nil {
		return nil, false, err
	}
	if existing.RecordDigest != event.RecordDigest {
		return nil, false, fmt.Errorf("idempotency key was already used for a different pursuit resource event")
	}
	return &existing, false, nil
}

func (r *GormRepository) FindResourceEventsForOwner(ownerIdentity string, pursuitID uuid.UUID, limit int) ([]models.PursuitResourceEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	items := []models.PursuitResourceEvent{}
	err := r.DB.Where("owner_identity = ? AND pursuit_id = ?", ownerIdentity, pursuitID).
		Order("occurred_at DESC, recorded_at DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *GormRepository) SummarizeResourceEventsForOwner(ownerIdentity string, pursuitID uuid.UUID) (PursuitResourceTotals, error) {
	totals := PursuitResourceTotals{}
	err := r.DB.Model(&models.PursuitResourceEvent{}).
		Select(`
			COALESCE(SUM(CASE WHEN kind = 'effort_recorded' THEN effort_minutes ELSE 0 END), 0) AS effort_minutes,
			COALESCE(SUM(CASE WHEN kind = 'spend_incurred' THEN amount_minor ELSE 0 END), 0) AS incurred_minor,
			COALESCE(SUM(CASE WHEN kind = 'spend_refund' THEN amount_minor ELSE 0 END), 0) AS refunded_minor,
			COUNT(*) AS event_count,
			MAX(recorded_at) AS latest_recorded_at
		`).
		Where("owner_identity = ? AND pursuit_id = ?", ownerIdentity, pursuitID).
		Scan(&totals).Error
	return totals, err
}

func (r *GormRepository) ReserveResource(reservation *models.PursuitResourceReservation, activity *models.PursuitActivity) (*models.PursuitResourceReservation, bool, error) {
	var stored models.PursuitResourceReservation
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_identity"}, {Name: "pursuit_id"}, {Name: "operation_id"}},
			DoNothing: true,
		}).Create(reservation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where(
				"owner_identity = ? AND pursuit_id = ? AND operation_id = ?",
				reservation.OwnerIdentity, reservation.PursuitID, reservation.OperationID,
			).First(&stored).Error; err != nil {
				return err
			}
			if stored.RecordDigest != reservation.RecordDigest {
				return fmt.Errorf("resource reservation operation was already used with different estimates")
			}
			return nil
		}
		created = true
		stored = *reservation
		if activity != nil {
			return appendResourceActivities(tx, reservation.PursuitID, []models.PursuitActivity{*activity})
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (r *GormRepository) FindResourceReservation(ownerIdentity string, pursuitID uuid.UUID, operationID string) (*models.PursuitResourceReservation, error) {
	var reservation models.PursuitResourceReservation
	if err := r.DB.Where(
		"owner_identity = ? AND pursuit_id = ? AND operation_id = ?", ownerIdentity, pursuitID, operationID,
	).First(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *GormRepository) FindResourceReservationByID(ownerIdentity string, pursuitID, reservationID uuid.UUID) (*models.PursuitResourceReservation, error) {
	var reservation models.PursuitResourceReservation
	if err := r.DB.Where(
		"owner_identity = ? AND pursuit_id = ? AND id = ?", ownerIdentity, pursuitID, reservationID,
	).First(&reservation).Error; err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *GormRepository) FindActiveResourceReservations(ownerIdentity string, pursuitID uuid.UUID, limit int) ([]models.PursuitResourceReservation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	reservations := []models.PursuitResourceReservation{}
	err := r.DB.Model(&models.PursuitResourceReservation{}).Table("pursuit_resource_reservations AS r").
		Select("r.*").
		Joins("LEFT JOIN pursuit_resource_reservation_settlements AS s ON s.reservation_id = r.id").
		Where("r.owner_identity = ? AND r.pursuit_id = ? AND s.id IS NULL", ownerIdentity, pursuitID).
		Order("r.reserved_at ASC, r.id ASC").
		Limit(limit).
		Scan(&reservations).Error
	return reservations, err
}

func (r *GormRepository) SummarizeActiveResourceReservations(ownerIdentity string, pursuitID uuid.UUID) (PursuitResourceReservationTotals, error) {
	totals := PursuitResourceReservationTotals{}
	err := r.DB.Model(&models.PursuitResourceReservation{}).Table("pursuit_resource_reservations AS r").
		Select(`
			COALESCE(SUM(r.estimated_effort_minutes), 0) AS effort_minutes,
			COALESCE(SUM(r.estimated_cost_micros), 0) AS cost_micros,
			COUNT(*) AS reservation_count,
			MAX(r.reserved_at) AS latest_reserved_at
		`).
		Joins("LEFT JOIN pursuit_resource_reservation_settlements AS s ON s.reservation_id = r.id").
		Where("r.owner_identity = ? AND r.pursuit_id = ? AND s.id IS NULL", ownerIdentity, pursuitID).
		Scan(&totals).Error
	return totals, err
}

func (r *GormRepository) SettleResourceReservation(
	settlement *models.PursuitResourceReservationSettlement,
	events []models.PursuitResourceEvent,
	activities []models.PursuitActivity,
) (*models.PursuitResourceReservationSettlement, bool, error) {
	var stored models.PursuitResourceReservationSettlement
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "reservation_id"}},
			DoNothing: true,
		}).Create(settlement)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where("reservation_id = ?", settlement.ReservationID).First(&stored).Error; err != nil {
				return err
			}
			if stored.RecordDigest != settlement.RecordDigest {
				return fmt.Errorf("resource reservation was already settled with a different outcome")
			}
			return nil
		}
		created = true
		stored = *settlement
		for index := range events {
			if err := tx.Create(&events[index]).Error; err != nil {
				return err
			}
		}
		return appendResourceActivities(tx, settlement.PursuitID, activities)
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func appendResourceActivities(tx *gorm.DB, pursuitID uuid.UUID, activities []models.PursuitActivity) error {
	var latest time.Time
	for index := range activities {
		if activities[index].PursuitID != pursuitID {
			return fmt.Errorf("resource activity pursuit does not match the authoritative ledger record")
		}
		if activities[index].CreatedAt.IsZero() {
			activities[index].CreatedAt = time.Now().UTC()
		}
		if err := tx.Create(&activities[index]).Error; err != nil {
			return err
		}
		if activities[index].CreatedAt.After(latest) {
			latest = activities[index].CreatedAt
		}
	}
	if latest.IsZero() {
		return nil
	}
	return tx.Model(&models.Pursuit{}).
		Where("id = ? AND (last_activity_at IS NULL OR last_activity_at < ?)", pursuitID, latest).
		Update("last_activity_at", latest).Error
}
