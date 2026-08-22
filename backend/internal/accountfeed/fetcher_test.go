package accountfeed

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFeedURLRejectsPrivateNetworkAddresses(t *testing.T) {
	for _, raw := range []string{
		"http://10.0.0.7/feed.json",
		"http://172.16.0.7/feed.json",
		"http://192.168.0.7/feed.json",
		"http://[fc00::7]/feed.json",
	} {
		if err := validateFeedURL(raw); err == nil {
			t.Fatalf("validateFeedURL(%q) unexpectedly allowed a private address", raw)
		}
	}
}

func TestValidateFeedURLRejectsEmbeddedCredentials(t *testing.T) {
	err := validateFeedURL("https://operator:secret@example.test/feed.json")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("validateFeedURL accepted embedded credentials: %v", err)
	}
}

func TestFetchFeedBytesDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer entry.Close()

	_, err := fetchFeedBytes(context.Background(), Feed{
		SourceType: SourceHTTPJSONFeed,
		URL:        entry.URL,
	}, FetchOptions{AllowHTTP: true})
	if err == nil || !strings.Contains(err.Error(), "feed HTTP 302") {
		t.Fatalf("redirect error=%v, want HTTP 302", err)
	}
	if redirected {
		t.Fatal("feed fetch followed a redirect")
	}
}

func TestAccountFeedTransportRejectsPrivateResolvedAddress(t *testing.T) {
	dialed := false
	transport := newAccountFeedHTTPTransport(
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.7")}}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, nil
		},
	)

	_, err := transport.DialContext(context.Background(), "tcp", "feed.example:80")
	if err == nil || !strings.Contains(err.Error(), "blocked address space") {
		t.Fatalf("DialContext error=%v, want blocked address space", err)
	}
	if dialed {
		t.Fatal("transport dialed a private resolved address")
	}
}

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
