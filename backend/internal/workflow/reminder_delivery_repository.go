package workflow

import (
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) FindOrCreateReminderDeliveryAuthorization(wanted *models.WorkflowReminderDeliveryAuthorization) (*models.WorkflowReminderDeliveryAuthorization, bool, error) {
	if r == nil || r.DB == nil || wanted == nil {
		return nil, false, fmt.Errorf("reminder delivery authorization is required")
	}
	stored := models.WorkflowReminderDeliveryAuthorization{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		activation, latest, err := loadReminderActivationRequest(tx, wanted.OwnerIdentity, wanted.ActivationRequestID, true)
		if err != nil {
			return err
		}
		if activation == nil || latest == nil || latest.ID != wanted.ActivationDecisionID || latest.Decision != ReminderActivationDecisionApproved ||
			activation.RecordDigest != wanted.ActivationRequestDigest || latest.RecordDigest != wanted.ActivationDecisionDigest {
			return fmt.Errorf("reminder delivery authorization lost its approval precondition")
		}
		source, err := loadReminderActivationSource(tx, wanted.OwnerIdentity, wanted.ChecklistItemID, true)
		if err != nil || source == nil {
			return firstReminderActivationError(err, "reminder delivery source is unavailable")
		}
		if digest, digestErr := reminderEvidenceDigest(*source); digestErr != nil || digest != wanted.ReminderDigest {
			return fmt.Errorf("reminder delivery source changed")
		}
		sourceBound := models.WorkflowReminderDeliveryAuthorization{}
		err = tx.Where(
			"owner_identity = ? AND activation_request_id = ? AND activation_decision_id = ? AND channel = ?",
			wanted.OwnerIdentity, wanted.ActivationRequestID, wanted.ActivationDecisionID, wanted.Channel,
		).First(&sourceBound).Error
		if err == nil {
			if sourceBound.IdempotencyKey != wanted.IdempotencyKey || sourceBound.RequestDigest != wanted.RequestDigest {
				return fmt.Errorf("approved reminder preparation already has a different delivery authorization")
			}
			stored = sourceBound
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "owner_identity"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(wanted)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			stored = *wanted
			created = true
			return nil
		}
		if err := tx.Where("owner_identity = ? AND idempotency_key = ?", wanted.OwnerIdentity, wanted.IdempotencyKey).First(&stored).Error; err != nil {
			return err
		}
		if stored.RequestDigest != wanted.RequestDigest ||
			stored.ActivationRequestID != wanted.ActivationRequestID ||
			stored.ActivationDecisionID != wanted.ActivationDecisionID ||
			stored.ReminderDigest != wanted.ReminderDigest ||
			stored.Channel != wanted.Channel {
			return fmt.Errorf("reminder delivery idempotency key is bound to different evidence")
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (r *GormRepository) FindDueReminderDeliveryAuthorizations(owner string, now time.Time, limit, maxAttempts int) ([]reminderDeliveryCandidate, error) {
	if r == nil || r.DB == nil || limit < 1 || maxAttempts < 1 {
		return nil, fmt.Errorf("valid reminder delivery query is required")
	}
	type row struct {
		models.WorkflowReminderDeliveryAuthorization
		AttemptCount int `gorm:"column:attempt_count"`
	}
	rows := []row{}
	query := r.DB.Table("workflow_reminder_delivery_authorizations AS authorization").
		Select("authorization.*, COUNT(attempt.id) AS attempt_count").
		Joins("LEFT JOIN workflow_reminder_delivery_attempts AS attempt ON attempt.authorization_id = authorization.id").
		Where("authorization.reminder_at <= ? AND authorization.expires_at >= ?", now, now).
		Where("NOT EXISTS (SELECT 1 FROM workflow_reminder_delivery_attempts final_attempt WHERE final_attempt.authorization_id = authorization.id AND final_attempt.status IN ?)", []string{ReminderDeliveryStatusDelivered, ReminderDeliveryStatusSuppressed, ReminderDeliveryStatusDeadLettered}).
		Group("authorization.id").Having("COUNT(attempt.id) < ?", maxAttempts).
		Order("authorization.reminder_at ASC, authorization.id ASC").Limit(limit)
	if strings.TrimSpace(owner) != "" {
		query = query.Where("authorization.owner_identity = ?", strings.TrimSpace(owner))
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]reminderDeliveryCandidate, 0, len(rows))
	for _, value := range rows {
		result = append(result, reminderDeliveryCandidate{Authorization: value.WorkflowReminderDeliveryAuthorization, AttemptCount: value.AttemptCount})
	}
	return result, nil
}

func (r *GormRepository) SaveReminderDeliveryAttempt(wanted *models.WorkflowReminderDeliveryAttempt) (*models.WorkflowReminderDeliveryAttempt, bool, error) {
	if r == nil || r.DB == nil || wanted == nil {
		return nil, false, fmt.Errorf("reminder delivery attempt is required")
	}
	stored := models.WorkflowReminderDeliveryAttempt{}
	result := r.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "authorization_id"}, {Name: "attempt_number"}}, DoNothing: true}).Create(wanted)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		copy := *wanted
		return &copy, true, nil
	}
	if err := r.DB.Where("authorization_id = ? AND attempt_number = ?", wanted.AuthorizationID, wanted.AttemptNumber).First(&stored).Error; err != nil {
		return nil, false, err
	}
	if stored.Status != wanted.Status || stored.Reason != wanted.Reason ||
		stored.ReminderDigest != wanted.ReminderDigest ||
		stored.AuthorizationDigest != wanted.AuthorizationDigest ||
		stored.Authority != wanted.Authority {
		return nil, false, fmt.Errorf("reminder delivery attempt number is bound to different evidence")
	}
	return &stored, false, nil
}

func (r *GormRepository) ListReminderDeliveryAuthorizationsForOwner(owner string, limit int) ([]models.WorkflowReminderDeliveryAuthorization, error) {
	items := []models.WorkflowReminderDeliveryAuthorization{}
	err := r.DB.Where("owner_identity = ?", strings.TrimSpace(owner)).Order("authorized_at DESC, id DESC").Limit(limit).Find(&items).Error
	return items, err
}

func (r *GormRepository) ListReminderDeliveryAttemptsForOwner(owner string, limit int) ([]models.WorkflowReminderDeliveryAttempt, error) {
	items := []models.WorkflowReminderDeliveryAttempt{}
	err := r.DB.Where("owner_identity = ?", strings.TrimSpace(owner)).Order("attempted_at DESC, id DESC").Limit(limit).Find(&items).Error
	return items, err
}
