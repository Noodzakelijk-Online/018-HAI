package source

import (
	"automation-hub-backend/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOdooJSON2SyncUsesFixedReadOnlyProfilesAndPreservesProvenance(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.Header.Get("Authorization") != "bearer test-odoo-key" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-Odoo-Database") != "hai" {
			t.Fatalf("X-Odoo-Database = %q", request.Header.Get("X-Odoo-Database"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["order"] != "write_date asc,id asc" || payload["limit"] != float64(2) {
			t.Fatalf("unexpected Odoo request payload: %#v", payload)
		}
		calls = append(calls, request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/json/2/crm.lead/search_read":
			_, _ = w.Write([]byte(`[{"id":7,"name":"Renewal follow-up","stage_id":[2,"Qualified"],"expected_revenue":1250,"activity_date_deadline":"2026-07-22","write_date":"2026-07-20 08:30:00"}]`))
		case "/json/2/project.task/search_read":
			_, _ = w.Write([]byte(`[{"id":8,"name":"Prepare evidence","project_id":[4,"Vivare dispute"],"stage_id":[1,"In progress"],"date_deadline":"2026-07-21","priority":"3","write_date":"2026-07-20 09:30:00"}]`))
		default:
			t.Fatalf("unexpected endpoint %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("HAI_ODOO_ENABLED", "true")
	t.Setenv("HAI_ODOO_BASE_URL", server.URL)
	t.Setenv("HAI_ODOO_DATABASE", "hai")
	t.Setenv("HAI_ODOO_API_KEY", "test-odoo-key")
	t.Setenv("HAI_ODOO_ALLOWED_MODELS", "crm.lead,project.task")
	t.Setenv("HAI_ODOO_SYNC_LIMIT", "2")

	repo := newFakeSourceRepo()
	service := NewService(repo, &fakeSourceMemoryService{})
	source, err := service.CreateSource(CreateSourceRequest{
		OwnerIdentity:     "robert",
		ConnectorKey:      odooJSON2ConnectorKey,
		Name:              "Odoo read-only workspace",
		Enabled:           true,
		LocalOnly:         false,
		SyncFrequency:     "manual",
		DefaultProjectKey: "operations",
	})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	result, err := service.Sync(source.ID, ImportRequest{Mode: ModeManualImport})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.ItemsSeen != 2 || result.Job.ItemsAdded != 2 || result.Job.CursorAfter != odooJSON2CursorPrefix+"2026-07-20T09:30:00Z" {
		t.Fatalf("sync result = %#v", result.Job)
	}
	if strings.Join(calls, ",") != "/json/2/crm.lead/search_read,/json/2/project.task/search_read" {
		t.Fatalf("calls = %#v", calls)
	}
	rawItems := make([]*models.SourceRawItem, 0, len(repo.rawItems))
	for _, item := range repo.rawItems {
		rawItems = append(rawItems, item)
	}
	extractions := make([]*models.SourceExtraction, 0, len(repo.extractions))
	for _, extraction := range repo.extractions {
		extractions = append(extractions, extraction)
	}
	if len(rawItems) != 2 || !containsOdooSourceURI(rawItems, "model=crm.lead") || containsOdooSecret(rawItems, "test-odoo-key") {
		t.Fatalf("raw items = %#v", repo.rawItems)
	}
	if len(extractions) != 2 || !containsOdooExtractionURI(extractions, "model=crm.lead") {
		t.Fatalf("extractions = %#v", repo.extractions)
	}
}

func containsOdooSourceURI(items []*models.SourceRawItem, needle string) bool {
	for _, item := range items {
		if strings.Contains(item.SourceURI, needle) {
			return true
		}
	}
	return false
}

func containsOdooSecret(items []*models.SourceRawItem, secret string) bool {
	for _, item := range items {
		if strings.Contains(item.Content, secret) || strings.Contains(item.Metadata, secret) || strings.Contains(item.SourceURI, secret) {
			return true
		}
	}
	return false
}

func containsOdooExtractionURI(items []*models.SourceExtraction, needle string) bool {
	for _, item := range items {
		if strings.Contains(item.SourceURI, needle) {
			return true
		}
	}
	return false
}

func TestOdooJSON2ConfigurationFailsClosed(t *testing.T) {
	t.Setenv("HAI_ODOO_ENABLED", "true")
	t.Setenv("HAI_ODOO_BASE_URL", "http://odoo.example.test")
	t.Setenv("HAI_ODOO_API_KEY", "not-exposed")
	if _, err := odooJSON2ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("config error = %v, want HTTPS rejection", err)
	}
	t.Setenv("HAI_ODOO_BASE_URL", "https://odoo.example.test")
	t.Setenv("HAI_ODOO_ALLOWED_MODELS", "res.users")
	if _, err := odooJSON2ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("config error = %v, want model allowlist rejection", err)
	}
	t.Setenv("HAI_ODOO_ENABLED", "false")
	service := NewService(newFakeSourceRepo(), &fakeSourceMemoryService{})
	connectors, err := service.Connectors()
	if err != nil {
		t.Fatalf("Connectors: %v", err)
	}
	for _, connector := range connectors {
		if connector.ConnectorKey == odooJSON2ConnectorKey {
			if connector.AdapterStatus != AdapterConfigurationRequired || !strings.Contains(connector.StatusReason, "configuration is incomplete") {
				t.Fatalf("connector = %#v", connector)
			}
			return
		}
	}
	t.Fatal("Odoo JSON-2 connector missing")
}
