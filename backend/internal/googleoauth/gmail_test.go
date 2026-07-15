package googleoauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockGmail serves the two Gmail endpoints the client uses, and asserts the
// bearer token is presented.
func mockGmail(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			t.Errorf("Authorization = %q, want bearer test-access-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/users/me/messages/"):
			id := strings.TrimPrefix(r.URL.Path, "/users/me/messages/")
			_, _ = w.Write([]byte(`{
				"id":"` + id + `",
				"snippet":"Hello from message ` + id + `",
				"internalDate":"1700000000000",
				"payload":{"headers":[
					{"name":"From","value":"alice@example.com"},
					{"name":"Subject","value":"Subject of ` + id + `"},
					{"name":"Date","value":"Tue, 14 Nov 2023 22:13:20 +0000"}
				]}
			}`))
		case strings.HasSuffix(r.URL.Path, "/users/me/messages"):
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestGmailFetchRecentParsesHeadersAndSnippet(t *testing.T) {
	srv := mockGmail(t)
	defer srv.Close()

	client := GmailClient{AccessToken: "test-access-token", BaseURL: srv.URL}
	msgs, err := client.FetchRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchRecent: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	first := msgs[0]
	if first.ID != "m1" {
		t.Errorf("id = %q, want m1", first.ID)
	}
	if first.From != "alice@example.com" {
		t.Errorf("from = %q", first.From)
	}
	if first.Subject != "Subject of m1" {
		t.Errorf("subject = %q", first.Subject)
	}
	if first.Snippet == "" {
		t.Error("snippet should be populated")
	}
	if first.Date.IsZero() {
		t.Error("date should be parsed from internalDate")
	}
}

func TestGmailSurfacesExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401}}`))
	}))
	defer srv.Close()

	client := GmailClient{AccessToken: "stale", BaseURL: srv.URL}
	_, err := client.ListRecentMessageIDs(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected a clear 401/expired error, got %v", err)
	}
}

// One malformed message must not fail the whole sync.
func TestGmailSkipsUnfetchableMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/me/messages"):
			_, _ = w.Write([]byte(`{"messages":[{"id":"ok"},{"id":"bad"}]}`))
		case strings.HasSuffix(r.URL.Path, "/messages/ok"):
			_, _ = w.Write([]byte(`{"id":"ok","snippet":"fine","payload":{"headers":[]}}`))
		default: // "bad" returns 500
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := GmailClient{AccessToken: "t", BaseURL: srv.URL}
	msgs, err := client.FetchRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchRecent should not fail on one bad message: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "ok" {
		t.Fatalf("want only the good message, got %+v", msgs)
	}
}
