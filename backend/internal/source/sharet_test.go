package source

import (
	"automation-hub-backend/internal/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShareTSyncReadsEveryPageWithoutPersistingCredentialsOrParticipantEmails(t *testing.T) {
	const token = "sharet_pat_" + "test-read-token"
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("Authorization header was not the configured connector token")
		}
		requests = append(requests, request.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/connector/status":
			_, _ = w.Write([]byte(`{"success":true,"capabilities":{"shareRead":true,"shareWrite":false}}`))
		case "/api/connector/shares":
			if request.URL.Query().Get("limit") != "100" {
				t.Fatalf("limit = %q, want 100", request.URL.Query().Get("limit"))
			}
			page := request.URL.Query().Get("page")
			var payload any
			switch page {
			case "1":
				data := make([]any, 0, 100)
				for index := 1; index <= 100; index++ {
					emails := []string{}
					if index == 1 {
						emails = []string{"private@example.test"}
					}
					data = append(data, map[string]any{
						"shareId": fmt.Sprintf("share-%03d", index), "cardId": fmt.Sprintf("card-%03d", index), "cardName": fmt.Sprintf("Task %03d", index), "boardId": "board-1", "boardName": "Delivery",
						"permissions": map[string]any{"canView": true, "canComment": true}, "allowedEmails": emails,
						"accessCount": index, "isActive": true, "createdAt": "2026-08-01T10:00:00Z", "updatedAt": "2026-08-02T10:00:00Z", "hasPassword": index == 1,
					})
				}
				payload = map[string]any{
					"success":    true,
					"data":       data,
					"pagination": map[string]any{"total": 101, "page": 1, "limit": 100, "pages": 2},
				}
			case "2":
				payload = map[string]any{
					"success": true,
					"data": []any{map[string]any{
						"shareId": "share-101", "cardId": "card-101", "cardName": "Task 101", "boardId": "board-1", "boardName": "Delivery",
						"permissions": map[string]any{"canView": true, "canDownload": true}, "allowedEmails": []string{},
						"accessCount": 2, "isActive": false, "createdAt": "2026-08-03T10:00:00Z", "updatedAt": "2026-08-04T10:00:00Z", "hasGuestRelay": true,
					}},
					"pagination": map[string]any{"total": 101, "page": 2, "limit": 100, "pages": 2},
				}
			default:
				t.Fatalf("unexpected page %q", page)
			}
			_ = json.NewEncoder(w).Encode(payload)
		default:
			t.Fatalf("unexpected endpoint %s", request.URL.Path)
		}
	}))
	defer server.Close()
	setShareTTestEnvironment(t, server.URL, token, "1000")

	repo := newFakeSourceRepo()
	service := NewService(repo, &fakeSourceMemoryService{})
	source, err := service.CreateSource(CreateSourceRequest{
		OwnerIdentity:     "robert",
		ConnectorKey:      shareTConnectorKey,
		Name:              "ShareT link inventory",
		Enabled:           true,
		LocalOnly:         false,
		SyncFrequency:     "manual",
		DefaultProjectKey: "delivery",
	})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	result, err := service.Sync(source.ID, ImportRequest{Mode: ModeManualImport})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.ItemsSeen != 101 || result.Job.ItemsAdded != 101 || result.Job.CursorAfter != shareTCursorPrefix+"2026-08-04T10:00:00Z" {
		t.Fatalf("sync result = %#v", result.Job)
	}
	if strings.Join(requests, ",") != "/api/connector/status,/api/connector/shares?page=1&limit=100,/api/connector/shares?page=2&limit=100" {
		t.Fatalf("requests = %#v", requests)
	}
	if len(repo.rawItems) != 101 {
		t.Fatalf("raw item count = %d", len(repo.rawItems))
	}
	for _, item := range repo.rawItems {
		combined := item.Content + "\n" + item.Metadata + "\n" + item.SourceURI
		if strings.Contains(combined, token) || strings.Contains(combined, "private@example.test") {
			t.Fatalf("raw ShareT item persisted a credential or participant email: %#v", item)
		}
		if !strings.Contains(item.Metadata, "write_back=disabled") || !strings.Contains(item.SourceURI, "/shared/") {
			t.Fatalf("raw ShareT provenance = %#v", item)
		}
	}
}

func TestShareTConfigurationFailsClosed(t *testing.T) {
	t.Setenv("HAI_SHARET_ENABLED", "true")
	t.Setenv("HAI_SHARET_BASE_URL", "http://sharet.example.test")
	t.Setenv("HAI_SHARET_CONNECTOR_TOKEN", "sharet_pat_not-exposed")
	if _, err := shareTConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("config error = %v, want HTTPS rejection", err)
	}
	t.Setenv("HAI_SHARET_BASE_URL", "https://sharet.example.test/api/connector")
	if _, err := shareTConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("config error = %v, want path rejection", err)
	}
	t.Setenv("HAI_SHARET_ENABLED", "false")
	service := NewService(newFakeSourceRepo(), &fakeSourceMemoryService{})
	connectors, err := service.Connectors()
	if err != nil {
		t.Fatalf("Connectors: %v", err)
	}
	for _, connector := range connectors {
		if connector.ConnectorKey == shareTConnectorKey {
			if connector.AdapterStatus != AdapterConfigurationRequired || !strings.Contains(connector.StatusReason, "configuration is incomplete") {
				t.Fatalf("connector = %#v", connector)
			}
			return
		}
	}
	t.Fatal("ShareT connector missing")
}

func TestShareTSyncRejectsAnIncompleteInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/connector/status" {
			_, _ = w.Write([]byte(`{"success":true,"capabilities":{"shareRead":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[],"pagination":{"total":101,"page":1,"limit":100,"pages":2}}`))
	}))
	defer server.Close()
	setShareTTestEnvironment(t, server.URL, "sharet_pat_inventory-limit", "100")

	items, _, err := fetchShareTSource(t.Context(), &models.ConnectedSource{})
	if err == nil || !strings.Contains(err.Error(), "completeness limit 100") || len(items) != 0 {
		t.Fatalf("items = %#v, error = %v", items, err)
	}
}

func setShareTTestEnvironment(t *testing.T, baseURL, token, limit string) {
	t.Helper()
	t.Setenv("HAI_SHARET_ENABLED", "true")
	t.Setenv("HAI_SHARET_BASE_URL", baseURL)
	t.Setenv("HAI_SHARET_CONNECTOR_TOKEN", token)
	t.Setenv("HAI_SHARET_SYNC_LIMIT", limit)
}
