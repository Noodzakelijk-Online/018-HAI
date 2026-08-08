package frameworkevidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ContractVersion  = 1
	StatusPassed     = "passed"
	maxIdentifierLen = 256
	maxPayloadBytes  = 1 << 20
)

var (
	ErrNotFound              = errors.New("framework evidence preflight not found")
	ErrConflict              = errors.New("framework evidence preflight replay conflicts with immutable record")
	ErrInvalidRecord         = errors.New("invalid framework evidence preflight record")
	ErrRepositoryUnavailable = errors.New("framework evidence preflight repository unavailable")
	lowerSHA256Pattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Record is the neutral, owner-scoped durable proof that a task's framework
// evidence preflight passed before execution. AssertionsJSON is retained byte
// for byte and its canonical semantics must reproduce PreflightDigest.
type Record struct {
	ContractVersion      int             `json:"contractVersion"`
	OwnerIdentity        string          `json:"ownerIdentity"`
	TaskPlanID           string          `json:"taskPlanId"`
	FrameworkSelectionID string          `json:"frameworkSelectionId"`
	PreflightDigest      string          `json:"preflightDigest"`
	Status               string          `json:"status"`
	AssertionsJSON       json.RawMessage `json:"assertionsJson"`
	EvaluatedAt          time.Time       `json:"evaluatedAt"`
	CreatedAt            time.Time       `json:"createdAt"`
}

// Repository exposes only append and exact owner-scoped resolution. Store is
// idempotent for a byte-identical semantic replay and rejects replay drift.
type Repository interface {
	Store(context.Context, Record) error
	Resolve(
		context.Context,
		string,
		string,
		string,
		string,
	) (Record, error)
}

type MemoryRepository struct {
	mu      sync.RWMutex
	records map[string]Record
}

var _ Repository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{records: make(map[string]Record)}
}

func (repository *MemoryRepository) Store(ctx context.Context, record Record) error {
	if repository == nil {
		return ErrRepositoryUnavailable
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	normalized, err := normalizeRecord(record)
	if err != nil {
		return err
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}

	key := recordKey(
		normalized.OwnerIdentity,
		normalized.TaskPlanID,
		normalized.FrameworkSelectionID,
		normalized.PreflightDigest,
	)
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if existing, exists := repository.records[key]; exists {
		if !sameSemanticRecord(existing, normalized) {
			return ErrConflict
		}
		return nil
	}
	repository.records[key] = cloneRecord(normalized)
	return nil
}

func (repository *MemoryRepository) Resolve(
	ctx context.Context,
	ownerIdentity string,
	taskPlanID string,
	frameworkSelectionID string,
	preflightDigest string,
) (Record, error) {
	if repository == nil {
		return Record{}, ErrRepositoryUnavailable
	}
	if err := contextError(ctx); err != nil {
		return Record{}, err
	}
	ownerIdentity, taskPlanID, frameworkSelectionID, preflightDigest, err := normalizeLookup(
		ownerIdentity,
		taskPlanID,
		frameworkSelectionID,
		preflightDigest,
	)
	if err != nil {
		return Record{}, err
	}

	repository.mu.RLock()
	record, exists := repository.records[recordKey(
		ownerIdentity,
		taskPlanID,
		frameworkSelectionID,
		preflightDigest,
	)]
	repository.mu.RUnlock()
	if !exists {
		return Record{}, ErrNotFound
	}
	normalized, err := normalizeRecord(record)
	if err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return cloneRecord(normalized), nil
}

func normalizeRecord(record Record) (Record, error) {
	if record.ContractVersion == 0 {
		record.ContractVersion = ContractVersion
	}
	if record.ContractVersion != ContractVersion {
		return Record{}, fmt.Errorf("%w: contract version must be %d", ErrInvalidRecord, ContractVersion)
	}
	var err error
	record.OwnerIdentity, err = normalizeIdentifier("owner identity", record.OwnerIdentity)
	if err != nil {
		return Record{}, err
	}
	record.TaskPlanID, err = normalizeIdentifier("task plan id", record.TaskPlanID)
	if err != nil {
		return Record{}, err
	}
	record.FrameworkSelectionID, err = normalizeIdentifier("framework selection id", record.FrameworkSelectionID)
	if err != nil {
		return Record{}, err
	}
	record.PreflightDigest = strings.TrimSpace(record.PreflightDigest)
	if !lowerSHA256Pattern.MatchString(record.PreflightDigest) {
		return Record{}, fmt.Errorf("%w: preflight digest must be a lowercase SHA256", ErrInvalidRecord)
	}
	record.Status = strings.TrimSpace(record.Status)
	if record.Status != StatusPassed {
		return Record{}, fmt.Errorf("%w: status must be %q", ErrInvalidRecord, StatusPassed)
	}
	if len(record.AssertionsJSON) == 0 || len(record.AssertionsJSON) > maxPayloadBytes || !json.Valid(record.AssertionsJSON) {
		return Record{}, fmt.Errorf("%w: assertions JSON must be valid and no larger than %d bytes", ErrInvalidRecord, maxPayloadBytes)
	}
	if record.EvaluatedAt.IsZero() {
		return Record{}, fmt.Errorf("%w: evaluated at is required", ErrInvalidRecord)
	}
	record.EvaluatedAt = record.EvaluatedAt.UTC().Truncate(time.Microsecond)
	computedDigest, err := PreflightDigest(
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.EvaluatedAt,
		record.AssertionsJSON,
	)
	if err != nil || computedDigest != record.PreflightDigest {
		return Record{}, fmt.Errorf(
			"%w: assertions do not reproduce the preflight digest (computed %s, stored %s)",
			ErrInvalidRecord,
			computedDigest,
			record.PreflightDigest,
		)
	}
	if !record.CreatedAt.IsZero() {
		record.CreatedAt = record.CreatedAt.UTC().Truncate(time.Microsecond)
	}
	record.AssertionsJSON = cloneBytes(record.AssertionsJSON)
	return record, nil
}

func normalizeLookup(
	ownerIdentity string,
	taskPlanID string,
	frameworkSelectionID string,
	preflightDigest string,
) (string, string, string, string, error) {
	ownerIdentity, err := normalizeIdentifier("owner identity", ownerIdentity)
	if err != nil {
		return "", "", "", "", err
	}
	taskPlanID, err = normalizeIdentifier("task plan id", taskPlanID)
	if err != nil {
		return "", "", "", "", err
	}
	frameworkSelectionID, err = normalizeIdentifier("framework selection id", frameworkSelectionID)
	if err != nil {
		return "", "", "", "", err
	}
	preflightDigest = strings.TrimSpace(preflightDigest)
	if !lowerSHA256Pattern.MatchString(preflightDigest) {
		return "", "", "", "", fmt.Errorf("%w: preflight digest must be a lowercase SHA256", ErrInvalidRecord)
	}
	return ownerIdentity, taskPlanID, frameworkSelectionID, preflightDigest, nil
}

func normalizeIdentifier(label string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > maxIdentifierLen || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("%w: %s must contain 1-%d safe characters", ErrInvalidRecord, label, maxIdentifierLen)
	}
	return value, nil
}

func sameSemanticRecord(left Record, right Record) bool {
	return left.ContractVersion == right.ContractVersion &&
		left.OwnerIdentity == right.OwnerIdentity &&
		left.TaskPlanID == right.TaskPlanID &&
		left.FrameworkSelectionID == right.FrameworkSelectionID &&
		left.PreflightDigest == right.PreflightDigest &&
		left.Status == right.Status &&
		left.EvaluatedAt.Equal(right.EvaluatedAt) &&
		bytes.Equal(left.AssertionsJSON, right.AssertionsJSON)
}

func cloneRecord(record Record) Record {
	record.AssertionsJSON = cloneBytes(record.AssertionsJSON)
	return record
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}

func recordKey(ownerIdentity, taskPlanID, frameworkSelectionID, preflightDigest string) string {
	return strings.Join([]string{ownerIdentity, taskPlanID, frameworkSelectionID, preflightDigest}, "\x00")
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidRecord)
	}
	return ctx.Err()
}
