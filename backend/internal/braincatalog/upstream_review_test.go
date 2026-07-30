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
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"full_name":"anomalyco/opencode","html_url":"https://github.com/anomalyco/opencode","archived":false,"default_branch":"main","pushed_at":"2026-07-19T12:00:00Z","license":{"spdx_id":"MIT"}}`)), Header: make(http.Header)}, nil
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
	if !review.Available || review.Archived || review.License != "MIT" || review.DefaultBranch != "main" || review.Disposition != StatusCandidate || review.Readiness != readinessReviewNow {
		t.Fatalf("unexpected review: %#v", review)
	}
	if !strings.Contains(review.Message, "does not install") {
		t.Fatalf("review must preserve activation boundary: %#v", review)
	}
	if review.RepositoryMoved || review.ResolvedRepository != "anomalyco/opencode" || review.ResolvedUpstreamURL != "https://github.com/anomalyco/opencode" {
		t.Fatalf("unexpected resolved repository metadata: %#v", review)
	}
}

func TestUpstreamReviewerReportsRenamedRepositoryWithoutChangingDisposition(t *testing.T) {
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"full_name":"new-owner/new-repository","html_url":"https://github.com/new-owner/new-repository","archived":false,"default_branch":"main","license":{"spdx_id":"Apache-2.0"}}`)), Header: make(http.Header)}, nil
	})}

	review, err := NewUpstreamReviewer(client).Review(Entry{ID: "renamed", Name: "Renamed project", UpstreamURL: "https://github.com/original-owner/original-repository", Status: StatusCandidate})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !review.RepositoryMoved || review.ResolvedRepository != "new-owner/new-repository" || review.ResolvedUpstreamURL != "https://github.com/new-owner/new-repository" {
		t.Fatalf("rename evidence = %#v", review)
	}
	if review.Disposition != StatusCandidate || !strings.Contains(review.Message, "has not changed") {
		t.Fatalf("rename check must retain a non-mutating review boundary: %#v", review)
	}
}

func TestUpstreamReviewerRetriesTransientMetadataResponse(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`temporary failure`)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"archived":false,"default_branch":"main","pushed_at":"2026-07-21T10:00:00Z","license":{"spdx_id":"MIT"}}`)), Header: make(http.Header)}, nil
	})}

	review, err := NewUpstreamReviewer(client).Review(Entry{ID: "candidate", Name: "Candidate", UpstreamURL: "https://github.com/owner/candidate", Status: StatusCandidate})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !review.Available || attempts != 2 {
		t.Fatalf("review=%#v attempts=%d, want available after two attempts", review, attempts)
	}
}

func TestUpstreamReviewerDoesNotRetryPermanentMetadataError(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`forbidden`)), Header: make(http.Header)}, nil
	})}

	if _, err := NewUpstreamReviewer(client).Review(Entry{ID: "candidate", Name: "Candidate", UpstreamURL: "https://github.com/owner/candidate", Status: StatusCandidate}); err == nil {
		t.Fatal("expected permanent metadata error")
	}
	if attempts != 1 {
		t.Fatalf("permanent metadata error attempts=%d, want 1", attempts)
	}
}

func TestReadinessAssessmentHoldsArchivedAndLicenseUnclearProjects(t *testing.T) {
	for _, test := range []struct {
		name      string
		entry     Entry
		review    UpstreamReview
		readiness string
	}{
		{name: "archived", entry: Entry{Status: StatusCandidate}, review: UpstreamReview{Available: true, Archived: true, License: "MIT"}, readiness: readinessArchived},
		{name: "no assertion", entry: Entry{Status: StatusCandidate}, review: UpstreamReview{Available: true, License: "NOASSERTION"}, readiness: readinessLicenseReview},
		{name: "excluded", entry: Entry{Status: StatusExcluded}, review: UpstreamReview{Available: true, License: "MIT"}, readiness: readinessNotAdopted},
	} {
		t.Run(test.name, func(t *testing.T) {
			applyReadinessAssessment(test.entry, &test.review)
			if test.review.Readiness != test.readiness || len(test.review.RequiredGates) == 0 {
				t.Fatalf("assessment = %#v", test.review)
			}
		})
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
