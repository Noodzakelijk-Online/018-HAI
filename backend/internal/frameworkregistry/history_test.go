package frameworkregistry

import (
	"strings"
	"testing"
	"time"
)

func TestConstitutionHistoryIsBoundedStableAndOwnerScoped(t *testing.T) {
	repository := NewMemoryRepository()
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time {
		now = now.Add(time.Minute)
		return now
	}

	first, err := service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   1,
		ChangeSummary: "First owner-reviewed amendment.",
		Preferences:   []string{"Prefer bounded, source-grounded planning."},
	})
	if err != nil {
		t.Fatalf("CreateConstitutionDraft first: %v", err)
	}
	staleDraft, err := service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   1,
		ChangeSummary: "Alternative amendment retained as a draft.",
	})
	if err != nil {
		t.Fatalf("CreateConstitutionDraft stale: %v", err)
	}
	if _, err := service.ActivateConstitution("alice", first.ID, "alice", ActivateConstitutionRequest{
		Confirmation: "ACTIVATE CONSTITUTION",
		ApprovalNote: "Alice reviewed and approved the first amendment.",
	}); err != nil {
		t.Fatalf("ActivateConstitution first: %v", err)
	}

	beforeSuperseded, err := service.ConstitutionHistory("alice", 20)
	if err != nil {
		t.Fatalf("ConstitutionHistory before superseded: %v", err)
	}
	firstDigest := historyDigestByID(t, beforeSuperseded.History, first.ID)

	latest, err := service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   first.Version,
		ChangeSummary: "Second owner-reviewed amendment.",
	})
	if err != nil {
		t.Fatalf("CreateConstitutionDraft latest: %v", err)
	}
	if _, err := service.ActivateConstitution("alice", latest.ID, "alice", ActivateConstitutionRequest{
		Confirmation: "ACTIVATE CONSTITUTION",
		ApprovalNote: "Alice reviewed and approved the second amendment.",
	}); err != nil {
		t.Fatalf("ActivateConstitution latest: %v", err)
	}

	page, err := service.ConstitutionHistory("alice", 2)
	if err != nil {
		t.Fatalf("ConstitutionHistory: %v", err)
	}
	if page.Limit != 2 || !page.Truncated || len(page.History) != 2 {
		t.Fatalf("bounded page = %#v", page)
	}
	if page.History[0].ID != latest.ID ||
		page.History[0].Status != ConstitutionActive ||
		page.History[1].ID != staleDraft.ID ||
		page.History[1].Status != ConstitutionDraft {
		t.Fatalf("history order = %#v", page.History)
	}
	for _, entry := range page.History {
		if len(entry.Digest) != 64 || strings.Trim(entry.Digest, "0123456789abcdef") != "" {
			t.Fatalf("invalid history digest %q", entry.Digest)
		}
	}

	full, err := service.ConstitutionHistory("alice", 100)
	if err != nil {
		t.Fatalf("full ConstitutionHistory: %v", err)
	}
	if got := historyDigestByID(t, full.History, first.ID); got != firstDigest {
		t.Fatalf("version digest changed after active became superseded: before=%s after=%s", firstDigest, got)
	}
	firstEntry := historyEntryByID(t, full.History, first.ID)
	if firstEntry.Status != ConstitutionSuperseded ||
		firstEntry.ApprovedBy != "alice" ||
		firstEntry.ApprovedAt == nil ||
		firstEntry.BaseVersion != 1 {
		t.Fatalf("superseded provenance = %#v", firstEntry)
	}

	bob, err := service.ConstitutionHistory("bob", 100)
	if err != nil {
		t.Fatalf("Bob ConstitutionHistory: %v", err)
	}
	if len(bob.History) != 0 {
		t.Fatalf("Alice history leaked to Bob: %#v", bob.History)
	}
}

func TestConstitutionHistoryDefaultsInvalidLimitsAndRequiresOwner(t *testing.T) {
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	page, err := service.ConstitutionHistory("alice", 101)
	if err != nil {
		t.Fatalf("ConstitutionHistory: %v", err)
	}
	if page.Limit != defaultHistoryLimit {
		t.Fatalf("limit = %d, want %d", page.Limit, defaultHistoryLimit)
	}
	if _, err := service.ConstitutionHistory(" ", 20); err == nil {
		t.Fatal("blank owner was accepted")
	}
}

func historyDigestByID(t *testing.T, history []ConstitutionHistoryEntry, id string) string {
	t.Helper()
	return historyEntryByID(t, history, id).Digest
}

func historyEntryByID(
	t *testing.T,
	history []ConstitutionHistoryEntry,
	id string,
) ConstitutionHistoryEntry {
	t.Helper()
	for _, entry := range history {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("history entry %s not found in %#v", id, history)
	return ConstitutionHistoryEntry{}
}
