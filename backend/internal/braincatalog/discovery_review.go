package braincatalog

import (
	"fmt"
	"strings"
	"unicode"
)

func discoveryByRepository(report OSSInsightRepositoryDiscoveryReport, repository string) (OSSInsightRepositoryDiscovery, bool) {
	wanted := strings.TrimSpace(repository)
	for _, discovery := range report.Discoveries {
		if discovery.Repository == wanted {
			return discovery, true
		}
	}
	return OSSInsightRepositoryDiscovery{}, false
}

func entryForDiscovery(discovery OSSInsightRepositoryDiscovery) (Entry, error) {
	repository := strings.TrimSpace(discovery.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || !isGitHubRepositorySegment(parts[0]) || !isGitHubRepositorySegment(parts[1]) {
		return Entry{}, fmt.Errorf("discovery repository must be a plain owner/repository path")
	}
	return Entry{
		ID:               "ossinsight-" + strings.ToLower(parts[0]) + "-" + strings.ToLower(parts[1]),
		Name:             repository,
		UpstreamURL:      "https://github.com/" + parts[0] + "/" + parts[1],
		SourceCatalogURL: discovery.SourceURL,
		SourceCollection: discovery.Collection,
		Status:           StatusCandidate,
		Category:         "OSS Insight discovery",
		IntegrationMode:  "manual review only",
		Activation:       "Verify metadata, then create a manual adapter review. No install or runtime activation is permitted.",
		Rationale:        discovery.Rationale,
	}, nil
}

func isGitHubRepositorySegment(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
