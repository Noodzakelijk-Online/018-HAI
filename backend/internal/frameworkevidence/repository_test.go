package frameworkevidence

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMemoryRepositoryContract(t *testing.T) {
	repository := NewMemoryRepository()
	record := validRecord()
	originalPayload := append([]byte(nil), record.AssertionsJSON...)

	if err := repository.Store(t.Context(), record); err != nil {
		t.Fatalf("store record: %v", err)
	}
	record.AssertionsJSON[2] = 'X'

	resolved, err := repository.Resolve(
		t.Context(),
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.PreflightDigest,
	)
	if err != nil {
		t.Fatalf("resolve record: %v", err)
	}
	if !bytes.Equal(resolved.AssertionsJSON, originalPayload) {
		t.Fatalf("stored payload changed through caller alias: %q", resolved.AssertionsJSON)
	}
	resolved.AssertionsJSON[2] = 'Y'
	reloaded, err := repository.Resolve(
		t.Context(),
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.PreflightDigest,
	)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	if !bytes.Equal(reloaded.AssertionsJSON, originalPayload) {
		t.Fatalf("resolved payload mutated repository state: %q", reloaded.AssertionsJSON)
	}

	replay := validRecord()
	if err := repository.Store(t.Context(), replay); err != nil {
		t.Fatalf("exact replay was not idempotent: %v", err)
	}
	conflicting := replay
	conflicting.AssertionsJSON = []byte(`{"assertions":[{"id":"different"}]}`)
	if err := repository.Store(t.Context(), conflicting); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("altered assertion replay error = %v, want ErrInvalidRecord", err)
	}

	if _, err := repository.Resolve(
		t.Context(),
		"another-owner",
		replay.TaskPlanID,
		replay.FrameworkSelectionID,
		replay.PreflightDigest,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner resolution error = %v, want ErrNotFound", err)
	}
}

func TestMemoryRepositoryResolveRejectsMutatedAssertionsWithoutDigestChange(t *testing.T) {
	repository := NewMemoryRepository()
	record := validRecord()

	if err := repository.Store(t.Context(), record); err != nil {
		t.Fatalf("store record: %v", err)
	}

	key := recordKey(
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.PreflightDigest,
	)
	stored := repository.records[key]
	stored.AssertionsJSON = []byte(`[{
		"requirementId":"requirement-1",
		"frameworkId":"framework-1",
		"phase":"pre_authorization",
		"validator":"deterministic_check",
		"status":"verified",
		"evidence":["mutated-proof"]
	}]`)
	repository.records[key] = stored

	_, err := repository.Resolve(
		t.Context(),
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.PreflightDigest,
	)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("resolve mutated record error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "assertions do not reproduce the preflight digest") {
		t.Fatalf("resolve mutated record error = %v, want canonical digest mismatch", err)
	}
}

func TestRepositoryRejectsInvalidRecordsAndLookups(t *testing.T) {
	repository := NewMemoryRepository()
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "owner", mutate: func(record *Record) { record.OwnerIdentity = " " }},
		{name: "task plan", mutate: func(record *Record) { record.TaskPlanID = "bad\nplan" }},
		{name: "selection", mutate: func(record *Record) { record.FrameworkSelectionID = "" }},
		{name: "uppercase digest", mutate: func(record *Record) { record.PreflightDigest = strings.Repeat("A", 64) }},
		{name: "short digest", mutate: func(record *Record) { record.PreflightDigest = "abcd" }},
		{name: "status", mutate: func(record *Record) { record.Status = "blocked" }},
		{name: "payload", mutate: func(record *Record) { record.AssertionsJSON = []byte(`{"broken"`) }},
		{name: "evaluated at", mutate: func(record *Record) { record.EvaluatedAt = time.Time{} }},
		{name: "contract", mutate: func(record *Record) { record.ContractVersion = 2 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := validRecord()
			test.mutate(&record)
			if err := repository.Store(t.Context(), record); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("store error = %v, want ErrInvalidRecord", err)
			}
		})
	}

	if _, err := repository.Resolve(t.Context(), "owner-a", "plan-1", "selection-1", strings.Repeat("A", 64)); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("invalid lookup error = %v, want ErrInvalidRecord", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repository.Store(cancelled, validRecord()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled store error = %v", err)
	}
}

func TestNilRepositoriesFailClosed(t *testing.T) {
	var memory *MemoryRepository
	if err := memory.Store(t.Context(), validRecord()); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil memory store error = %v", err)
	}
	gormRepository := NewGormRepository(nil)
	if err := gormRepository.Store(t.Context(), validRecord()); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil gorm store error = %v", err)
	}
	if _, err := gormRepository.Resolve(t.Context(), "owner-a", "plan-1", "selection-1", strings.Repeat("a", 64)); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil gorm resolve error = %v", err)
	}
}

func validRecord() Record {
	record := Record{
		ContractVersion:      ContractVersion,
		OwnerIdentity:        "owner-a",
		TaskPlanID:           "plan-1",
		FrameworkSelectionID: "selection-1",
		Status:               StatusPassed,
		AssertionsJSON:       []byte(`[{"requirementId":"requirement-1","frameworkId":"framework-1","phase":"pre_authorization","validator":"deterministic_check","status":"verified","evidence":["proof-1"]}]`),
		EvaluatedAt:          time.Date(2026, time.August, 4, 10, 11, 12, 123456000, time.UTC),
	}
	record.PreflightDigest, _ = PreflightDigest(
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.EvaluatedAt,
		record.AssertionsJSON,
	)
	return record
}
