package braincatalog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type discoveryScoutStub struct {
	report OSSInsightRepositoryDiscoveryReport
	err    error
}

func (s discoveryScoutStub) DiscoverRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	return s.report, s.err
}

func (s discoveryScoutStub) DiscoverReviewableRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	return s.report, s.err
}

func (s discoveryScoutStub) DiscoverRepositoriesFor(_ OSSInsightDiscoveryScope) (OSSInsightRepositoryDiscoveryReport, error) {
	return s.report, s.err
}

type discoveryReviewerStub struct {
	entry Entry
}

func (r *discoveryReviewerStub) Review(entry Entry) (UpstreamReview, error) {
	r.entry = entry
	return UpstreamReview{ID: entry.ID, Name: entry.Name, UpstreamURL: entry.UpstreamURL, Available: true, License: "MIT", Disposition: entry.Status}, nil
}

func TestRevalidateDiscoveryRequiresCurrentSourceDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reviewer := &discoveryReviewerStub{}
	handler := NewHandlerWithReviewersAndScout(reviewer, nil, discoveryScoutStub{report: OSSInsightRepositoryDiscoveryReport{
		Discoveries: []OSSInsightRepositoryDiscovery{{Collection: "MCP Servers", Repository: "owner/verified-repo", SourceURL: "https://api.ossinsight.io/v1/collections/1/repos/", Rationale: "candidate"}},
	}})

	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"repository":"owner/verified-repo"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler.RevalidateDiscovery(context)

	if response.Code != http.StatusOK {
		t.Fatalf("expected metadata review success, got %d: %s", response.Code, response.Body.String())
	}
	if reviewer.entry.UpstreamURL != "https://github.com/owner/verified-repo" {
		t.Fatalf("unexpected derived upstream: %#v", reviewer.entry)
	}
	if reviewer.entry.Status != StatusCandidate {
		t.Fatalf("discovery review must remain a candidate: %#v", reviewer.entry)
	}
}

func TestRevalidateDiscoveryRejectsRepositoryOutsideCurrentReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reviewer := &discoveryReviewerStub{}
	handler := NewHandlerWithReviewersAndScout(reviewer, nil, discoveryScoutStub{})

	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"repository":"attacker/arbitrary"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler.RevalidateDiscovery(context)

	if response.Code != http.StatusNotFound || reviewer.entry.ID != "" {
		t.Fatalf("arbitrary repository must not be reviewed: status=%d entry=%#v", response.Code, reviewer.entry)
	}
}

func TestEntryForDiscoveryRejectsUnsafeRepositoryPath(t *testing.T) {
	if _, err := entryForDiscovery(OSSInsightRepositoryDiscovery{Repository: "owner/repo?redirect=https://example.invalid"}); err == nil {
		t.Fatal("unsafe repository path must be rejected")
	}
}
