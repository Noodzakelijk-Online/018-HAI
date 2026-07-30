package lifeops

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GormRepository stores owner-scoped whole-life context in canonical
// PostgreSQL. Observations and capacity snapshots are append-only at the
// database layer; links and goals remain deliberately editable.
type GormRepository struct {
	DB *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{DB: db}
}

func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (r *GormRepository) EntityDomainLinks(ownerIdentity, entityType, entityID string) ([]EntityDomainLink, error) {
	var rows []models.LifeEntityDomainLink
	err := r.DB.
		Where("owner_identity = ? AND entity_type = ? AND entity_id = ?", ownerIdentity, entityType, entityID).
		Order(`"primary" DESC, domain_id ASC, id ASC`).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]EntityDomainLink, 0, len(rows))
	for _, row := range rows {
		item, err := linkFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *GormRepository) ReplaceEntityDomainLinks(ownerIdentity, entityType, entityID string, links []EntityDomainLink) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("owner_identity = ? AND entity_type = ? AND entity_id = ?", ownerIdentity, entityType, entityID).
			Delete(&models.LifeEntityDomainLink{}).Error; err != nil {
			return err
		}
		for _, link := range links {
			if link.OwnerIdentity != ownerIdentity || link.EntityType != entityType || link.EntityID != entityID {
				return fmt.Errorf("entity-domain link scope does not match replacement scope")
			}
			row, err := linkToModel(link)
			if err != nil {
				return err
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) SaveNeedObservation(observation NeedObservation) error {
	row, err := needToModel(observation)
	if err != nil {
		return err
	}
	return r.DB.Create(&row).Error
}

func (r *GormRepository) NeedObservations(ownerIdentity string, domainID DomainID, limit int) ([]NeedObservation, error) {
	query := r.DB.Where("owner_identity = ?", ownerIdentity)
	if domainID != "" {
		query = query.Where("domain_id = ?", domainID)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []models.LifeNeedObservation
	if err := query.Order("observed_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]NeedObservation, 0, len(rows))
	for _, row := range rows {
		item, err := needFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *GormRepository) SaveCapacitySnapshot(snapshot CapacitySnapshot) error {
	row, err := capacityToModel(snapshot)
	if err != nil {
		return err
	}
	return r.DB.Create(&row).Error
}

func (r *GormRepository) CapacitySnapshots(ownerIdentity string, limit int) ([]CapacitySnapshot, error) {
	query := r.DB.Where("owner_identity = ?", ownerIdentity)
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []models.LifeCapacitySnapshot
	if err := query.Order("captured_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]CapacitySnapshot, 0, len(rows))
	for _, row := range rows {
		item, err := capacityFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *GormRepository) FindGoal(ownerIdentity string, id uuid.UUID) (*GoalNode, error) {
	var row models.LifeGoalNode
	if err := r.DB.Where("owner_identity = ? AND id = ?", ownerIdentity, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item, err := goalFromModel(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) ListGoals(ownerIdentity string) ([]GoalNode, error) {
	var rows []models.LifeGoalNode
	if err := r.DB.
		Where("owner_identity = ?", ownerIdentity).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]GoalNode, 0, len(rows))
	for _, row := range rows {
		item, err := goalFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *GormRepository) SaveGoal(goal GoalNode) error {
	row, err := goalToModel(goal)
	if err != nil {
		return err
	}
	return r.DB.Save(&row).Error
}

func (r *GormRepository) SavePriorityAssessment(assessment PriorityAssessment) error {
	row, err := priorityAssessmentToModel(assessment)
	if err != nil {
		return err
	}
	return r.DB.Create(&row).Error
}

func (r *GormRepository) PriorityAssessments(ownerIdentity, entityType, entityID string, limit int) ([]PriorityAssessment, error) {
	query := r.DB.Where("owner_identity = ?", ownerIdentity)
	if entityType != "" {
		query = query.Where("entity_type = ? AND entity_id = ?", entityType, entityID)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []models.LifePriorityAssessment
	if err := query.Order("assessed_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PriorityAssessment, 0, len(rows))
	for _, row := range rows {
		item, err := priorityAssessmentFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func linkToModel(item EntityDomainLink) (models.LifeEntityDomainLink, error) {
	evidence, err := marshalStringSlice(item.Evidence)
	if err != nil {
		return models.LifeEntityDomainLink{}, err
	}
	return models.LifeEntityDomainLink{
		ID: item.ID, OwnerIdentity: item.OwnerIdentity, EntityType: item.EntityType,
		EntityID: item.EntityID, DomainID: string(item.DomainID), Primary: item.Primary,
		Confidence: item.Confidence, SourceLabel: item.SourceLabel, SourceURI: item.SourceURI,
		EvidenceJSON: evidence, VerificationStatus: item.VerificationStatus,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}, nil
}

func linkFromModel(row models.LifeEntityDomainLink) (EntityDomainLink, error) {
	var evidence []string
	if err := unmarshalJSON(row.EvidenceJSON, &evidence); err != nil {
		return EntityDomainLink{}, fmt.Errorf("decode life domain link evidence: %w", err)
	}
	return EntityDomainLink{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, EntityType: row.EntityType,
		EntityID: row.EntityID, DomainID: DomainID(row.DomainID), Primary: row.Primary,
		Confidence: row.Confidence, SourceLabel: row.SourceLabel, SourceURI: row.SourceURI,
		Evidence: evidence, VerificationStatus: row.VerificationStatus,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func needToModel(item NeedObservation) (models.LifeNeedObservation, error) {
	evidence, err := marshalStringSlice(item.Evidence)
	if err != nil {
		return models.LifeNeedObservation{}, err
	}
	return models.LifeNeedObservation{
		ID: item.ID, OwnerIdentity: item.OwnerIdentity, DomainID: string(item.DomainID),
		NeedLevel: item.NeedLevel, State: item.State, CurrentLevel: item.CurrentLevel,
		TargetLevel: item.TargetLevel, Gap: item.Gap, Priority: item.Priority,
		Confidence: item.Confidence, EvidenceJSON: evidence, SourceLabel: item.SourceLabel,
		SourceURI: item.SourceURI, ObservedAt: item.ObservedAt, ExpiresAt: item.ExpiresAt,
		NeedsReview: item.NeedsReview, CreatedAt: item.CreatedAt,
	}, nil
}

func needFromModel(row models.LifeNeedObservation) (NeedObservation, error) {
	var evidence []string
	if err := unmarshalJSON(row.EvidenceJSON, &evidence); err != nil {
		return NeedObservation{}, fmt.Errorf("decode need evidence: %w", err)
	}
	return NeedObservation{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, DomainID: DomainID(row.DomainID),
		NeedLevel: row.NeedLevel, State: row.State, CurrentLevel: row.CurrentLevel,
		TargetLevel: row.TargetLevel, Gap: row.Gap, Priority: row.Priority,
		Confidence: row.Confidence, Evidence: evidence, SourceLabel: row.SourceLabel,
		SourceURI: row.SourceURI, ObservedAt: row.ObservedAt, ExpiresAt: row.ExpiresAt,
		NeedsReview: row.NeedsReview, CreatedAt: row.CreatedAt,
	}, nil
}

func capacityToModel(item CapacitySnapshot) (models.LifeCapacitySnapshot, error) {
	signals, err := marshalJSON(item.Signals)
	if err != nil {
		return models.LifeCapacitySnapshot{}, err
	}
	constraints, err := marshalStringSlice(item.Constraints)
	if err != nil {
		return models.LifeCapacitySnapshot{}, err
	}
	return models.LifeCapacitySnapshot{
		ID: item.ID, OwnerIdentity: item.OwnerIdentity, Status: item.Status,
		SignalsJSON: signals, TimeAvailableMinutes: item.TimeAvailableMinutes,
		ConcurrentWorkLimit: item.ConcurrentWorkLimit, CurrentLoad: item.CurrentLoad,
		PlanningStepLimit: item.PlanningStepLimit, ConstraintsJSON: constraints,
		SourceLabel: item.SourceLabel, SourceURI: item.SourceURI, CapturedAt: item.CapturedAt,
		Confidence: item.Confidence, Fresh: item.Fresh, NeedsReview: item.NeedsReview,
		CreatedAt: item.CreatedAt,
	}, nil
}

func capacityFromModel(row models.LifeCapacitySnapshot) (CapacitySnapshot, error) {
	var signals CapacitySignals
	if err := unmarshalJSON(row.SignalsJSON, &signals); err != nil {
		return CapacitySnapshot{}, fmt.Errorf("decode capacity signals: %w", err)
	}
	var constraints []string
	if err := unmarshalJSON(row.ConstraintsJSON, &constraints); err != nil {
		return CapacitySnapshot{}, fmt.Errorf("decode capacity constraints: %w", err)
	}
	return CapacitySnapshot{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, Status: row.Status, Signals: signals,
		TimeAvailableMinutes: row.TimeAvailableMinutes, ConcurrentWorkLimit: row.ConcurrentWorkLimit,
		CurrentLoad: row.CurrentLoad, PlanningStepLimit: row.PlanningStepLimit,
		Constraints: constraints, SourceLabel: row.SourceLabel, SourceURI: row.SourceURI,
		CapturedAt: row.CapturedAt, Confidence: row.Confidence, Fresh: row.Fresh,
		NeedsReview: row.NeedsReview, CreatedAt: row.CreatedAt,
	}, nil
}

func goalToModel(item GoalNode) (models.LifeGoalNode, error) {
	domains := make([]string, 0, len(item.DomainIDs))
	for _, domain := range item.DomainIDs {
		domains = append(domains, string(domain))
	}
	domainJSON, err := marshalStringSlice(domains)
	if err != nil {
		return models.LifeGoalNode{}, err
	}
	successJSON, err := marshalStringSlice(item.SuccessCriteria)
	if err != nil {
		return models.LifeGoalNode{}, err
	}
	stopJSON, err := marshalStringSlice(item.StopConditions)
	if err != nil {
		return models.LifeGoalNode{}, err
	}
	return models.LifeGoalNode{
		ID: item.ID, OwnerIdentity: item.OwnerIdentity, ParentID: item.ParentID,
		Level: string(item.Level), DomainIDsJSON: domainJSON, Title: item.Title,
		Description: item.Description, SuccessCriteriaJSON: successJSON,
		StopConditionsJSON: stopJSON, Status: item.Status, Confidence: item.Confidence,
		SourceLabel: item.SourceLabel, SourceURI: item.SourceURI, TargetAt: item.TargetAt,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}, nil
}

func goalFromModel(row models.LifeGoalNode) (GoalNode, error) {
	var domains []string
	if err := unmarshalJSON(row.DomainIDsJSON, &domains); err != nil {
		return GoalNode{}, fmt.Errorf("decode goal domains: %w", err)
	}
	domainIDs := make([]DomainID, 0, len(domains))
	for _, domain := range domains {
		domainIDs = append(domainIDs, DomainID(domain))
	}
	var success []string
	if err := unmarshalJSON(row.SuccessCriteriaJSON, &success); err != nil {
		return GoalNode{}, fmt.Errorf("decode goal success criteria: %w", err)
	}
	var stops []string
	if err := unmarshalJSON(row.StopConditionsJSON, &stops); err != nil {
		return GoalNode{}, fmt.Errorf("decode goal stop conditions: %w", err)
	}
	return GoalNode{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, ParentID: row.ParentID,
		Level: GoalLevel(row.Level), DomainIDs: domainIDs, Title: row.Title,
		Description: row.Description, SuccessCriteria: success, StopConditions: stops,
		Status: row.Status, Confidence: row.Confidence, SourceLabel: row.SourceLabel,
		SourceURI: row.SourceURI, TargetAt: row.TargetAt, CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func priorityAssessmentToModel(item PriorityAssessment) (models.LifePriorityAssessment, error) {
	factors, err := marshalJSON(item.Factors)
	if err != nil {
		return models.LifePriorityAssessment{}, err
	}
	contributions, err := marshalJSON(item.Contributions)
	if err != nil {
		return models.LifePriorityAssessment{}, err
	}
	if item.Contributions == nil {
		contributions = "[]"
	}
	reasons, err := marshalStringSlice(item.Reasons)
	if err != nil {
		return models.LifePriorityAssessment{}, err
	}
	return models.LifePriorityAssessment{
		ID: item.ID, OwnerIdentity: item.OwnerIdentity, EntityType: item.EntityType,
		EntityID: item.EntityID, Title: item.Title, Score: item.Score, Band: item.Band,
		FactorsJSON: factors, ContributionsJSON: contributions, ReasonsJSON: reasons,
		CapacityApplied: item.CapacityApplied, AlgorithmVersion: item.AlgorithmVersion,
		SourceLabel: item.SourceLabel, SourceURI: item.SourceURI, AssessedAt: item.AssessedAt,
	}, nil
}

func priorityAssessmentFromModel(row models.LifePriorityAssessment) (PriorityAssessment, error) {
	var factors PriorityFactors
	if err := unmarshalJSON(row.FactorsJSON, &factors); err != nil {
		return PriorityAssessment{}, fmt.Errorf("decode priority factors: %w", err)
	}
	var contributions []FactorContribution
	if err := unmarshalJSON(row.ContributionsJSON, &contributions); err != nil {
		return PriorityAssessment{}, fmt.Errorf("decode priority contributions: %w", err)
	}
	var reasons []string
	if err := unmarshalJSON(row.ReasonsJSON, &reasons); err != nil {
		return PriorityAssessment{}, fmt.Errorf("decode priority reasons: %w", err)
	}
	return PriorityAssessment{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, EntityType: row.EntityType,
		EntityID: row.EntityID, Title: row.Title, Score: row.Score, Band: row.Band,
		Factors: factors, Contributions: contributions, Reasons: reasons,
		CapacityApplied: row.CapacityApplied, AlgorithmVersion: row.AlgorithmVersion,
		SourceLabel: row.SourceLabel, SourceURI: row.SourceURI, AssessedAt: row.AssessedAt,
	}, nil
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalStringSlice(value []string) (string, error) {
	if value == nil {
		value = []string{}
	}
	return marshalJSON(value)
}

func unmarshalJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("stored JSON is empty")
	}
	return json.Unmarshal([]byte(raw), target)
}
