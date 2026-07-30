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
			body = `{"data":{"rows":[{"repo_name":"github/github-mcp-server"},{"repo_name":"owner/new-mcp"}],"result":{"limit":50}}}`
		case "https://api.ossinsight.io/v1/collections/10136/repos/":
			body = `{"data":{"rows":[{"repo_name":"qodo-ai/pr-agent"},{"repo_name":"owner/new-review"}],"result":{"limit":50}}}`
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
	if len(report.KnownProfiles) != 2 || report.KnownProfiles[0].Repository != "github/github-mcp-server" || len(report.KnownProfiles[0].CatalogEntryIDs) != 1 || report.KnownProfiles[0].CatalogEntryIDs[0] != "github-mcp-server" {
		t.Fatalf("known profiles must remain source-linked and catalog-linked: %#v", report.KnownProfiles)
	}
	if len(report.Discoveries) != 2 || report.Discoveries[0].Repository != "owner/new-mcp" || report.Discoveries[1].Repository != "owner/new-review" {
		t.Fatalf("unexpected discoveries: %#v", report.Discoveries)
	}
	if report.MaximumDiscoveries < 800 || report.SourceQueryLimit != 50 || report.CollectionsAtQueryLimit != 0 {
		t.Fatalf("discovery capacity and source-query context must be explicit: %#v", report)
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

func TestOSSInsightRepositoryScoutReviewableScopeQueriesRepresentedCategoriesSeparately(t *testing.T) {
	requested := []string{}
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.String())
		body := `{"data":{"rows":[{"id":"10105","name":"MCP Servers"},{"id":"10109","name":"LLM Inference Engines"},{"id":"10030","name":"Finance"}]}}`
		switch req.URL.String() {
		case ossInsightCollectionsURL:
		case "https://api.ossinsight.io/v1/collections/10105/repos/":
			body = `{"data":{"rows":[{"repo_name":"owner/new-mcp"}],"result":{"limit":50}}}`
		case "https://api.ossinsight.io/v1/collections/10109/repos/":
			body = `{"data":{"rows":[{"repo_name":"owner/new-inference"}],"result":{"limit":50}}}`
		default:
			t.Fatalf("unexpected source request: %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	scout := NewOSSInsightRepositoryScout(client).(*ossInsightRepositoryScout)
	scout.now = func() time.Time { return time.Date(2026, 7, 19, 18, 0, 0, 0, time.UTC) }

	report, err := scout.DiscoverReviewableRepositories()
	if err != nil {
		t.Fatalf("DiscoverReviewableRepositories() error = %v", err)
	}
	if !report.Available || report.Scope != OSSInsightReviewableScope || report.EligibleCollections <= report.CandidateCollections || report.CollectionsChecked != 2 || len(report.Discoveries) != 2 {
		t.Fatalf("unexpected reviewable report: %#v", report)
	}
	if report.Discoveries[0].Repository != "owner/new-inference" || report.Discoveries[0].Disposition != CollectionRepresented || report.Discoveries[0].Priority <= report.Discoveries[1].Priority {
		t.Fatalf("priority-ranked represented discovery must keep its collection disposition: %#v", report.Discoveries)
	}
	for _, url := range requested {
		if strings.Contains(url, "/10030/") {
			t.Fatalf("deferred collection must not be queried: %v", requested)
		}
	}

	candidate, err := scout.DiscoverRepositories()
	if err != nil || candidate.Scope != OSSInsightCandidateScope || len(candidate.Discoveries) != 1 {
		t.Fatalf("candidate cache must remain separate from reviewable scope: report=%#v err=%v", candidate, err)
	}
}

func TestOSSInsightRepositoryScoutRetriesTransientRepositoryResponse(t *testing.T) {
	repositoryAttempts := 0
	client := &http.Client{Transport: reviewRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case ossInsightCollectionsURL:
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"rows":[{"id":"10105","name":"MCP Servers"}]}}`)), Header: make(http.Header)}, nil
		case "https://api.ossinsight.io/v1/collections/10105/repos/":
			repositoryAttempts++
			if repositoryAttempts == 1 {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`slow down`)), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"rows":[{"repo_name":"owner/retry-safe"}],"result":{"limit":20}}}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected source request: %s", req.URL)
			return nil, nil
		}
	})}
	scout := NewOSSInsightRepositoryScout(client).(*ossInsightRepositoryScout)
	report, err := scout.DiscoverRepositories()
	if err != nil {
		t.Fatalf("DiscoverRepositories() error = %v", err)
	}
	if !report.Available || repositoryAttempts != 2 || len(report.Discoveries) != 1 || report.Discoveries[0].Repository != "owner/retry-safe" {
		t.Fatalf("report=%#v repositoryAttempts=%d", report, repositoryAttempts)
	}
}

func TestOSSInsightRepositoryScoutRejectsUnknownScope(t *testing.T) {
	scout := NewOSSInsightRepositoryScout(&http.Client{}).(*ossInsightRepositoryScout)
	if _, err := scout.DiscoverRepositoriesFor("all"); err == nil {
		t.Fatal("unknown repository discovery scope must be rejected")
	}
}

func TestCatalogRepositoriesIncludesOnlyExplicitReviewedAliases(t *testing.T) {
	known := catalogRepositories()
	for _, repository := range []string{
		"all-hands-ai/openhands",
		"prefecthq/fastmcp",
		"googleapis/genai-toolbox",
		"paul-gauthier/aider",
		"microsoft/presidio",
		"codium-ai/pr-agent",
		"block/goose",
	} {
		if !known[repository] {
			t.Fatalf("reviewed upstream alias %q must suppress a duplicate discovery", repository)
		}
	}
	if !known["opencode-ai/opencode"] {
		t.Fatal("the explicitly reviewed archived same-name repository must not remain a viable discovery candidate")
	}
}

func TestCatalogRepositoryEntryIDsPreserveTheProfileForAReviewedAlias(t *testing.T) {
	profiles := catalogRepositoryEntryIDs()
	if ids := profiles["prefecthq/fastmcp"]; len(ids) != 1 || ids[0] != "fastmcp" {
		t.Fatalf("reviewed alias must retain its catalog profile: %#v", ids)
	}
	if ids := profiles["googleapis/genai-toolbox"]; len(ids) != 1 || ids[0] != "google-genai-toolbox" {
		t.Fatalf("renamed MCP Toolbox repository must retain its catalog profile: %#v", ids)
	}
}

func TestRepositoryDiscoveryTriageKeepsKnownOverlapsAndHighRiskSurfacesOutOfAdapterReview(t *testing.T) {
	for repository, wantStatus := range map[string]string{
		"HKUDS/RAG-Anything":        discoveryTriageReference,
		"apache/airflow":             discoveryTriageReference,
		"e2b-dev/open-computer-use": discoveryTriageDeferred,
		"stripe/agent-toolkit":       discoveryTriageDeferred,
	} {
		status, reason, reviewAllowed := repositoryDiscoveryTriage(repository)
		if status != wantStatus || reviewAllowed || strings.TrimSpace(reason) == "" {
			t.Fatalf("triage(%q) = status=%q reviewAllowed=%v reason=%q", repository, status, reviewAllowed, reason)
		}
	}
	status, _, reviewAllowed := repositoryDiscoveryTriage("owner/new-reviewed-candidate")
	if status != discoveryTriageManualReview || !reviewAllowed {
		t.Fatalf("unknown repository must remain a manual review candidate: status=%q reviewAllowed=%v", status, reviewAllowed)
	}
}

func TestMergeDiscoveryPreservesSourceProvenanceAndStrongerTrack(t *testing.T) {
	existing := OSSInsightRepositoryDiscovery{
		Collection: "AI Agent Frameworks", Repository: "owner/shared", ReviewTrack: "orchestration", Priority: 68,
		RelatedCollections: []string{"AI Agent Frameworks"}, RelatedSourceURLs: []string{"https://source/frameworks"},
	}
	incoming := OSSInsightRepositoryDiscovery{
		Collection: "MCP Servers", Repository: "owner/shared", ReviewTrack: "controlled execution", Priority: 72,
		RelatedCollections: []string{"MCP Servers"}, RelatedSourceURLs: []string{"https://source/mcp"},
	}

	merged := mergeDiscovery(existing, incoming)
	if merged.Collection != "MCP Servers" || merged.ReviewTrack != "controlled execution" || merged.Priority != 72 {
		t.Fatalf("stronger review track must become primary: %#v", merged)
	}
	if len(merged.RelatedCollections) != 2 || len(merged.RelatedSourceURLs) != 2 {
		t.Fatalf("all discovery provenance must be preserved: %#v", merged)
	}
}
