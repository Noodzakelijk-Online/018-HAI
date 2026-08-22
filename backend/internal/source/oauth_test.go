package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/googleoauth"
	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

// gmailIncrementalQuery turns the stored cursor into Gmail's `q` filter so a
// sync fetches only mail newer than the last run.
func TestGmailIncrementalQuery(t *testing.T) {
	cases := []struct {
		name, cursor, want string
	}{
		{"empty cursor fetches recent", "", ""},
		{"unparseable cursor is ignored", "not-a-time", ""},
		{"rfc3339 cursor becomes after: filter", "2026-07-23T03:39:26Z", "after:1784777966"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gmailIncrementalQuery(tc.cursor); got != tc.want {
				t.Fatalf("gmailIncrementalQuery(%q) = %q, want %q", tc.cursor, got, tc.want)
			}
		})
	}
}

func TestGmailBackfillCapturesHistoryBoundaryAndMessageContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/me/profile":
			_, _ = w.Write([]byte(`{"historyId":"100"}`))
		case "/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"}]}`))
		case "/users/me/messages/m1":
			_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","historyId":"99","snippet":"Follow up: send the requested document.","internalDate":"1700000000000","payload":{"headers":[{"name":"From","value":"lawyer@example.com"},{"name":"Subject","value":"Document request"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := &models.ConnectedSource{DefaultProjectKey: "legal"}
	items, cursorValue, err := fetchGmailSourceWithClient(context.Background(), googleoauth.GmailClient{AccessToken: "token", BaseURL: server.URL}, source)
	if err != nil || len(items) != 1 || !strings.Contains(items[0].Content, "requested document") || !strings.Contains(items[0].Metadata, `"historyId":"99"`) {
		t.Fatalf("items=%#v cursor=%q err=%v", items, cursorValue, err)
	}
	cursor, err := decodeGmailCursor(cursorValue)
	if err != nil || cursor.Phase != "history" || cursor.HistoryID != "100" {
		t.Fatalf("cursor=%#v err=%v", cursor, err)
	}
}

func TestGoogleOAuthStateFailsClosedWithoutDedicatedKey(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	config.AppConfig.OAuthStateSigningKey = ""
	if _, err := signState(uuid.Nil); err == nil {
		t.Fatal("signState must fail without a dedicated signing key")
	}
}

func TestGoogleOAuthConfigUsesOneConnectorSpecificReadonlyScope(t *testing.T) {
	cases := []struct {
		connector string
		wantScope string
	}{
		{gmailConnectorKey, googleoauth.GmailReadonlyScope},
		{driveConnectorKey, googleoauth.DriveReadonlyScope},
		{contactsConnectorKey, googleoauth.ContactsReadonlyScope},
		{calendarConnectorKey, googleoauth.CalendarReadonlyScope},
	}
	for _, tc := range cases {
		t.Run(tc.connector, func(t *testing.T) {
			cfg, err := googleOAuthConfigForConnector(tc.connector)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Scopes) != 1 || cfg.Scopes[0] != tc.wantScope {
				t.Fatalf("scopes = %#v, want only %q", cfg.Scopes, tc.wantScope)
			}
		})
	}
	if _, err := googleOAuthConfigForConnector("unknown"); err == nil {
		t.Fatal("unknown connector must not receive a Google OAuth configuration")
	}
}

func TestGmailCursorUpgradesTimestampAndRoundTripsNativeState(t *testing.T) {
	legacy, err := decodeGmailCursor("2026-07-23T03:39:26Z")
	if err != nil || legacy.Phase != "backfill" {
		t.Fatalf("legacy cursor = %#v, %v", legacy, err)
	}
	encoded, err := encodeGmailCursor(gmailCursor{Phase: "history", HistoryID: "123", PageToken: "next"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeGmailCursor(encoded)
	if err != nil || decoded.Phase != "history" || decoded.HistoryID != "123" || decoded.PageToken != "next" {
		t.Fatalf("decoded cursor = %#v, %v", decoded, err)
	}
}

func TestGoogleConnectionHealthDistinguishesDisconnectedAndReady(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	config.AppConfig.GoogleOAuthClientID = "client"
	config.AppConfig.GoogleOAuthClientSecret = "secret"
	config.AppConfig.GoogleOAuthRedirectURL = "https://example.test/callback"
	config.AppConfig.OAuthTokenEncryptionKey = "token-key"
	config.AppConfig.OAuthStateSigningKey = "state-key"

	sourceID := uuid.New()
	cursor, _ := encodeGmailCursor(gmailCursor{Phase: "history", HistoryID: "100"})
	repo := newFakeSourceRepo(&models.ConnectedSource{ID: sourceID, ConnectorKey: gmailConnectorKey, Enabled: true, Status: "active", Cursor: cursor})
	service := NewService(repo, nil).(*service)
	health, err := service.ConnectionHealth(sourceID)
	if err != nil || health.Status != "disconnected" || health.Authorized {
		t.Fatalf("disconnected health = %#v, %v", health, err)
	}
	if err := repo.SaveOAuthToken(&models.SourceOAuthToken{SourceID: sourceID, Scope: googleoauth.GmailReadonlyScope, RefreshToken: []byte("encrypted"), Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	health, err = service.ConnectionHealth(sourceID)
	if err != nil || health.Status != "ready" || !health.Authorized || health.CursorPhase != "history" {
		t.Fatalf("ready health = %#v, %v", health, err)
	}
}

func TestTrelloConnectionHealthRequiresCredentialsAndValidBoardTarget(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, ConnectorKey: trelloConnectorKey, Enabled: true, Status: "active",
		SyncTarget: "https://trello.com/b/abc12345/example-board",
	})
	service := NewService(repo, nil).(*service)

	health, err := service.ConnectionHealth(sourceID)
	if err != nil || health.Status != "configuration_required" || health.Configured || health.Authorized {
		t.Fatalf("unconfigured Trello health = %#v, %v", health, err)
	}

	t.Setenv(trelloAPIKeyEnv, "key")
	t.Setenv(trelloReadTokenEnv, "read-token")
	health, err = service.ConnectionHealth(sourceID)
	if err != nil || health.Status != "ready" || !health.Configured || health.Authorized {
		t.Fatalf("configured Trello health = %#v, %v", health, err)
	}

	repo.sources[sourceID].SyncTarget = "not a board id"
	health, err = service.ConnectionHealth(sourceID)
	if err != nil || health.Status != "configuration_required" || health.Configured {
		t.Fatalf("invalid Trello target health = %#v, %v", health, err)
	}
}
