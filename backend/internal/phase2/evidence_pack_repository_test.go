package phase2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type testEvidencePackRepository struct {
	mu        sync.Mutex
	packs     map[string]EvidencePack
	createErr error
	getErr    error
}

func newTestEvidencePackRepository() *testEvidencePackRepository {
	return &testEvidencePackRepository{packs: make(map[string]EvidencePack)}
}

func (repository *testEvidencePackRepository) Create(
	_ context.Context,
	pack EvidencePack,
) (EvidencePack, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.createErr != nil {
		return EvidencePack{}, repository.createErr
	}
	normalized, err := normalizeEvidencePack(pack)
	if err != nil {
		return EvidencePack{}, err
	}
	repository.packs[testEvidencePackKey(
		normalized.OwnerIdentity,
		normalized.WorkspaceID,
		normalized.ID,
	)] = normalized
	return normalized, nil
}

func (repository *testEvidencePackRepository) Get(
	_ context.Context,
	ownerIdentity string,
	workspaceID string,
	id uuid.UUID,
) (EvidencePack, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.getErr != nil {
		return EvidencePack{}, repository.getErr
	}
	pack, ok := repository.packs[testEvidencePackKey(ownerIdentity, workspaceID, id)]
	if !ok {
		return EvidencePack{}, ErrEvidencePackNotFound
	}
	return pack, nil
}

func testEvidencePackKey(ownerIdentity, workspaceID string, id uuid.UUID) string {
	return ownerIdentity + "\x00" + workspaceID + "\x00" + id.String()
}

var errTestEvidenceStorage = errors.New("test evidence storage failure")

func TestEvidencePackRowRoundTripPreservesScopeAndProvenance(t *testing.T) {
	sourceID := uuid.New()
	receivedAt := time.Date(2026, time.July, 30, 18, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	pack, err := normalizeEvidencePack(EvidencePack{
		OwnerIdentity: "alice",
		WorkspaceID:   "legal",
		OperationID:   uuid.New(),
		Title:         "Evidence",
		Markdown:      "# Evidence",
		Provenance: EvidenceProvenance{
			SourceType:         "email",
			SourceID:           &sourceID,
			SourceURI:          "gmail://message/42",
			SourceReceivedAt:   &receivedAt,
			SourceRevisionHash: "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			DedupeKey:          "gmail:42",
		},
		GeneratedAt: receivedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("normalize pack: %v", err)
	}

	got := evidencePackFromRow(evidencePackToRow(pack))
	if got.ID != pack.ID || got.OwnerIdentity != pack.OwnerIdentity ||
		got.WorkspaceID != pack.WorkspaceID || got.OperationID != pack.OperationID ||
		got.ContentDigest != pack.ContentDigest ||
		got.Provenance.SourceID == nil || *got.Provenance.SourceID != sourceID ||
		got.Provenance.SourceURI != pack.Provenance.SourceURI ||
		got.Provenance.SourceRevisionHash != pack.Provenance.SourceRevisionHash ||
		got.Provenance.DedupeKey != pack.Provenance.DedupeKey ||
		got.Provenance.SourceReceivedAt == nil ||
		!got.Provenance.SourceReceivedAt.Equal(receivedAt) {
		t.Fatalf("row round trip lost evidence fields: got %#v want %#v", got, pack)
	}
}

func TestNilGormEvidencePackRepositoryFailsClosed(t *testing.T) {
	repository := NewGormEvidencePackRepository(nil)
	if _, err := repository.Create(t.Context(), EvidencePack{}); !errors.Is(err, ErrEvidencePackRepositoryUnavailable) {
		t.Fatalf("create error = %v, want repository unavailable", err)
	}
	if _, err := repository.Get(t.Context(), "alice", "local", uuid.New()); !errors.Is(err, ErrEvidencePackRepositoryUnavailable) {
		t.Fatalf("get error = %v, want repository unavailable", err)
	}
}
