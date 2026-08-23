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
