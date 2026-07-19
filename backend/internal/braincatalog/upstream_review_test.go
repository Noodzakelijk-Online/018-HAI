package braincatalog

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type reviewRoundTripper func(*http.Request) (*http.Response, error)

func (f reviewRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestUpstreamReviewerUsesFixedGitHubMetadataRequest(t *testing.T) {
	var requestedURL string
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		requestedURL = req.URL.String()
		if req.Header.Get("User-Agent") != "HAI-BrainCatalog/1.0" {
			t.Fatalf("unexpected User-Agent: %q", req.Header.Get("User-Agent"))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"archived":false,"default_branch":"main","pushed_at":"2026-07-19T12:00:00Z","license":{"spdx_id":"MIT"}}`)), Header: make(http.Header)}, nil
	})}
	reviewer := NewUpstreamReviewer(client).(*githubUpstreamReviewer)
	reviewer.now = func() time.Time { return time.Date(2026, 7, 19, 12, 30, 0, 0, time.UTC) }

	review, err := reviewer.Review(Entry{ID: "opencode", Name: "OpenCode", UpstreamURL: "https://github.com/anomalyco/opencode", Status: StatusCandidate})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if requestedURL != "https://api.github.com/repos/anomalyco/opencode" {
		t.Fatalf("unexpected metadata URL: %s", requestedURL)
	}
	if !review.Available || review.Archived || review.License != "MIT" || review.DefaultBranch != "main" || review.Disposition != StatusCandidate {
		t.Fatalf("unexpected review: %#v", review)
	}
	if !strings.Contains(review.Message, "does not install") {
		t.Fatalf("review must preserve activation boundary: %#v", review)
	}
}

func TestGitHubRepositoryPathRejectsNonRepositoryURLs(t *testing.T) {
	for _, rawURL := range []string{
		"http://github.com/anomalyco/opencode",
		"https://github.com/anomalyco/opencode/issues",
		"https://github.com/anomalyco/opencode?token=secret",
		"https://example.com/anomalyco/opencode",
		"https://user:pass@github.com/anomalyco/opencode",
	} {
		if _, _, err := githubRepositoryPath(rawURL); err == nil {
			t.Fatalf("githubRepositoryPath(%q) unexpectedly succeeded", rawURL)
		}
	}
}
