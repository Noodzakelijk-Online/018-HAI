package evaluation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrAlreadyExists = errors.New("evaluation: already exists")
	ErrNotFound      = errors.New("evaluation: not found")
)

// Repository is the production persistence contract. Its create-only methods
// preserve immutable version/run semantics; a database adapter should enforce
// owner-bound unique constraints on dataset versions, runs, and receipts.
type Repository interface {
	CreateDataset(context.Context, string, Dataset) error
	GetDataset(context.Context, string, string, uint32) (Dataset, error)
	ListDatasetVersions(context.Context, string, string) ([]Dataset, error)
	CreateRun(context.Context, string, RunRecord) error
	GetRun(context.Context, string, string) (RunRecord, error)
	ListRuns(context.Context, string, RunQuery) ([]RunRecord, error)
	CreateComparisonReceipt(context.Context, string, BaselineComparisonReceipt) error
	GetComparisonReceipt(context.Context, string, string) (BaselineComparisonReceipt, error)
	ListComparisonReceipts(context.Context, string, ReceiptQuery) ([]BaselineComparisonReceipt, error)
	CreatePromotionDecisionReceipt(context.Context, string, PromotionDecisionReceipt) error
	GetPromotionDecisionReceipt(context.Context, string, string) (PromotionDecisionReceipt, error)
	ListPromotionDecisionReceipts(context.Context, string, ReceiptQuery) ([]PromotionDecisionReceipt, error)
}

type RunQuery struct {
	DatasetID string
	SubjectID string
	Mode      RunMode
	Limit     int
}

type ReceiptQuery struct {
	CandidateRunID string
	Limit          int
}

type MemoryRepository struct {
	mu                 sync.RWMutex
	datasets           map[string]Dataset
	runs               map[string]RunRecord
	comparisonReceipts map[string]BaselineComparisonReceipt
	promotionReceipts  map[string]PromotionDecisionReceipt
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		datasets:           make(map[string]Dataset),
		runs:               make(map[string]RunRecord),
		comparisonReceipts: make(map[string]BaselineComparisonReceipt),
		promotionReceipts:  make(map[string]PromotionDecisionReceipt),
	}
}

func (repository *MemoryRepository) CreateDataset(ctx context.Context, owner string, dataset Dataset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateDataset(dataset); err != nil {
		return err
	}
	key := ownerKey(owner, caseKey(dataset.ID, dataset.Version))
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.datasets[key]; exists {
		return fmt.Errorf("%w: dataset %s", ErrAlreadyExists, key)
	}
	repository.datasets[key] = cloneDataset(dataset)
	return nil
}

func (repository *MemoryRepository) GetDataset(
	ctx context.Context,
	owner string,
	id string,
	version uint32,
) (Dataset, error) {
	if err := ctx.Err(); err != nil {
		return Dataset{}, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return Dataset{}, err
	}
	repository.mu.RLock()
	dataset, exists := repository.datasets[ownerKey(owner, caseKey(id, version))]
	repository.mu.RUnlock()
	if !exists {
		return Dataset{}, fmt.Errorf("%w: dataset %s@%d", ErrNotFound, id, version)
	}
	return cloneDataset(dataset), nil
}

func (repository *MemoryRepository) ListDatasetVersions(
	ctx context.Context,
	owner string,
	id string,
) ([]Dataset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]Dataset, 0)
	prefix := owner + "\x00"
	for key, dataset := range repository.datasets {
		if strings.HasPrefix(key, prefix) && dataset.ID == id {
			result = append(result, cloneDataset(dataset))
		}
	}
	if len(result) == 0 {
		return []Dataset{}, nil
	}
	for left := 0; left < len(result); left++ {
		for right := left + 1; right < len(result); right++ {
			if result[right].Version < result[left].Version {
				result[left], result[right] = result[right], result[left]
			}
		}
	}
	return result, nil
}

func (repository *MemoryRepository) CreateRun(ctx context.Context, owner string, record RunRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateRunRecord(record); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	runKey := ownerKey(owner, record.ID)
	if _, exists := repository.runs[runKey]; exists {
		return fmt.Errorf("%w: run %s", ErrAlreadyExists, record.ID)
	}
	dataset, exists := repository.datasets[ownerKey(owner, caseKey(record.Dataset.ID, record.Dataset.Version))]
	if !exists || dataset.Digest != record.Dataset.Digest {
		return fmt.Errorf("%w: referenced dataset version", ErrNotFound)
	}
	if err := validateRunAgainstDataset(record, dataset); err != nil {
		return err
	}
	if record.BaselineRunID != "" {
		baseline, exists := repository.runs[ownerKey(owner, record.BaselineRunID)]
		if !exists {
			return fmt.Errorf("%w: baseline run %s", ErrNotFound, record.BaselineRunID)
		}
		if baseline.Dataset != record.Dataset {
			return fmt.Errorf("%w: baseline run uses another dataset version", ErrInvalidRun)
		}
	}
	repository.runs[runKey] = cloneRun(record)
	return nil
}

func (repository *MemoryRepository) GetRun(
	ctx context.Context,
	owner string,
	id string,
) (RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return RunRecord{}, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return RunRecord{}, err
	}
	repository.mu.RLock()
	record, exists := repository.runs[ownerKey(owner, id)]
	repository.mu.RUnlock()
	if !exists {
		return RunRecord{}, fmt.Errorf("%w: run %s", ErrNotFound, id)
	}
	return cloneRun(record), nil
}

func (repository *MemoryRepository) ListRuns(
	ctx context.Context,
	owner string,
	query RunQuery,
) ([]RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	repository.mu.RLock()
	result := make([]RunRecord, 0)
	prefix := owner + "\x00"
	for key, record := range repository.runs {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if query.DatasetID != "" && record.Dataset.ID != query.DatasetID {
			continue
		}
		if query.SubjectID != "" && record.Subject.ID != query.SubjectID {
			continue
		}
		if query.Mode != "" && record.Mode != query.Mode {
			continue
		}
		result = append(result, cloneRun(record))
	}
	repository.mu.RUnlock()
	sortRuns(result)
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

func (repository *MemoryRepository) CreateComparisonReceipt(
	ctx context.Context,
	owner string,
	receipt BaselineComparisonReceipt,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidateBaselineComparisonReceipt(receipt); err != nil {
		return err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	key := ownerKey(owner, receipt.ID)
	if _, exists := repository.comparisonReceipts[key]; exists {
		return fmt.Errorf("%w: comparison receipt %s", ErrAlreadyExists, receipt.ID)
	}
	candidate, candidateExists := repository.runs[ownerKey(owner, receipt.CandidateRunID)]
	baseline, baselineExists := repository.runs[ownerKey(owner, receipt.BaselineRunID)]
	if !candidateExists || !baselineExists {
		return fmt.Errorf("%w: comparison receipt run", ErrNotFound)
	}
	if err := validateComparisonReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
		return err
	}
	repository.comparisonReceipts[key] = cloneBaselineComparisonReceipt(receipt)
	return nil
}

func (repository *MemoryRepository) GetComparisonReceipt(
	ctx context.Context,
	owner string,
	id string,
) (BaselineComparisonReceipt, error) {
	if err := ctx.Err(); err != nil {
		return BaselineComparisonReceipt{}, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return BaselineComparisonReceipt{}, err
	}
	repository.mu.RLock()
	receipt, exists := repository.comparisonReceipts[ownerKey(owner, id)]
	repository.mu.RUnlock()
	if !exists {
		return BaselineComparisonReceipt{}, fmt.Errorf("%w: comparison receipt %s", ErrNotFound, id)
	}
	return cloneBaselineComparisonReceipt(receipt), nil
}

func (repository *MemoryRepository) ListComparisonReceipts(
	ctx context.Context,
	owner string,
	query ReceiptQuery,
) ([]BaselineComparisonReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	query.Limit = boundedReceiptLimit(query.Limit)
	repository.mu.RLock()
	result := make([]BaselineComparisonReceipt, 0)
	prefix := owner + "\x00"
	for key, receipt := range repository.comparisonReceipts {
		if !strings.HasPrefix(key, prefix) ||
			(query.CandidateRunID != "" && receipt.CandidateRunID != query.CandidateRunID) {
			continue
		}
		result = append(result, cloneBaselineComparisonReceipt(receipt))
	}
	repository.mu.RUnlock()
	sort.Slice(result, func(left, right int) bool {
		if result[left].CreatedAt.Equal(result[right].CreatedAt) {
			return result[left].ID < result[right].ID
		}
		return result[left].CreatedAt.After(result[right].CreatedAt)
	})
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

func (repository *MemoryRepository) CreatePromotionDecisionReceipt(
	ctx context.Context,
	owner string,
	receipt PromotionDecisionReceipt,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return err
	}
	if err := ValidatePromotionDecisionReceipt(receipt); err != nil {
		return err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	key := ownerKey(owner, receipt.ID)
	if _, exists := repository.promotionReceipts[key]; exists {
		return fmt.Errorf("%w: promotion receipt %s", ErrAlreadyExists, receipt.ID)
	}
	candidate, exists := repository.runs[ownerKey(owner, receipt.CandidateRunID)]
	if !exists {
		return fmt.Errorf("%w: promotion receipt candidate run", ErrNotFound)
	}
	var baseline *RunRecord
	if receipt.BaselineRunID != "" {
		stored, exists := repository.runs[ownerKey(owner, receipt.BaselineRunID)]
		if !exists {
			return fmt.Errorf("%w: promotion receipt baseline run", ErrNotFound)
		}
		copy := cloneRun(stored)
		baseline = &copy
	}
	if err := validatePromotionReceiptAgainstRuns(receipt, candidate, baseline); err != nil {
		return err
	}
	if err := repository.validateComparisonReferenceLocked(owner, receipt); err != nil {
		return err
	}
	repository.promotionReceipts[key] = clonePromotionDecisionReceipt(receipt)
	return nil
}

func (repository *MemoryRepository) GetPromotionDecisionReceipt(
	ctx context.Context,
	owner string,
	id string,
) (PromotionDecisionReceipt, error) {
	if err := ctx.Err(); err != nil {
		return PromotionDecisionReceipt{}, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return PromotionDecisionReceipt{}, err
	}
	repository.mu.RLock()
	receipt, exists := repository.promotionReceipts[ownerKey(owner, id)]
	repository.mu.RUnlock()
	if !exists {
		return PromotionDecisionReceipt{}, fmt.Errorf("%w: promotion receipt %s", ErrNotFound, id)
	}
	return clonePromotionDecisionReceipt(receipt), nil
}

func (repository *MemoryRepository) ListPromotionDecisionReceipts(
	ctx context.Context,
	owner string,
	query ReceiptQuery,
) ([]PromotionDecisionReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := normalizeOwner(owner)
	if err != nil {
		return nil, err
	}
	query.Limit = boundedReceiptLimit(query.Limit)
	repository.mu.RLock()
	result := make([]PromotionDecisionReceipt, 0)
	prefix := owner + "\x00"
	for key, receipt := range repository.promotionReceipts {
		if !strings.HasPrefix(key, prefix) ||
			(query.CandidateRunID != "" && receipt.CandidateRunID != query.CandidateRunID) {
			continue
		}
		result = append(result, clonePromotionDecisionReceipt(receipt))
	}
	repository.mu.RUnlock()
	sort.Slice(result, func(left, right int) bool {
		if result[left].CreatedAt.Equal(result[right].CreatedAt) {
			return result[left].ID < result[right].ID
		}
		return result[left].CreatedAt.After(result[right].CreatedAt)
	})
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

func (repository *MemoryRepository) validateComparisonReferenceLocked(
	owner string,
	receipt PromotionDecisionReceipt,
) error {
	if receipt.ComparisonReceiptID == "" {
		return nil
	}
	comparison, exists := repository.comparisonReceipts[ownerKey(owner, receipt.ComparisonReceiptID)]
	if !exists {
		return fmt.Errorf("%w: comparison receipt %s", ErrNotFound, receipt.ComparisonReceiptID)
	}
	if comparison.CandidateRunID != receipt.CandidateRunID ||
		comparison.BaselineRunID != receipt.BaselineRunID ||
		!reflectThresholdsEqual(comparison.Thresholds, receipt.Thresholds) ||
		receipt.Decision.Comparison == nil ||
		!reflectComparisonEqual(comparison.Comparison, *receipt.Decision.Comparison) {
		return fmt.Errorf("evaluation: promotion receipt comparison binding mismatch")
	}
	return nil
}

func normalizeOwner(owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || len(owner) > 255 {
		return "", fmt.Errorf("evaluation: owner identity is required and must not exceed 255 characters")
	}
	return owner, nil
}

func ownerKey(owner string, value string) string {
	return owner + "\x00" + strings.TrimSpace(value)
}

func boundedReceiptLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func reflectThresholdsEqual(left, right RegressionThresholds) bool {
	return left == right
}

func reflectComparisonEqual(left, right BaselineComparison) bool {
	if left.CandidateRunID != right.CandidateRunID ||
		left.BaselineRunID != right.BaselineRunID ||
		!sameFloat(left.OverallScoreDelta, right.OverallScoreDelta) ||
		!sameFloat(left.CasePassRateDelta, right.CasePassRateDelta) ||
		left.Regressed != right.Regressed ||
		len(left.Violations) != len(right.Violations) {
		return false
	}
	for index := range left.Violations {
		if left.Violations[index] != right.Violations[index] {
			return false
		}
	}
	return true
}

var _ Repository = (*MemoryRepository)(nil)
