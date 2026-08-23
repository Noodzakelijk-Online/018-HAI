package accountfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchHTTPFeedRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(strings.Repeat("x", maxFeedBytes+1)))
	}))
	defer server.Close()

	feed := testFeed("")
	feed.SourceType = SourceHTTPJSONFeed
	feed.URL = server.URL
	if _, err := fetchFeedBytes(context.Background(), feed, FetchOptions{AllowHTTP: true}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("fetchFeedBytes error = %v, want explicit size-limit rejection", err)
	}
}

func TestFetchHTTPFeedDoesNotFollowRedirects(t *testing.T) {
	finalRequests := 0
	final := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		finalRequests++
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer final.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	feed := testFeed("")
	feed.SourceType = SourceHTTPJSONFeed
	feed.URL = redirect.URL
	if _, err := fetchFeedBytes(context.Background(), feed, FetchOptions{AllowHTTP: true}); err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("fetchFeedBytes error = %v, want redirect rejection", err)
	}
	if finalRequests != 0 {
		t.Fatalf("redirect target received %d request(s), want none", finalRequests)
	}
}

func TestValidateFeedURLRejectsPrivateNetworkAddress(t *testing.T) {
	if err := validateFeedURL("http://192.168.1.10/feed.json"); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("validateFeedURL error = %v, want private-address rejection", err)
	}
}
