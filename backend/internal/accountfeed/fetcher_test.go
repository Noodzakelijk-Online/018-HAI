package accountfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchFeedBytesRejectsOversizedLocalFeedBeforeLoadingIt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "oversized.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxFeedBytes+1)), 0o600); err != nil {
		t.Fatalf("write oversized feed: %v", err)
	}

	_, err := fetchFeedBytes(context.Background(), Feed{
		SourceType: SourceLocalJSONFile,
		Path:       "oversized.json",
	}, FetchOptions{FeedsRoot: root})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("oversized local feed error=%v", err)
	}
}

func TestFetchFeedBytesRejectsOversizedHTTPFeedInsteadOfTruncatingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxFeedBytes+1)))
	}))
	defer server.Close()

	_, err := fetchFeedBytes(context.Background(), Feed{
		SourceType: SourceHTTPJSONFeed,
		URL:        server.URL,
	}, FetchOptions{AllowHTTP: true})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("oversized HTTP feed error=%v", err)
	}
}
