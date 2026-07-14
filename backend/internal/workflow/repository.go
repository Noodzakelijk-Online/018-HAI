package workflow

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error)
	UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error)
	FindItem(id uuid.UUID) (*models.WorkflowItem, error)
	FindActiveItemBySourceIdentity(sourceType, sourceID string) (*models.WorkflowItem, error)
	FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error)
	FindActiveItemBySourceIdentityForOwner(ownerIdentity, sourceType, sourceID string) (*models.WorkflowItem, error)
	FindActiveItemBySourceURIForOwner(ownerIdentity, sourceURI string) (*models.WorkflowItem, error)
	FindItems(includeArchived bool) ([]models.WorkflowItem, error)
	FindApprovalItems() ([]models.WorkflowItem, error)
	FindRunnableItems(now time.Time, limit int) ([]models.WorkflowItem, error)
	FindRunnableItemsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowItem, error)
	ClaimRunnableItem(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error)
	ClaimRunnableItemForOwner(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error)
	RenewRunnableItemClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error)
	UpdateClaimedItem(item *models.WorkflowItem, claimID string) (*models.WorkflowItem, bool, error)
	FindExpiredWorkflowClaims(now time.Time, limit int) ([]models.WorkflowItem, error)
	FindExpiredWorkflowClaimsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowItem, error)
	RecoverExpiredWorkflowClaim(item models.WorkflowItem, now time.Time) (*models.WorkflowItem, bool, error)
	CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error)
	UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error)
	FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error)
	SaveIntakeRecord(record *models.WorkflowIntakeRecord) (*models.WorkflowIntakeRecord, error)
	FindIntakeRecords(workflowID uuid.UUID) ([]models.WorkflowIntakeRecord, error)
	CreateProjectMatch(match *models.WorkflowProjectMatch) (*models.WorkflowProjectMatch, error)
	FindProjectMatches(workflowID uuid.UUID) ([]models.WorkflowProjectMatch, error)
	FindLinkedPursuits(workflowID uuid.UUID) ([]WorkflowPursuitContext, error)
	CreateEvidenceClaim(claim *models.WorkflowEvidenceClaim) (*models.WorkflowEvidenceClaim, error)
	FindEvidenceClaims(workflowID uuid.UUID) ([]models.WorkflowEvidenceClaim, error)
	CreateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error)
	UpdateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error)
	FindOpenLoops(workflowID uuid.UUID) ([]models.WorkflowOpenLoop, error)
	FindDashboardOpenLoops(now time.Time) ([]models.WorkflowOpenLoop, error)
	FindDashboardOpenLoopsForOwner(ownerIdentity string, now time.Time) ([]models.WorkflowOpenLoop, error)
	ClaimDueOpenLoop(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error)
	ClaimDueOpenLoopForOwner(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error)
	RenewOpenLoopClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error)
	UpdateClaimedOpenLoop(loop *models.WorkflowOpenLoop, claimID string) (*models.WorkflowOpenLoop, bool, error)
	FindExpiredOpenLoopClaims(now time.Time, limit int) ([]models.WorkflowOpenLoop, error)
	FindExpiredOpenLoopClaimsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowOpenLoop, error)
	RecoverExpiredOpenLoopClaim(loop models.WorkflowOpenLoop, now time.Time) (*models.WorkflowOpenLoop, bool, error)
	CreateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error)
	UpdateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error)
	FindProposals(workflowID uuid.UUID) ([]models.WorkflowProposal, error)
	CreateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error)
	UpdateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error)
	FindQualityGates(workflowID uuid.UUID) ([]models.WorkflowQualityGate, error)
	SaveRule(rule *models.WorkflowRule) (*models.WorkflowRule, error)
	FindRules() ([]models.WorkflowRule, error)
	CreateTransition(transition *models.WorkflowTransition) (*models.WorkflowTransition, error)
	FindTransitions(workflowID uuid.UUID) ([]models.WorkflowTransition, error)
	CreateSourceLink(link *models.WorkflowSourceLink) (*models.WorkflowSourceLink, error)
	FindSourceLinks(workflowID uuid.UUID) ([]models.WorkflowSourceLink, error)
	CreateDecision(decision *models.WorkflowDecision) (*models.WorkflowDecision, error)
	FindDecisions(workflowID uuid.UUID) ([]models.WorkflowDecision, error)
	CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error)
	FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error)
}

type GormRepository struct {
	DB *gorm.DB
}

func NewGormRepository(db *gorm.DB) Repository {
	return &GormRepository{DB: db}
}

func DefaultRepository() Repository {
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewGormRepository(db)
}

func (r *GormRepository) CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if err := r.DB.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if err := r.DB.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) FindItem(id uuid.UUID) (*models.WorkflowItem, error) {
	var item models.WorkflowItem
	if err := r.DB.First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindActiveItemBySourceIdentity(sourceType, sourceID string) (*models.WorkflowItem, error) {
	var item models.WorkflowItem
	err := r.DB.Where(
		"source_type = ? AND source_id = ? AND archived = ?",
		sourceType,
		sourceID,
		false,
	).Order("updated_at desc").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error) {
	var item models.WorkflowItem
	err := r.DB.Where("source_uri = ? AND archived = ?", sourceURI, false).Order("updated_at desc").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindActiveItemBySourceIdentityForOwner(ownerIdentity, sourceType, sourceID string) (*models.WorkflowItem, error) {
	if ownerIdentity == "" {
		return r.FindActiveItemBySourceIdentity(sourceType, sourceID)
	}
	var item models.WorkflowItem
	err := r.DB.Where(
		"owner_identity = ? AND source_type = ? AND source_id = ? AND archived = ?",
		ownerIdentity,
		sourceType,
		sourceID,
		false,
	).Order("updated_at desc").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindActiveItemBySourceURIForOwner(ownerIdentity, sourceURI string) (*models.WorkflowItem, error) {
	if ownerIdentity == "" {
		return r.FindActiveItemBySourceURI(sourceURI)
	}
	var item models.WorkflowItem
	err := r.DB.Where(
		"owner_identity = ? AND source_uri = ? AND archived = ?",
		ownerIdentity,
		sourceURI,
		false,
	).Order("updated_at desc").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindItems(includeArchived bool) ([]models.WorkflowItem, error) {
	var items []models.WorkflowItem
	query := r.DB.Order("priority_score desc, updated_at desc")
	if !includeArchived {
		query = query.Where("archived = ?", false)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *GormRepository) FindApprovalItems() ([]models.WorkflowItem, error) {
	var items []models.WorkflowItem
	err := r.DB.
		Where("archived = ? AND current_state = ?", false, StateNeedsApproval).
		Order("priority_score desc, updated_at desc").
		Find(&items).Error
	return items, err
}

func (r *GormRepository) FindRunnableItems(now time.Time, limit int) ([]models.WorkflowItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var items []models.WorkflowItem
	err := r.DB.
		Where("archived = ? AND current_state = ? AND retry_count < CASE WHEN max_retries <= 0 THEN 2 ELSE max_retries END", false, StateReady).
		Where("requires_approval = ? OR approval_status = ?", false, "approved").
		Where("next_run_at IS NULL OR next_run_at <= ?", now).
		Order("priority_score desc, updated_at asc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *GormRepository) FindRunnableItemsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowItem, error) {
	if ownerIdentity == "" {
		return r.FindRunnableItems(now, limit)
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var items []models.WorkflowItem
	err := r.DB.
		Where("owner_identity = ?", ownerIdentity).
		Where("archived = ? AND current_state = ? AND retry_count < CASE WHEN max_retries <= 0 THEN 2 ELSE max_retries END", false, StateReady).
		Where("requires_approval = ? OR approval_status = ?", false, "approved").
		Where("next_run_at IS NULL OR next_run_at <= ?", now).
		Order("priority_score desc, updated_at asc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *GormRepository) ClaimRunnableItem(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error) {
	return r.claimRunnableItem("", id, claimID, now, leaseUntil)
}

func (r *GormRepository) ClaimRunnableItemForOwner(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error) {
	return r.claimRunnableItem(ownerIdentity, id, claimID, now, leaseUntil)
}

func (r *GormRepository) claimRunnableItem(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error) {
	query := r.DB.
		Model(&models.WorkflowItem{}).
		Where("id = ? AND archived = ? AND current_state = ? AND retry_count < CASE WHEN max_retries <= 0 THEN 2 ELSE max_retries END", id, false, StateReady).
		Where("requires_approval = ? OR approval_status = ?", false, "approved").
		Where("next_run_at IS NULL OR next_run_at <= ?", now)
	if ownerIdentity != "" {
		query = query.Where("owner_identity = ?", ownerIdentity)
	}
	result := query.Updates(map[string]interface{}{
		"current_state":      StateInProgress,
		"last_run_at":        now,
		"next_action":        "task engine is executing claimed workflow item",
		"last_worker_error":  "",
		"worker_claim_id":    claimID,
		"worker_lease_until": leaseUntil,
		"updated_at":         now,
	})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	item, err := r.FindItem(id)
	if err != nil {
		return nil, false, err
	}
	r.startAutonomyAttempt(item, now)
	return item, true, nil
}

func (r *GormRepository) RenewRunnableItemClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error) {
	result := r.DB.
		Model(&models.WorkflowItem{}).
		Where("id = ? AND current_state = ? AND worker_claim_id = ?", id, StateInProgress, claimID).
		Updates(map[string]interface{}{
			"worker_lease_until": leaseUntil,
			"updated_at":         time.Now().UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) UpdateClaimedItem(item *models.WorkflowItem, claimID string) (*models.WorkflowItem, bool, error) {
	now := time.Now().UTC()
	result := r.DB.
		Model(&models.WorkflowItem{}).
		Where("id = ? AND current_state = ? AND worker_claim_id = ?", item.ID, StateInProgress, claimID).
		Updates(map[string]interface{}{
			"current_state":       item.CurrentState,
			"approval_status":     item.ApprovalStatus,
			"blocked_reason":      item.BlockedReason,
			"next_action":         item.NextAction,
			"retry_count":         item.RetryCount,
			"max_retries":         item.MaxRetries,
			"next_run_at":         item.NextRunAt,
			"last_run_at":         item.LastRunAt,
			"completed_at":        item.CompletedAt,
			"verification_status": item.VerificationStatus,
			"recovery_status":     item.RecoveryStatus,
			"recovery_note":       item.RecoveryNote,
			"last_task_plan_id":   item.LastTaskPlanID,
			"last_worker_error":   item.LastWorkerError,
			"worker_claim_id":     "",
			"worker_lease_until":  nil,
			"updated_at":          now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	updated, err := r.FindItem(item.ID)
	if err != nil {
		return nil, false, err
	}
	r.finishAutonomyAttempt(updated, now)
	return updated, true, nil
}

func (r *GormRepository) FindExpiredWorkflowClaims(now time.Time, limit int) ([]models.WorkflowItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	legacyBefore := now.Add(-claimLeaseDuration())
	var items []models.WorkflowItem
	err := r.DB.
		Where("archived = ? AND current_state = ?", false, StateInProgress).
		Where("(worker_claim_id <> ? AND worker_lease_until IS NOT NULL AND worker_lease_until <= ?) OR ((worker_claim_id = ? OR worker_claim_id IS NULL) AND worker_lease_until IS NULL AND last_run_at IS NOT NULL AND last_run_at <= ?)", "", now, "", legacyBefore).
		Order("worker_lease_until asc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *GormRepository) FindExpiredWorkflowClaimsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowItem, error) {
	if ownerIdentity == "" {
		return r.FindExpiredWorkflowClaims(now, limit)
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	legacyBefore := now.Add(-claimLeaseDuration())
	var items []models.WorkflowItem
	err := r.DB.
		Where("owner_identity = ?", ownerIdentity).
		Where("archived = ? AND current_state = ?", false, StateInProgress).
		Where("(worker_claim_id <> ? AND worker_lease_until IS NOT NULL AND worker_lease_until <= ?) OR ((worker_claim_id = ? OR worker_claim_id IS NULL) AND worker_lease_until IS NULL AND last_run_at IS NOT NULL AND last_run_at <= ?)", "", now, "", legacyBefore).
		Order("worker_lease_until asc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *GormRepository) RecoverExpiredWorkflowClaim(item models.WorkflowItem, now time.Time) (*models.WorkflowItem, bool, error) {
	reason := "worker lease expired; execution outcome is unknown and requires human review"
	query := r.DB.Model(&models.WorkflowItem{}).Where("id = ? AND current_state = ?", item.ID, StateInProgress)
	if item.WorkerClaimID == "" {
		query = query.Where("(worker_claim_id = ? OR worker_claim_id IS NULL) AND worker_lease_until IS NULL AND last_run_at IS NOT NULL AND last_run_at <= ?", "", now.Add(-claimLeaseDuration()))
	} else {
		query = query.Where("worker_claim_id = ? AND worker_lease_until <= ?", item.WorkerClaimID, now)
	}
	result := query.
		Updates(map[string]interface{}{
			"current_state":      StateBlocked,
			"blocked_reason":     reason,
			"next_action":        "review external side effects before retrying interrupted workflow",
			"last_worker_error":  reason,
			"recovery_status":    RecoveryNeedsReview,
			"recovery_note":      "",
			"retry_count":        gorm.Expr("retry_count + 1"),
			"next_run_at":        nil,
			"worker_claim_id":    "",
			"worker_lease_until": nil,
			"updated_at":         now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	updated, err := r.FindItem(item.ID)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func (r *GormRepository) CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if err := r.DB.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if err := r.DB.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error) {
	var items []models.WorkflowChecklistItem
	err := r.DB.Where("workflow_id = ?", workflowID).Order("position asc, created_at asc").Find(&items).Error
	return items, err
}

func (r *GormRepository) SaveIntakeRecord(record *models.WorkflowIntakeRecord) (*models.WorkflowIntakeRecord, error) {
	if err := r.DB.Save(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *GormRepository) FindIntakeRecords(workflowID uuid.UUID) ([]models.WorkflowIntakeRecord, error) {
	var records []models.WorkflowIntakeRecord
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&records).Error
	return records, err
}

func (r *GormRepository) CreateProjectMatch(match *models.WorkflowProjectMatch) (*models.WorkflowProjectMatch, error) {
	if err := r.DB.Create(match).Error; err != nil {
		return nil, err
	}
	return match, nil
}

func (r *GormRepository) FindProjectMatches(workflowID uuid.UUID) ([]models.WorkflowProjectMatch, error) {
	var matches []models.WorkflowProjectMatch
	err := r.DB.Where("workflow_id = ?", workflowID).Order("confidence desc, created_at desc").Find(&matches).Error
	return matches, err
}

func (r *GormRepository) FindLinkedPursuits(workflowID uuid.UUID) ([]WorkflowPursuitContext, error) {
	var rows []struct {
		ID                    uuid.UUID
		OwnerIdentity         string
		Title                 string
		Status                string
		RiskLevel             string
		PriorityScore         int
		Confidence            float64
		AutonomyLevel         string
		NeedCategory          string
		WhyItMatters          string
		DesiredOutcome        string
		CurrentStateSummary   string
		NextRecommendedAction string
		CompletionDefinition  string
		CompletionState       string
		LinkID                uuid.UUID
		Relationship          string
		SourceURI             string
		SourceLabel           string
		LinkConfidence        float64
	}
	err := r.DB.Table("pursuit_links AS links").
		Select(`pursuits.id,
			pursuits.owner_identity,
			pursuits.title,
			pursuits.status,
			pursuits.risk_level,
			pursuits.priority_score,
			pursuits.confidence,
			pursuits.autonomy_level,
			pursuits.need_category,
			pursuits.why_it_matters,
			pursuits.desired_outcome,
			pursuits.current_state_summary,
			pursuits.next_recommended_action,
			pursuits.completion_definition,
			pursuits.completion_state,
			links.id AS link_id,
			links.relationship,
			links.source_uri,
			links.source_label,
			links.confidence AS link_confidence`).
		Joins("JOIN pursuits ON pursuits.id = links.pursuit_id").
		Where("links.link_type = ? AND links.link_id = ? AND pursuits.archived = ?", "workflow", workflowID.String(), false).
		Order("pursuits.priority_score DESC, links.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]WorkflowPursuitContext, 0, len(rows))
	for _, row := range rows {
		result = append(result, WorkflowPursuitContext{
			ID:                    row.ID,
			OwnerIdentity:         row.OwnerIdentity,
			Title:                 row.Title,
			Status:                row.Status,
			RiskLevel:             row.RiskLevel,
			PriorityScore:         row.PriorityScore,
			Confidence:            row.Confidence,
			AutonomyLevel:         row.AutonomyLevel,
			NeedCategory:          row.NeedCategory,
			WhyItMatters:          row.WhyItMatters,
			DesiredOutcome:        row.DesiredOutcome,
			CurrentStateSummary:   row.CurrentStateSummary,
			NextRecommendedAction: row.NextRecommendedAction,
			CompletionDefinition:  row.CompletionDefinition,
			CompletionState:       row.CompletionState,
			LinkID:                row.LinkID,
			Relationship:          row.Relationship,
			SourceURI:             row.SourceURI,
			SourceLabel:           row.SourceLabel,
			LinkConfidence:        row.LinkConfidence,
		})
	}
	return result, nil
}

func (r *GormRepository) CreateEvidenceClaim(claim *models.WorkflowEvidenceClaim) (*models.WorkflowEvidenceClaim, error) {
	if err := r.DB.Create(claim).Error; err != nil {
		return nil, err
	}
	return claim, nil
}

func (r *GormRepository) FindEvidenceClaims(workflowID uuid.UUID) ([]models.WorkflowEvidenceClaim, error) {
	var claims []models.WorkflowEvidenceClaim
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&claims).Error
	return claims, err
}

func (r *GormRepository) CreateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error) {
	if err := r.DB.Create(loop).Error; err != nil {
		return nil, err
	}
	return loop, nil
}

func (r *GormRepository) UpdateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error) {
	if err := r.DB.Save(loop).Error; err != nil {
		return nil, err
	}
	return loop, nil
}

func (r *GormRepository) FindOpenLoops(workflowID uuid.UUID) ([]models.WorkflowOpenLoop, error) {
	var loops []models.WorkflowOpenLoop
	err := r.DB.Where("workflow_id = ?", workflowID).Order("follow_up_at asc NULLS LAST, updated_at desc").Find(&loops).Error
	return loops, err
}

func (r *GormRepository) FindDashboardOpenLoops(now time.Time) ([]models.WorkflowOpenLoop, error) {
	var loops []models.WorkflowOpenLoop
	err := r.DB.
		Table("workflow_open_loops").
		Joins("JOIN workflow_items ON workflow_items.id = workflow_open_loops.workflow_id").
		Where("workflow_open_loops.status = ?", "open").
		Where("workflow_open_loops.follow_up_at IS NULL OR workflow_open_loops.follow_up_at <= ?", now).
		Where("workflow_items.archived = ? AND workflow_items.current_state NOT IN ?", false, []string{StateArchived, StateCompleted}).
		Order("workflow_open_loops.follow_up_at asc NULLS LAST, workflow_open_loops.updated_at desc").
		Limit(50).
		Find(&loops).Error
	return loops, err
}

func (r *GormRepository) FindDashboardOpenLoopsForOwner(ownerIdentity string, now time.Time) ([]models.WorkflowOpenLoop, error) {
	if ownerIdentity == "" {
		return r.FindDashboardOpenLoops(now)
	}
	var loops []models.WorkflowOpenLoop
	err := r.DB.
		Table("workflow_open_loops").
		Joins("JOIN workflow_items ON workflow_items.id = workflow_open_loops.workflow_id").
		Where("workflow_items.owner_identity = ?", ownerIdentity).
		Where("workflow_open_loops.status = ?", "open").
		Where("workflow_open_loops.follow_up_at IS NULL OR workflow_open_loops.follow_up_at <= ?", now).
		Where("workflow_items.archived = ? AND workflow_items.current_state NOT IN ?", false, []string{StateArchived, StateCompleted}).
		Order("workflow_open_loops.follow_up_at asc NULLS LAST, workflow_open_loops.updated_at desc").
		Limit(50).
		Find(&loops).Error
	return loops, err
}

func (r *GormRepository) ClaimDueOpenLoop(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error) {
	return r.claimDueOpenLoop("", id, claimID, now, leaseUntil)
}

func (r *GormRepository) ClaimDueOpenLoopForOwner(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error) {
	return r.claimDueOpenLoop(ownerIdentity, id, claimID, now, leaseUntil)
}

func (r *GormRepository) claimDueOpenLoop(ownerIdentity string, id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error) {
	existsQuery := "EXISTS (SELECT 1 FROM workflow_items WHERE workflow_items.id = workflow_open_loops.workflow_id AND workflow_items.archived = ? AND workflow_items.current_state NOT IN ?)"
	existsArgs := []interface{}{false, []string{StateArchived, StateCompleted}}
	if ownerIdentity != "" {
		existsQuery = "EXISTS (SELECT 1 FROM workflow_items WHERE workflow_items.id = workflow_open_loops.workflow_id AND workflow_items.owner_identity = ? AND workflow_items.archived = ? AND workflow_items.current_state NOT IN ?)"
		existsArgs = []interface{}{ownerIdentity, false, []string{StateArchived, StateCompleted}}
	}
	result := r.DB.
		Model(&models.WorkflowOpenLoop{}).
		Where("id = ? AND status = ?", id, "open").
		Where("follow_up_at IS NULL OR follow_up_at <= ?", now).
		Where(existsQuery, existsArgs...).
		Updates(map[string]interface{}{
			"status":      "processing",
			"claim_id":    claimID,
			"lease_until": leaseUntil,
			"updated_at":  now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	var loop models.WorkflowOpenLoop
	if err := r.DB.First(&loop, "id = ?", id).Error; err != nil {
		return nil, false, err
	}
	return &loop, true, nil
}

func (r *GormRepository) RenewOpenLoopClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error) {
	result := r.DB.
		Model(&models.WorkflowOpenLoop{}).
		Where("id = ? AND status = ? AND claim_id = ?", id, "processing", claimID).
		Updates(map[string]interface{}{
			"lease_until": leaseUntil,
			"updated_at":  time.Now().UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) UpdateClaimedOpenLoop(loop *models.WorkflowOpenLoop, claimID string) (*models.WorkflowOpenLoop, bool, error) {
	now := time.Now().UTC()
	result := r.DB.
		Model(&models.WorkflowOpenLoop{}).
		Where("id = ? AND status = ? AND claim_id = ?", loop.ID, "processing", claimID).
		Updates(map[string]interface{}{
			"status":      loop.Status,
			"claim_id":    "",
			"lease_until": nil,
			"updated_at":  now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	var updated models.WorkflowOpenLoop
	if err := r.DB.First(&updated, "id = ?", loop.ID).Error; err != nil {
		return nil, false, err
	}
	return &updated, true, nil
}

func (r *GormRepository) FindExpiredOpenLoopClaims(now time.Time, limit int) ([]models.WorkflowOpenLoop, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	legacyBefore := now.Add(-claimLeaseDuration())
	var loops []models.WorkflowOpenLoop
	err := r.DB.
		Where("status = ?", "processing").
		Where("(claim_id <> ? AND lease_until IS NOT NULL AND lease_until <= ?) OR ((claim_id = ? OR claim_id IS NULL) AND lease_until IS NULL AND updated_at <= ?)", "", now, "", legacyBefore).
		Order("lease_until asc").
		Limit(limit).
		Find(&loops).Error
	return loops, err
}

func (r *GormRepository) FindExpiredOpenLoopClaimsForOwner(ownerIdentity string, now time.Time, limit int) ([]models.WorkflowOpenLoop, error) {
	if ownerIdentity == "" {
		return r.FindExpiredOpenLoopClaims(now, limit)
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	legacyBefore := now.Add(-claimLeaseDuration())
	var loops []models.WorkflowOpenLoop
	err := r.DB.
		Table("workflow_open_loops").
		Joins("JOIN workflow_items ON workflow_items.id = workflow_open_loops.workflow_id").
		Where("workflow_items.owner_identity = ?", ownerIdentity).
		Where("workflow_open_loops.status = ?", "processing").
		Where("(workflow_open_loops.claim_id <> ? AND workflow_open_loops.lease_until IS NOT NULL AND workflow_open_loops.lease_until <= ?) OR ((workflow_open_loops.claim_id = ? OR workflow_open_loops.claim_id IS NULL) AND workflow_open_loops.lease_until IS NULL AND workflow_open_loops.updated_at <= ?)", "", now, "", legacyBefore).
		Order("workflow_open_loops.lease_until asc").
		Limit(limit).
		Find(&loops).Error
	return loops, err
}

func (r *GormRepository) RecoverExpiredOpenLoopClaim(loop models.WorkflowOpenLoop, now time.Time) (*models.WorkflowOpenLoop, bool, error) {
	query := r.DB.Model(&models.WorkflowOpenLoop{}).Where("id = ? AND status = ?", loop.ID, "processing")
	if loop.ClaimID == "" {
		query = query.Where("(claim_id = ? OR claim_id IS NULL) AND lease_until IS NULL AND updated_at <= ?", "", now.Add(-claimLeaseDuration()))
	} else {
		query = query.Where("claim_id = ? AND lease_until <= ?", loop.ClaimID, now)
	}
	result := query.
		Updates(map[string]interface{}{
			"status":      "open",
			"claim_id":    "",
			"lease_until": nil,
			"updated_at":  now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	var updated models.WorkflowOpenLoop
	if err := r.DB.First(&updated, "id = ?", loop.ID).Error; err != nil {
		return nil, false, err
	}
	return &updated, true, nil
}

func (r *GormRepository) CreateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error) {
	if err := r.DB.Create(proposal).Error; err != nil {
		return nil, err
	}
	return proposal, nil
}

func (r *GormRepository) UpdateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error) {
	if err := r.DB.Save(proposal).Error; err != nil {
		return nil, err
	}
	return proposal, nil
}

func (r *GormRepository) FindProposals(workflowID uuid.UUID) ([]models.WorkflowProposal, error) {
	var proposals []models.WorkflowProposal
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&proposals).Error
	return proposals, err
}

func (r *GormRepository) CreateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error) {
	if err := r.DB.Create(gate).Error; err != nil {
		return nil, err
	}
	return gate, nil
}

func (r *GormRepository) UpdateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error) {
	if err := r.DB.Save(gate).Error; err != nil {
		return nil, err
	}
	return gate, nil
}

func (r *GormRepository) FindQualityGates(workflowID uuid.UUID) ([]models.WorkflowQualityGate, error) {
	var gates []models.WorkflowQualityGate
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&gates).Error
	return gates, err
}

func (r *GormRepository) SaveRule(rule *models.WorkflowRule) (*models.WorkflowRule, error) {
	var existing models.WorkflowRule
	if rule.ID == uuid.Nil {
		err := r.DB.Where("rule_key = ?", rule.RuleKey).First(&existing).Error
		if err == nil {
			rule.ID = existing.ID
			if rule.CreatedAt.IsZero() {
				rule.CreatedAt = existing.CreatedAt
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else if rule.CreatedAt.IsZero() {
		if err := r.DB.First(&existing, "id = ?", rule.ID).Error; err == nil {
			rule.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := r.DB.Save(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *GormRepository) FindRules() ([]models.WorkflowRule, error) {
	var rules []models.WorkflowRule
	err := r.DB.Order("category asc, rule_key asc").Find(&rules).Error
	return rules, err
}

func (r *GormRepository) CreateTransition(transition *models.WorkflowTransition) (*models.WorkflowTransition, error) {
	if err := r.DB.Create(transition).Error; err != nil {
		return nil, err
	}
	return transition, nil
}

func (r *GormRepository) FindTransitions(workflowID uuid.UUID) ([]models.WorkflowTransition, error) {
	var transitions []models.WorkflowTransition
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&transitions).Error
	return transitions, err
}

func (r *GormRepository) CreateSourceLink(link *models.WorkflowSourceLink) (*models.WorkflowSourceLink, error) {
	if err := r.DB.Create(link).Error; err != nil {
		return nil, err
	}
	return link, nil
}

func (r *GormRepository) FindSourceLinks(workflowID uuid.UUID) ([]models.WorkflowSourceLink, error) {
	var links []models.WorkflowSourceLink
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&links).Error
	return links, err
}

func (r *GormRepository) CreateDecision(decision *models.WorkflowDecision) (*models.WorkflowDecision, error) {
	if err := r.DB.Create(decision).Error; err != nil {
		return nil, err
	}
	return decision, nil
}

func (r *GormRepository) FindDecisions(workflowID uuid.UUID) ([]models.WorkflowDecision, error) {
	var decisions []models.WorkflowDecision
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&decisions).Error
	return decisions, err
}

func (r *GormRepository) CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error) {
	if err := r.DB.Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *GormRepository) FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error) {
	var events []models.WorkflowEvent
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&events).Error
	return events, err
}
