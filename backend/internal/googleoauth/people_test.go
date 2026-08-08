package googleoauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPeopleClientUsesReadOnlyBoundedIncrementalRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("authorization header was not set")
		}
		query := request.URL.Query()
		if query.Get("syncToken") != "sync-1" || query.Get("pageToken") != "page-2" {
			t.Fatalf("cursor query = %v", query)
		}
		if query.Has("requestSyncToken") {
			t.Fatal("incremental request must not request a second initial sync token")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"connections":[{"resourceName":"people/1","names":[{"displayName":"Ada Lovelace"}]}],"nextSyncToken":"sync-2"}`))
	}))
	defer server.Close()

	page, err := (PeopleClient{AccessToken: "access-token", BaseURL: server.URL}).
		ListConnectionsPage(context.Background(), "page-2", "sync-1", 200)
	if err != nil {
		t.Fatalf("ListConnectionsPage: %v", err)
	}
	if len(page.Connections) != 1 || page.Connections[0].Names[0].DisplayName != "Ada Lovelace" || page.NextSyncToken != "sync-2" {
		t.Fatalf("page = %#v", page)
	}
}

func TestPeopleClientExplainsExpiredSyncToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusGone)
	}))
	defer server.Close()
	_, err := (PeopleClient{AccessToken: "token", BaseURL: server.URL}).
		ListConnectionsPage(context.Background(), "", "expired", 100)
	if err == nil {
		t.Fatal("expired People sync token was accepted")
	}
	if !errors.Is(err, ErrPeopleSyncTokenExpired) {
		t.Fatalf("expired token error = %v", err)
	}
}
