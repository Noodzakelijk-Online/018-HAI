package googleoauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDriveInitialInventoryAndChangesArePaged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/changes/startPageToken":
			_, _ = w.Write([]byte(`{"startPageToken":"change-10"}`))
		case "/files":
			if r.URL.Query().Get("q") != "trashed = false" || r.URL.Query().Get("pageToken") != "page-1" {
				t.Errorf("files query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"nextPageToken":"page-2","files":[{"id":"f1","name":"Plan.md","mimeType":"text/markdown","modifiedTime":"2026-08-03T10:00:00Z","size":"42","webViewLink":"https://drive.google.com/file/d/f1/view"}]}`))
		case "/changes":
			if r.URL.Query().Get("pageToken") != "change-10" {
				t.Errorf("changes page token = %q", r.URL.Query().Get("pageToken"))
			}
			_, _ = w.Write([]byte(`{"newStartPageToken":"change-12","changes":[{"fileId":"f2","time":"2026-08-03T11:00:00Z","file":{"id":"f2","name":"Notes.txt","mimeType":"text/plain","size":"8"}},{"fileId":"gone","removed":true}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := DriveClient{AccessToken: "token", BaseURL: server.URL}
	token, err := client.GetStartPageToken(context.Background())
	if err != nil || token != "change-10" {
		t.Fatalf("GetStartPageToken = %q, %v", token, err)
	}
	files, err := client.ListFilesPage(context.Background(), "page-1", 100)
	if err != nil || len(files.Files) != 1 || files.NextPageToken != "page-2" || files.Files[0].Size != 42 {
		t.Fatalf("ListFilesPage = %#v, %v", files, err)
	}
	changes, err := client.ListChangesPage(context.Background(), token, 100)
	if err != nil || len(changes.Changes) != 2 || changes.NewStartPageToken != "change-12" || !changes.Changes[1].Removed {
		t.Fatalf("ListChangesPage = %#v, %v", changes, err)
	}
}

func TestDriveFetchTextExportsDocsAndLeavesBinaryMetadataOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/doc-1/export" || r.URL.Query().Get("mimeType") != "text/plain" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("source-backed document text"))
	}))
	defer server.Close()

	client := DriveClient{AccessToken: "token", BaseURL: server.URL}
	text, fetched, err := client.FetchText(context.Background(), DriveFile{ID: "doc-1", MimeType: "application/vnd.google-apps.document"})
	if err != nil || !fetched || text != "source-backed document text" {
		t.Fatalf("FetchText = %q, %v, %v", text, fetched, err)
	}
	text, fetched, err = client.FetchText(context.Background(), DriveFile{ID: "pdf-1", MimeType: "application/pdf", Size: 100})
	if err != nil || fetched || strings.TrimSpace(text) != "" {
		t.Fatalf("binary FetchText = %q, %v, %v", text, fetched, err)
	}
}
