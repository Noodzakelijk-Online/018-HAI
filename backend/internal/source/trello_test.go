package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

// trelloTestServer returns an httptest server that serves a small read-only
// board and records every request method and whether credentials were sent.
func trelloTestServer(t *testing.T, cardsJSON string) (*httptest.Server, *[]string) {
	t.Helper()
	methods := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*methods = append(*methods, r.Method)
		if r.URL.Query().Get("key") == "" || r.URL.Query().Get("token") == "" {
			http.Error(w, "missing credentials", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/lists"):
			_, _ = w.Write([]byte(`[{"id":"list-1","name":"Doing"},{"id":"list-2","name":"Done"}]`))
		case strings.HasSuffix(r.URL.Path, "/cards"):
			_, _ = w.Write([]byte(cardsJSON))
		default: // board metadata
			_, _ = w.Write([]byte(`{"id":"board-1","name":"Client Delivery","shortUrl":"https://trello.com/b/abc123"}`))
		}
	}))
	return server, methods
}

func newTrelloSource(id uuid.UUID, target, cursor string) *models.ConnectedSource {
	return &models.ConnectedSource{
		ID:                id,
		ConnectorKey:      trelloConnectorKey,
		Name:              "Delivery board",
		Category:          "project_board",
		Enabled:           true,
		Status:            "active",
		SyncFrequency:     "manual",
		SyncTarget:        target,
		DefaultProjectKey: "018-HAI",
		Cursor:            cursor,
	}
}

func TestSyncTrelloImportsCardsWithProvenanceAndCursor(t *testing.T) {
	cards := `[
		{"id":"card-1","name":"Prepare client quote","desc":"Draft the quote","shortUrl":"https://trello.com/c/card-1","due":"2026-07-20T09:00:00Z","dateLastActivity":"2026-07-10T12:00:00Z","idList":"list-1","labels":[{"name":"priority","color":"red"}]},
		{"id":"card-2","name":"Archived idea","dateLastActivity":"2026-07-05T08:00:00Z","idList":"list-2","closed":true}
	]`
	server, methods := trelloTestServer(t, cards)
	defer server.Close()

	t.Setenv(trelloBaseURLEnv, server.URL)
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOW_LINK_LOCAL", "true")
	t.Setenv(trelloAPIKeyEnv, "test-key")
	t.Setenv(trelloReadTokenEnv, "test-read-token")

	sourceID := uuid.New()
	repo := newFakeSourceRepo(newTrelloSource(sourceID, "abc123XY", ""))
	result, err := NewService(repo, &fakeSourceMemoryService{}).Sync(sourceID, ImportRequest{Mode: ModeIncrementalSync})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Closed card must be skipped; only the open card is ingested.
	if result.Job.ItemsSeen != 1 {
		t.Fatalf("ItemsSeen = %d, want 1 (closed card skipped)", result.Job.ItemsSeen)
	}
	if result.Job.CursorAfter != "2026-07-10T12:00:00Z" {
		t.Fatalf("CursorAfter = %q, want 2026-07-10T12:00:00Z", result.Job.CursorAfter)
	}
	if len(result.Extractions) != 1 {
		t.Fatalf("extractions = %d, want 1", len(result.Extractions))
	}
	if !strings.Contains(result.Extractions[0].SourceURI, "trello.com/c/card-1") {
		t.Fatalf("extraction provenance = %q, want card shortUrl", result.Extractions[0].SourceURI)
	}
	if result.Extractions[0].ProjectKey != "018-HAI" {
		t.Fatalf("project key = %q, want default", result.Extractions[0].ProjectKey)
	}
	if !repo.hasAudit("source.synced") {
		t.Fatalf("expected trello sync audit record")
	}
	// Read-only guarantee: every request the adapter made must be a GET.
	for _, method := range *methods {
		if method != http.MethodGet {
			t.Fatalf("trello adapter issued a %s request; the connector must be read-only", method)
		}
	}
}

func TestSyncTrelloIncrementalSkipsUnchangedCards(t *testing.T) {
	cards := `[
		{"id":"card-old","name":"Stale card","shortUrl":"https://trello.com/c/old","dateLastActivity":"2026-07-01T00:00:00Z","idList":"list-1"},
		{"id":"card-new","name":"Fresh activity","shortUrl":"https://trello.com/c/new","dateLastActivity":"2026-07-12T00:00:00Z","idList":"list-1"}
	]`
	server, _ := trelloTestServer(t, cards)
	defer server.Close()

	t.Setenv(trelloBaseURLEnv, server.URL)
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOW_LINK_LOCAL", "true")
	t.Setenv(trelloAPIKeyEnv, "test-key")
	t.Setenv(trelloReadTokenEnv, "test-read-token")

	sourceID := uuid.New()
	// Cursor sits between the two cards' activity timestamps.
	repo := newFakeSourceRepo(newTrelloSource(sourceID, "abc123XY", "2026-07-05T00:00:00Z"))
	result, err := NewService(repo, &fakeSourceMemoryService{}).Sync(sourceID, ImportRequest{Mode: ModeIncrementalSync})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.ItemsSeen != 1 {
		t.Fatalf("ItemsSeen = %d, want 1 (only the card changed after the cursor)", result.Job.ItemsSeen)
	}
	if len(result.Extractions) != 1 || !strings.Contains(result.Extractions[0].SourceURI, "/c/new") {
		t.Fatalf("expected only the freshly-updated card, got %#v", result.Extractions)
	}
	if result.Job.CursorAfter != "2026-07-12T00:00:00Z" {
		t.Fatalf("CursorAfter = %q, want advance to newest activity", result.Job.CursorAfter)
	}
}

func TestSyncTrelloRequiresCredentials(t *testing.T) {
	server, _ := trelloTestServer(t, `[]`)
	defer server.Close()
	t.Setenv(trelloBaseURLEnv, server.URL)
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOW_LINK_LOCAL", "true")
	t.Setenv(trelloAPIKeyEnv, "")
	t.Setenv(trelloReadTokenEnv, "")

	sourceID := uuid.New()
	repo := newFakeSourceRepo(newTrelloSource(sourceID, "abc123XY", ""))
	_, err := NewService(repo, &fakeSourceMemoryService{}).Sync(sourceID, ImportRequest{Mode: ModeIncrementalSync})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %v, want a not-configured credential error", err)
	}
}

func TestSyncTrelloRejectsUnallowlistedHost(t *testing.T) {
	t.Setenv(trelloBaseURLEnv, "https://trello.example.net")
	t.Setenv("CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv(trelloAPIKeyEnv, "test-key")
	t.Setenv(trelloReadTokenEnv, "test-read-token")

	sourceID := uuid.New()
	repo := newFakeSourceRepo(newTrelloSource(sourceID, "abc123XY", ""))
	_, err := NewService(repo, &fakeSourceMemoryService{}).Sync(sourceID, ImportRequest{Mode: ModeIncrementalSync})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("error = %v, want allowlist rejection", err)
	}
}

func TestTrelloBoardID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"board123ABC", "board123ABC", false},
		{"https://trello.com/b/abc123XY/client-board", "abc123XY", false},
		{"https://user:pass@trello.com/b/abc123XY", "", true},
		{"", "", true},
		{"has spaces", "", true},
	}
	for _, tc := range cases {
		got, err := trelloBoardID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("trelloBoardID(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("trelloBoardID(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}
