package evaluation

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrAlreadyExists = errors.New("evaluation: already exists")
	ErrNotFound      = errors.New("evaluation: not found")
)

// Repository is the production persistence contract. Its create-only methods
// preserve immutable version/run semantics; a database adapter should enforce
// unique constraints on (dataset_id, version) and run id.
type Repository interface {
	CreateDataset(context.Context, Dataset) error
	GetDataset(context.Context, string, uint32) (Dataset, error)
	ListDatasetVersions(context.Context, string) ([]Dataset, error)
	CreateRun(context.Context, RunRecord) error
	GetRun(context.Context, string) (RunRecord, error)
	ListRuns(context.Context, RunQuery) ([]RunRecord, error)
}

type RunQuery struct {
	DatasetID string
	SubjectID string
	Mode      RunMode
	Limit     int
}

type MemoryRepository struct {
	mu       sync.RWMutex
	datasets map[string]Dataset
	runs     map[string]RunRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		datasets: make(map[string]Dataset),
		runs:     make(map[string]RunRecord),
	}
}

func (repository *MemoryRepository) CreateDataset(ctx context.Context, dataset Dataset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateDataset(dataset); err != nil {
		return err
	}
	key := caseKey(dataset.ID, dataset.Version)
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.datasets[key]; exists {
		return fmt.Errorf("%w: dataset %s", ErrAlreadyExists, key)
	}
	repository.datasets[key] = cloneDataset(dataset)
	return nil
}

func (repository *MemoryRepository) GetDataset(ctx context.Context, id string, version uint32) (Dataset, error) {
	if err := ctx.Err(); err != nil {
		return Dataset{}, err
	}
	repository.mu.RLock()
	dataset, exists := repository.datasets[caseKey(id, version)]
	repository.mu.RUnlock()
	if !exists {
		return Dataset{}, fmt.Errorf("%w: dataset %s@%d", ErrNotFound, id, version)
	}
	return cloneDataset(dataset), nil
}

func (repository *MemoryRepository) ListDatasetVersions(ctx context.Context, id string) ([]Dataset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]Dataset, 0)
	for _, dataset := range repository.datasets {
		if dataset.ID == id {
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

func (repository *MemoryRepository) CreateRun(ctx context.Context, record RunRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateRunRecord(record); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.runs[record.ID]; exists {
		return fmt.Errorf("%w: run %s", ErrAlreadyExists, record.ID)
	}
	dataset, exists := repository.datasets[caseKey(record.Dataset.ID, record.Dataset.Version)]
	if !exists || dataset.Digest != record.Dataset.Digest {
		return fmt.Errorf("%w: referenced dataset version", ErrNotFound)
	}
	if err := validateRunAgainstDataset(record, dataset); err != nil {
		return err
	}
	if record.BaselineRunID != "" {
		baseline, exists := repository.runs[record.BaselineRunID]
		if !exists {
			return fmt.Errorf("%w: baseline run %s", ErrNotFound, record.BaselineRunID)
		}
		if baseline.Dataset != record.Dataset {
			return fmt.Errorf("%w: baseline run uses another dataset version", ErrInvalidRun)
		}
	}
	repository.runs[record.ID] = cloneRun(record)
	return nil
}

func (repository *MemoryRepository) GetRun(ctx context.Context, id string) (RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return RunRecord{}, err
	}
	repository.mu.RLock()
	record, exists := repository.runs[id]
	repository.mu.RUnlock()
	if !exists {
		return RunRecord{}, fmt.Errorf("%w: run %s", ErrNotFound, id)
	}
	return cloneRun(record), nil
}

func (repository *MemoryRepository) ListRuns(ctx context.Context, query RunQuery) ([]RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	repository.mu.RLock()
	result := make([]RunRecord, 0)
	for _, record := range repository.runs {
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

var _ Repository = (*MemoryRepository)(nil)
