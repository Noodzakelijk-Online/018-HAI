package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const airbyteWorkspaceID = "11111111-1111-4111-8111-111111111111"

func TestAirbyteInventorySyncReadsOnlyApprovedMetadata(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer test-airbyte-key" || request.Header.Get("User-Agent") != "HAI-Airbyte-Inventory/1.0" {
			t.Fatalf("unexpected Airbyte request: %s %#v", request.Method, request.Header)
		}
		if request.URL.Query().Get("workspaceIds") != airbyteWorkspaceID || request.URL.Query().Get("limit") != "100" || request.URL.Query().Get("offset") != "0" || request.URL.Query().Get("includeDeleted") != "false" {
			t.Fatalf("unexpected Airbyte query: %s", request.URL.RawQuery)
		}
		calls = append(calls, request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/sources":
			_, _ = w.Write([]byte(`{"data":[{"sourceId":"22222222-2222-4222-8222-222222222222","name":"Robert Gmail","sourceType":"source-gmail","workspaceId":"11111111-1111-4111-8111-111111111111","configuration":{"client_secret":"must-not-be-stored"}},{"sourceId":"33333333-3333-4333-8333-333333333333","name":"Other workspace","sourceType":"source-notion","workspaceId":"44444444-4444-4444-8444-444444444444"}]}`))
		case "/v1/connections":
			_, _ = w.Write([]byte(`{"data":[{"connectionId":"55555555-5555-4555-8555-555555555555","name":"Gmail to local index","sourceId":"22222222-2222-4222-8222-222222222222","destinationId":"66666666-6666-4666-8666-666666666666","workspaceId":"11111111-1111-4111-8111-111111111111","status":"active","schedule":{"scheduleType":"cron"},"configurations":{"streams":[{"name":"private_mail"}]}},{"connectionId":"77777777-7777-4777-8777-777777777777","name":"Other connection","sourceId":"33333333-3333-4333-8333-333333333333","destinationId":"88888888-8888-4888-8888-888888888888","workspaceId":"44444444-4444-4444-8444-444444444444","status":"active","schedule":{"scheduleType":"manual"}}]}`))
		default:
			t.Fatalf("unexpected Airbyte path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("HAI_AIRBYTE_ENABLED", "true")
	t.Setenv("HAI_AIRBYTE_BASE_URL", server.URL+"/v1")
	t.Setenv("HAI_AIRBYTE_API_KEY", "test-airbyte-key")
	t.Setenv("HAI_AIRBYTE_WORKSPACE_IDS", airbyteWorkspaceID)

	repo := newFakeSourceRepo()
	service := NewService(repo, &fakeSourceMemoryService{})
	source, err := service.CreateSource(CreateSourceRequest{OwnerIdentity: "robert", ConnectorKey: airbyteInventoryConnectorKey, Name: "Airbyte account inventory", Enabled: true, LocalOnly: true, SyncFrequency: "manual", DefaultProjectKey: "operations"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	result, err := service.Sync(source.ID, ImportRequest{Mode: ModeManualImport})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.ItemsSeen != 2 || result.Job.ItemsAdded != 2 || result.Job.CursorAfter != airbyteInventoryCursor {
		t.Fatalf("sync result = %#v", result.Job)
	}
	if strings.Join(calls, ",") != "/v1/sources,/v1/connections" {
		t.Fatalf("calls = %#v", calls)
	}
	for _, item := range repo.rawItems {
		if strings.Contains(item.Content, "must-not-be-stored") || strings.Contains(item.Content, "private_mail") || strings.Contains(item.Metadata, "test-airbyte-key") || strings.Contains(item.SourceURI, "test-airbyte-key") {
			t.Fatalf("Airbyte secret/configuration leaked into source record: %#v", item)
		}
	}
}

func TestAirbyteInventoryConfigurationFailsClosed(t *testing.T) {
	t.Setenv("HAI_AIRBYTE_ENABLED", "true")
	t.Setenv("HAI_AIRBYTE_BASE_URL", "http://airbyte.example.test/v1")
	t.Setenv("HAI_AIRBYTE_API_KEY", "not-exposed")
	t.Setenv("HAI_AIRBYTE_WORKSPACE_IDS", airbyteWorkspaceID)
	if _, err := airbyteInventoryConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "local Airbyte host") {
		t.Fatalf("config error = %v, want local-host rejection", err)
	}
	t.Setenv("HAI_AIRBYTE_BASE_URL", "https://airbyte.example.test/v1")
	if _, err := airbyteInventoryConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "local Airbyte host") {
		t.Fatalf("config error = %v, want HTTPS remote-host rejection", err)
	}
	t.Setenv("HAI_AIRBYTE_BASE_URL", "http://localhost:8000/v1")
	t.Setenv("HAI_AIRBYTE_WORKSPACE_IDS", "not-a-uuid")
	if _, err := airbyteInventoryConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "invalid UUID") {
		t.Fatalf("config error = %v, want UUID rejection", err)
	}
	t.Setenv("HAI_AIRBYTE_ENABLED", "false")
	service := NewService(newFakeSourceRepo(), &fakeSourceMemoryService{})
	connectors, err := service.Connectors()
	if err != nil {
		t.Fatalf("Connectors: %v", err)
	}
	for _, connector := range connectors {
		if connector.ConnectorKey == airbyteInventoryConnectorKey {
			if connector.AdapterStatus != AdapterConfigurationRequired || !strings.Contains(connector.StatusReason, "configuration is incomplete") {
				t.Fatalf("connector = %#v", connector)
			}
			return
		}
	}
	t.Fatal("Airbyte inventory connector missing")
}
