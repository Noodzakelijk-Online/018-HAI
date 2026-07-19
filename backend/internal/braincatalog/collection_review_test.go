package braincatalog

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOSSInsightCollectionReviewerUsesFixedBoundedCollectionList(t *testing.T) {
	var requestedURL string
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		requestedURL = req.URL.String()
		if req.Header.Get("User-Agent") != ossInsightCollectionReviewAgent {
			t.Fatalf("unexpected User-Agent: %q", req.Header.Get("User-Agent"))
		}
		body := `{"data":{"rows":[{"name":"MCP Servers"},{"name":"Future collection"}]}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	reviewer := NewOSSInsightCollectionReviewer(client).(*ossInsightCollectionReviewer)
	reviewer.now = func() time.Time { return time.Date(2026, 7, 19, 16, 0, 0, 0, time.UTC) }

	review, err := reviewer.ReviewCollections()
	if err != nil {
		t.Fatalf("ReviewCollections() error = %v", err)
	}
	if requestedURL != ossInsightCollectionsURL || !review.Available || review.ExpectedTotal != 138 || review.CurrentTotal != 2 {
		t.Fatalf("unexpected collection review: %#v", review)
	}
	if len(review.NewCollections) != 1 || review.NewCollections[0] != "Future collection" || len(review.MissingExpected) != 137 {
		t.Fatalf("unexpected collection drift: %#v", review)
	}
	if !strings.Contains(review.Message, "did not install") {
		t.Fatalf("review must preserve activation boundary: %#v", review)
	}
}
