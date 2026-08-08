package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PortfolioAllocationAccepted              = "accepted"
	PortfolioAllocationAcceptedNeedsApproval = "accepted_needs_approval"
	portfolioAllocationActivityType          = "pursuit.portfolio_allocation_accepted"
	portfolioAllocationActivitySource        = "pursuit_portfolio_allocation"
)

// FindPortfolioAllocationForOwner returns the immutable accepted allocation
// before any availability-sensitive replanning occurs. This is the durable
// replay lookup: an absent owner/plan pair is not an error.
func (r *GormRepository) FindPortfolioAllocationForOwner(
	ownerIdentity, planID string,
) (*models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, error) {
	if r == nil || r.DB == nil {
		return nil, nil, fmt.Errorf("pursuit portfolio allocation repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	planID = strings.TrimSpace(planID)
	if ownerIdentity == "" || !validPortfolioPlanID(planID) {
		return nil, nil, fmt.Errorf("valid portfolio allocation owner and plan id are required")
	}
	var allocation models.PursuitPortfolioAllocation
	err := r.DB.Where("owner_identity = ? AND plan_id = ?", ownerIdentity, planID).
		First(&allocation).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	items := []models.PursuitPortfolioAllocationItem{}
	if err := r.DB.Where("owner_identity = ? AND allocation_id = ?", ownerIdentity, allocation.ID).
		Order("scheduled_start ASC, pursuit_id ASC").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return &allocation, items, nil
}

// ListPortfolioAllocationsForOwner returns a bounded newest-first inspection
// view. Both parent and item queries are constrained by the verified owner.
func (r *GormRepository) ListPortfolioAllocationsForOwner(
	ownerIdentity string, limit int,
) ([]models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, error) {
	if r == nil || r.DB == nil {
		return nil, nil, fmt.Errorf("pursuit portfolio allocation repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || limit < 1 || limit > 100 {
		return nil, nil, fmt.Errorf("valid portfolio allocation owner and bounded limit are required")
	}
	allocations := []models.PursuitPortfolioAllocation{}
	if err := r.DB.Where("owner_identity = ?", ownerIdentity).
		Order("accepted_at DESC, id DESC").Limit(limit).Find(&allocations).Error; err != nil {
		return nil, nil, err
	}
	if len(allocations) == 0 {
		return allocations, []models.PursuitPortfolioAllocationItem{}, nil
	}
	allocationIDs := make([]uuid.UUID, 0, len(allocations))
	for _, allocation := range allocations {
		allocationIDs = append(allocationIDs, allocation.ID)
	}
	items := []models.PursuitPortfolioAllocationItem{}
	if err := r.DB.Where("owner_identity = ? AND allocation_id IN ?", ownerIdentity, allocationIDs).
		Order("allocation_id ASC, scheduled_start ASC, pursuit_id ASC").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return allocations, items, nil
}

// SavePortfolioAllocation atomically persists an owner-accepted allocation,
// its resource holds, and its pursuit audit entries. The operation is
// append-only and idempotent by owner plus plan ID. A replay is accepted only
// when every parent, item, and reservation digest is unchanged.
func (r *GormRepository) SavePortfolioAllocation(
	allocation *models.PursuitPortfolioAllocation,
	items []models.PursuitPortfolioAllocationItem,
	reservations []models.PursuitResourceReservation,
	activities []models.PursuitActivity,
) (*models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, bool, error) {
	if r == nil || r.DB == nil {
		return nil, nil, false, fmt.Errorf("pursuit portfolio allocation repository is unavailable")
	}
	if err := validatePortfolioAllocationAggregate(allocation, items, reservations, activities); err != nil {
		return nil, nil, false, err
	}

	var stored models.PursuitPortfolioAllocation
	storedItems := []models.PursuitPortfolioAllocationItem{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		if err := verifyPortfolioPursuitOwnership(tx, allocation.OwnerIdentity, items); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_identity"}, {Name: "plan_id"}},
			DoNothing: true,
		}).Create(allocation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return loadAndVerifyPortfolioAllocationReplay(
				tx, allocation, items, reservations, activities, &stored, &storedItems,
			)
		}

		created = true
		stored = *allocation
		for index := range reservations {
			if err := tx.Create(&reservations[index]).Error; err != nil {
				return fmt.Errorf("create portfolio resource reservation: %w", err)
			}
		}
		for index := range items {
			if err := tx.Create(&items[index]).Error; err != nil {
				return fmt.Errorf("create portfolio allocation item: %w", err)
			}
			storedItems = append(storedItems, items[index])
		}
		for index := range activities {
			if err := appendResourceActivities(tx, activities[index].PursuitID, []models.PursuitActivity{activities[index]}); err != nil {
				return fmt.Errorf("create portfolio allocation activity: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	sortPortfolioAllocationItems(storedItems)
	return &stored, storedItems, created, nil
}

func validatePortfolioAllocationAggregate(
	allocation *models.PursuitPortfolioAllocation,
	items []models.PursuitPortfolioAllocationItem,
	reservations []models.PursuitResourceReservation,
	activities []models.PursuitActivity,
) error {
	if allocation == nil {
		return fmt.Errorf("pursuit portfolio allocation is required")
	}
	if allocation.ID == uuid.Nil || strings.TrimSpace(allocation.OwnerIdentity) == "" ||
		strings.TrimSpace(allocation.OwnerIdentity) != allocation.OwnerIdentity {
		return fmt.Errorf("portfolio allocation requires a stable ID and normalized owner identity")
	}
	if !validPortfolioPlanID(allocation.PlanID) || strings.TrimSpace(allocation.PlanID) != allocation.PlanID {
		return fmt.Errorf("portfolio allocation plan id is invalid")
	}
	if !validPortfolioRecordDigest(allocation.RequestDigest) ||
		!validPortfolioRecordDigest(allocation.DecisionDigest) ||
		!validPortfolioRecordDigest(allocation.RecordDigest) {
		return fmt.Errorf("portfolio allocation digests must be lowercase SHA-256 values")
	}
	if allocation.Status != PortfolioAllocationAccepted && allocation.Status != PortfolioAllocationAcceptedNeedsApproval {
		return fmt.Errorf("portfolio allocation status must be accepted or accepted_needs_approval")
	}
	if allocation.DurationMode != "expected" && allocation.DurationMode != "conservative" {
		return fmt.Errorf("portfolio allocation duration mode must be expected or conservative")
	}
	if allocation.HorizonStart.IsZero() || !allocation.HorizonEnd.After(allocation.HorizonStart) || allocation.AcceptedAt.IsZero() {
		return fmt.Errorf("portfolio allocation requires a valid horizon and acceptance time")
	}
	if strings.TrimSpace(allocation.Actor) == "" || strings.TrimSpace(allocation.Actor) != allocation.Actor || len(allocation.Actor) > 255 {
		return fmt.Errorf("portfolio allocation requires a normalized actor")
	}
	if strings.TrimSpace(allocation.Confirmation) == "" || strings.TrimSpace(allocation.Confirmation) != allocation.Confirmation || len(allocation.Confirmation) > 255 {
		return fmt.Errorf("portfolio allocation requires explicit confirmation")
	}
	if len(items) == 0 || len(items) > 500 || len(reservations) != len(items) || len(activities) != len(items) {
		return fmt.Errorf("portfolio allocation requires one item, reservation, and activity per scheduled pursuit")
	}

	reservationsByID := make(map[uuid.UUID]models.PursuitResourceReservation, len(reservations))
	for _, reservation := range reservations {
		if reservation.ID == uuid.Nil {
			return fmt.Errorf("portfolio allocation reservation id is required")
		}
		if _, duplicate := reservationsByID[reservation.ID]; duplicate {
			return fmt.Errorf("portfolio allocation contains a duplicate reservation")
		}
		reservationsByID[reservation.ID] = reservation
	}
	activitiesByPursuit := make(map[uuid.UUID]models.PursuitActivity, len(activities))
	for _, activity := range activities {
		if activity.ID == uuid.Nil || activity.PursuitID == uuid.Nil || activity.CreatedAt.IsZero() {
			return fmt.Errorf("portfolio allocation activity requires stable identity and time")
		}
		if _, duplicate := activitiesByPursuit[activity.PursuitID]; duplicate {
			return fmt.Errorf("portfolio allocation contains duplicate pursuit activity")
		}
		activitiesByPursuit[activity.PursuitID] = activity
	}

	seenPursuits := make(map[uuid.UUID]struct{}, len(items))
	requiresApproval := false
	for _, item := range items {
		if item.ID == uuid.Nil || item.AllocationID != allocation.ID || item.PursuitID == uuid.Nil ||
			item.OwnerIdentity != allocation.OwnerIdentity || item.ReservationID == uuid.Nil {
			return fmt.Errorf("portfolio allocation item identity or owner binding is invalid")
		}
		if _, duplicate := seenPursuits[item.PursuitID]; duplicate {
			return fmt.Errorf("portfolio allocation contains a duplicate pursuit")
		}
		seenPursuits[item.PursuitID] = struct{}{}
		if item.ScheduledStart.Before(allocation.HorizonStart) || item.ScheduledEnd.After(allocation.HorizonEnd) ||
			item.DurationMinutes <= 0 || item.ScheduledEnd.Sub(item.ScheduledStart) != time.Duration(item.DurationMinutes)*time.Minute ||
			item.EstimatedCostMicros < 0 || item.CreatedAt.IsZero() {
			return fmt.Errorf("portfolio allocation item schedule or estimate is invalid")
		}
		if !validPortfolioRecordDigest(item.RecordDigest) {
			return fmt.Errorf("portfolio allocation item digest must be a lowercase SHA-256 value")
		}
		if item.RequiresApproval {
			requiresApproval = true
			if len(item.ApprovalReasons) == 0 {
				return fmt.Errorf("approval-required portfolio item must include at least one reason")
			}
		} else if len(item.ApprovalReasons) != 0 {
			return fmt.Errorf("portfolio item without an approval requirement cannot include approval reasons")
		}
		if len(item.ApprovalReasons) > 20 {
			return fmt.Errorf("portfolio allocation item contains too many approval reasons")
		}
		for _, reason := range item.ApprovalReasons {
			if strings.TrimSpace(reason) == "" || strings.TrimSpace(reason) != reason || len(reason) > 1000 {
				return fmt.Errorf("portfolio allocation approval reason is invalid")
			}
		}

		reservation, ok := reservationsByID[item.ReservationID]
		if !ok || reservation.PursuitID != item.PursuitID || reservation.OwnerIdentity != allocation.OwnerIdentity ||
			reservation.EstimatedEffortMinutes != item.DurationMinutes || reservation.EstimatedCostMicros != item.EstimatedCostMicros ||
			reservation.Actor != allocation.Actor || reservation.ReservedAt.IsZero() || !validPortfolioRecordDigest(reservation.RecordDigest) {
			return fmt.Errorf("portfolio allocation item does not match its resource reservation")
		}
		if strings.TrimSpace(reservation.OperationID) == "" || len(reservation.OperationID) > 160 || strings.TrimSpace(reservation.Reason) == "" {
			return fmt.Errorf("portfolio allocation reservation metadata is invalid")
		}
		if item.EstimatedCostMicros > 0 && reservation.Currency != "EUR" || item.EstimatedCostMicros == 0 && reservation.Currency != "" {
			return fmt.Errorf("portfolio allocation reservation currency is invalid")
		}

		activity, ok := activitiesByPursuit[item.PursuitID]
		if !ok || activity.EventType != portfolioAllocationActivityType || activity.Actor != allocation.Actor ||
			activity.SourceType != portfolioAllocationActivitySource || activity.SourceID != allocation.ID.String() {
			return fmt.Errorf("portfolio allocation item does not match its pursuit activity")
		}
	}
	if requiresApproval && allocation.Status != PortfolioAllocationAcceptedNeedsApproval {
		return fmt.Errorf("portfolio allocation with approval-required items must remain accepted_needs_approval")
	}
	if !requiresApproval && allocation.Status != PortfolioAllocationAccepted {
		return fmt.Errorf("portfolio allocation without approval-required items must be accepted")
	}
	return nil
}

func verifyPortfolioPursuitOwnership(tx *gorm.DB, ownerIdentity string, items []models.PursuitPortfolioAllocationItem) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PursuitID)
	}
	var count int64
	if err := tx.Model(&models.Pursuit{}).
		Where("owner_identity = ? AND id IN ?", ownerIdentity, ids).
		Count(&count).Error; err != nil {
		return fmt.Errorf("verify portfolio pursuit ownership: %w", err)
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("one or more portfolio pursuits are unavailable to this owner")
	}
	return nil
}

func loadAndVerifyPortfolioAllocationReplay(
	tx *gorm.DB,
	wanted *models.PursuitPortfolioAllocation,
	wantedItems []models.PursuitPortfolioAllocationItem,
	wantedReservations []models.PursuitResourceReservation,
	wantedActivities []models.PursuitActivity,
	stored *models.PursuitPortfolioAllocation,
	storedItems *[]models.PursuitPortfolioAllocationItem,
) error {
	if err := tx.Where("owner_identity = ? AND plan_id = ?", wanted.OwnerIdentity, wanted.PlanID).
		First(stored).Error; err != nil {
		return err
	}
	if stored.RequestDigest != wanted.RequestDigest || stored.DecisionDigest != wanted.DecisionDigest || stored.RecordDigest != wanted.RecordDigest {
		return fmt.Errorf("portfolio allocation plan was already accepted with different digests")
	}
	if err := tx.Where("owner_identity = ? AND allocation_id = ?", wanted.OwnerIdentity, stored.ID).
		Order("scheduled_start ASC, pursuit_id ASC").Find(storedItems).Error; err != nil {
		return err
	}
	if len(*storedItems) != len(wantedItems) {
		return fmt.Errorf("stored portfolio allocation does not match the replayed item set")
	}
	wantedByPursuit := make(map[uuid.UUID]models.PursuitPortfolioAllocationItem, len(wantedItems))
	reservationByPursuit := make(map[uuid.UUID]models.PursuitResourceReservation, len(wantedReservations))
	for _, item := range wantedItems {
		wantedByPursuit[item.PursuitID] = item
	}
	for _, reservation := range wantedReservations {
		reservationByPursuit[reservation.PursuitID] = reservation
	}
	for _, persisted := range *storedItems {
		expected, ok := wantedByPursuit[persisted.PursuitID]
		if !ok || persisted.RecordDigest != expected.RecordDigest {
			return fmt.Errorf("portfolio allocation plan was already accepted with different item digests")
		}
		expectedReservation, ok := reservationByPursuit[persisted.PursuitID]
		if !ok {
			return fmt.Errorf("portfolio allocation replay is missing a bound reservation")
		}
		var storedReservation models.PursuitResourceReservation
		if err := tx.Where(
			"id = ? AND owner_identity = ? AND pursuit_id = ?",
			persisted.ReservationID, wanted.OwnerIdentity, persisted.PursuitID,
		).First(&storedReservation).Error; err != nil {
			return fmt.Errorf("load portfolio allocation reservation: %w", err)
		}
		if storedReservation.RecordDigest != expectedReservation.RecordDigest {
			return fmt.Errorf("portfolio allocation plan was already accepted with different reservation digests")
		}
	}

	var activityCount int64
	if err := tx.Table("pursuit_activities AS a").
		Joins("JOIN pursuits AS p ON p.id = a.pursuit_id").
		Where(
			"p.owner_identity = ? AND a.source_type = ? AND a.source_id = ? AND a.event_type = ?",
			wanted.OwnerIdentity, portfolioAllocationActivitySource, stored.ID.String(), portfolioAllocationActivityType,
		).Count(&activityCount).Error; err != nil {
		return fmt.Errorf("verify portfolio allocation activities: %w", err)
	}
	if activityCount != int64(len(wantedActivities)) {
		return fmt.Errorf("stored portfolio allocation audit activity is incomplete")
	}
	return nil
}

func sortPortfolioAllocationItems(items []models.PursuitPortfolioAllocationItem) {
	sort.Slice(items, func(left, right int) bool {
		if !items[left].ScheduledStart.Equal(items[right].ScheduledStart) {
			return items[left].ScheduledStart.Before(items[right].ScheduledStart)
		}
		return items[left].PursuitID.String() < items[right].PursuitID.String()
	})
}

func validPortfolioRecordDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
