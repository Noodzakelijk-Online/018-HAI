package googleoauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCalendarClientUsesReadonlyBoundedInitialRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request = %s auth=%q", request.Method, request.Header.Get("Authorization"))
		}
		query := request.URL.Query()
		if query.Get("timeMin") == "" || query.Get("syncToken") != "" || query.Get("showDeleted") != "true" || query.Get("singleEvents") != "true" {
			t.Fatalf("initial query = %v", query)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"items":[{"id":"event-1","summary":"Review"}],"nextSyncToken":"sync-1"}`))
	}))
	defer server.Close()

	page, err := (CalendarClient{AccessToken: "token", BaseURL: server.URL}).ListPrimaryEventsPage(context.Background(), "", "", "2025-01-01T00:00:00Z", 200)
	if err != nil || len(page.Events) != 1 || page.NextSyncToken != "sync-1" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestCalendarClientIncrementalRequestOmitsTimeMin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("syncToken") != "sync-1" || query.Get("pageToken") != "page-2" || query.Has("timeMin") {
			t.Fatalf("incremental query = %v", query)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"items":[],"nextSyncToken":"sync-2"}`))
	}))
	defer server.Close()
	_, err := (CalendarClient{AccessToken: "token", BaseURL: server.URL}).ListPrimaryEventsPage(context.Background(), "page-2", "sync-1", "ignored", 200)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCalendarClientReturnsTypedExpiredSyncToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusGone) }))
	defer server.Close()
	_, err := (CalendarClient{AccessToken: "token", BaseURL: server.URL}).ListPrimaryEventsPage(context.Background(), "", "expired", "", 200)
	if !errors.Is(err, ErrCalendarSyncTokenExpired) {
		t.Fatalf("error = %v", err)
	}
}
