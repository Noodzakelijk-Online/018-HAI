package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/googleoauth"
	"automation-hub-backend/internal/models"
)

func TestDriveBackfillCapturesBoundaryAndAdvancesPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/changes/startPageToken":
			_, _ = w.Write([]byte(`{"startPageToken":"changes-10"}`))
		case "/files":
			_, _ = w.Write([]byte(`{"nextPageToken":"files-2","files":[{"id":"doc-1","name":"Project plan","mimeType":"application/vnd.google-apps.document","modifiedTime":"2026-08-03T10:00:00Z","webViewLink":"https://drive.google.com/document/d/doc-1"}]}`))
		case "/files/doc-1/export":
			_, _ = w.Write([]byte("Decision: use live incremental sync. Follow up: verify the workflow."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := &models.ConnectedSource{DefaultProjectKey: "018-HAI"}
	items, cursorValue, err := fetchDriveSourceWithClient(context.Background(), googleoauth.DriveClient{AccessToken: "token", BaseURL: server.URL}, source)
	if err != nil {
		t.Fatalf("fetchDriveSourceWithClient: %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Content, "live incremental sync") || !strings.Contains(items[0].Metadata, `"contentFetched":true`) {
		t.Fatalf("items = %#v", items)
	}
	cursor, err := decodeDriveCursor(cursorValue)
	if err != nil || cursor.Phase != "backfill" || cursor.PageToken != "files-2" || cursor.ChangeToken != "changes-10" {
		t.Fatalf("cursor = %#v, %v", cursor, err)
	}
}

func TestDriveChangesRetainRemovalAsReviewableTombstone(t *testing.T) {
	cursorValue, err := encodeDriveCursor(driveCursor{Phase: "changes", PageToken: "changes-10"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"newStartPageToken":"changes-11","changes":[{"fileId":"gone","removed":true,"time":"2026-08-03T11:00:00Z"}]}`))
	}))
	defer server.Close()

	source := &models.ConnectedSource{Cursor: cursorValue, DefaultProjectKey: "018-HAI"}
	items, next, err := fetchDriveSourceWithClient(context.Background(), googleoauth.DriveClient{AccessToken: "token", BaseURL: server.URL}, source)
	if err != nil || len(items) != 1 || items[0].ItemType != "drive_file_removed" || !strings.Contains(items[0].Content, "do not treat removal as permission") {
		t.Fatalf("items=%#v next=%q err=%v", items, next, err)
	}
	decoded, err := decodeDriveCursor(next)
	if err != nil || decoded.PageToken != "changes-11" {
		t.Fatalf("next cursor = %#v, %v", decoded, err)
	}
}
