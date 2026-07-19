package braincatalog

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOSSInsightRepositoryScoutOnlyQueriesCandidateCollectionEndpoints(t *testing.T) {
	requested := []string{}
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.String())
		body := `{"data":{"rows":[{"id":"10105","name":"MCP Servers"},{"id":"10136","name":"AI Code Review"},{"id":"10030","name":"Finance"}]}}`
		switch req.URL.String() {
		case ossInsightCollectionsURL:
		case "https://api.ossinsight.io/v1/collections/10105/repos/":
			body = `{"data":{"rows":[{"repo_name":"github/github-mcp-server"},{"repo_name":"owner/new-mcp"}]}}`
		case "https://api.ossinsight.io/v1/collections/10136/repos/":
			body = `{"data":{"rows":[{"repo_name":"qodo-ai/pr-agent"},{"repo_name":"owner/new-review"}]}}`
		default:
			t.Fatalf("unexpected source request: %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	scout := NewOSSInsightRepositoryScout(client).(*ossInsightRepositoryScout)
	scout.now = func() time.Time { return time.Date(2026, 7, 19, 18, 0, 0, 0, time.UTC) }

	report, err := scout.DiscoverRepositories()
	if err != nil {
		t.Fatalf("DiscoverRepositories() error = %v", err)
	}
	if !report.Available || report.CollectionsScreened != 138 || report.CandidateCollections == 0 || report.CollectionsChecked != 2 || report.RepositoriesChecked != 4 || report.KnownProfileHits != 2 {
		t.Fatalf("unexpected discovery report: %#v", report)
	}
	if len(report.Discoveries) != 2 || report.Discoveries[0].Repository != "owner/new-mcp" || report.Discoveries[1].Repository != "owner/new-review" {
		t.Fatalf("unexpected discoveries: %#v", report.Discoveries)
	}
	for _, url := range requested {
		if strings.Contains(url, "/10030/") {
			t.Fatalf("deferred collection must not be queried: %v", requested)
		}
	}
	if !strings.Contains(report.Message, "did not add catalog entries") {
		t.Fatalf("discovery must preserve activation boundary: %#v", report)
	}
	cached, err := scout.DiscoverRepositories()
	if err != nil || !cached.Cached || len(requested) != 3 {
		t.Fatalf("repeat discovery must reuse the bounded report: cached=%#v err=%v requests=%v", cached, err, requested)
	}
}
