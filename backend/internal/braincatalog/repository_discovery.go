package braincatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxOSSInsightRepositoryBytes = 128 << 10
	maxOSSInsightDiscoveries     = 120
	ossInsightDiscoveryCacheTTL  = 5 * time.Minute
)

// OSSInsightRepositoryScout reads repository names only from collections HAI
// has already classified as operational candidates. It cannot install, run, or
// add a project to HAI's catalog.
type OSSInsightRepositoryScout interface {
	DiscoverRepositories() (OSSInsightRepositoryDiscoveryReport, error)
}

type OSSInsightRepositoryDiscovery struct {
	Collection string `json:"collection"`
	Repository string `json:"repository"`
	SourceURL  string `json:"sourceUrl"`
	Rationale  string `json:"rationale"`
}

type OSSInsightRepositoryDiscoveryReport struct {
	CheckedAt              string                          `json:"checkedAt"`
	SourceURL              string                          `json:"sourceUrl"`
	Available              bool                            `json:"available"`
	Cached                 bool                            `json:"cached"`
	CollectionsScreened    int                             `json:"collectionsScreened"`
	CandidateCollections   int                             `json:"candidateCollections"`
	CollectionsChecked     int                             `json:"collectionsChecked"`
	RepositoriesChecked    int                             `json:"repositoriesChecked"`
	KnownProfileHits       int                             `json:"knownProfileHits"`
	Discoveries            []OSSInsightRepositoryDiscovery `json:"discoveries,omitempty"`
	MissingCollections     []string                        `json:"missingCollections,omitempty"`
	UnavailableCollections []string                        `json:"unavailableCollections,omitempty"`
	DiscoveriesTruncated   bool                            `json:"discoveriesTruncated"`
	Message                string                          `json:"message"`
}

type ossInsightRepositoryScout struct {
	client       *http.Client
	now          func() time.Time
	cacheMu      sync.Mutex
	cachedReport OSSInsightRepositoryDiscoveryReport
	cacheExpires time.Time
}

func NewOSSInsightRepositoryScout(client *http.Client) OSSInsightRepositoryScout {
	if client == nil {
		client = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &ossInsightRepositoryScout{client: client, now: time.Now}
}

func (s *ossInsightRepositoryScout) DiscoverRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cacheExpires.After(s.now()) {
		report := s.cachedReport
		report.Cached = true
		return report, nil
	}
	report, err := s.discoverRepositories()
	if err == nil {
		s.cachedReport = report
		s.cacheExpires = s.now().Add(ossInsightDiscoveryCacheTTL)
	}
	return report, err
}

func (s *ossInsightRepositoryScout) discoverRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	report := OSSInsightRepositoryDiscoveryReport{
		CheckedAt:           s.now().UTC().Format(time.RFC3339),
		SourceURL:           ossInsightCollectionsURL,
		CollectionsScreened: len(CollectionScreenings()),
	}
	live, err := s.liveCollections()
	if err != nil {
		return report, err
	}
	candidates := candidateCollectionDecisions()
	report.CandidateCollections = len(candidates)
	known := catalogRepositories()
	liveNames := map[string]bool{}

	for _, collection := range live {
		liveNames[collection.Name] = true
		decision, ok := candidates[collection.Name]
		if !ok {
			continue
		}
		if !isDecimalID(collection.ID) {
			report.UnavailableCollections = append(report.UnavailableCollections, collection.Name)
			continue
		}
		repositories, err := s.collectionRepositories(collection.ID)
		if err != nil {
			report.UnavailableCollections = append(report.UnavailableCollections, collection.Name)
			continue
		}
		report.CollectionsChecked++
		report.RepositoriesChecked += len(repositories)
		for _, repository := range repositories {
			repository = strings.TrimSpace(repository)
			if repository == "" {
				continue
			}
			if known[strings.ToLower(repository)] {
				report.KnownProfileHits++
				continue
			}
			if len(report.Discoveries) >= maxOSSInsightDiscoveries {
				report.DiscoveriesTruncated = true
				continue
			}
			report.Discoveries = append(report.Discoveries, OSSInsightRepositoryDiscovery{
				Collection: collection.Name,
				Repository: repository,
				SourceURL:  ossInsightCollectionRepositoriesURL(collection.ID),
				Rationale:  decision.rationale,
			})
		}
	}
	for name := range candidates {
		if !liveNames[name] {
			report.MissingCollections = append(report.MissingCollections, name)
		}
	}
	sort.Strings(report.MissingCollections)
	sort.Strings(report.UnavailableCollections)
	report.Available = report.CollectionsChecked > 0
	if !report.Available {
		return report, fmt.Errorf("OSS Insight candidate collection repositories were unavailable")
	}
	report.Message = "HAI checked the full collection index and read repository names only from pre-screened candidate categories. Discoveries remain unreviewed: this scan did not add catalog entries, install software, create credentials, or execute a project."
	return report, nil
}

type ossInsightCollection struct {
	ID   string
	Name string
}

func (s *ossInsightRepositoryScout) liveCollections() ([]ossInsightCollection, error) {
	req, err := http.NewRequest(http.MethodGet, ossInsightCollectionsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not prepare OSS Insight collection request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ossInsightCollectionReviewAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OSS Insight collection request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSS Insight collection request returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			Rows []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOSSInsightCollectionsBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("OSS Insight collection response was invalid")
	}
	result := make([]ossInsightCollection, 0, len(payload.Data.Rows))
	for _, row := range payload.Data.Rows {
		name := strings.TrimSpace(row.Name)
		id := strings.TrimSpace(row.ID)
		if name != "" && id != "" {
			result = append(result, ossInsightCollection{ID: id, Name: name})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("OSS Insight collection response contained no collections")
	}
	return result, nil
}

func (s *ossInsightRepositoryScout) collectionRepositories(id string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, ossInsightCollectionRepositoriesURL(id), nil)
	if err != nil {
		return nil, fmt.Errorf("could not prepare OSS Insight repository request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ossInsightCollectionReviewAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OSS Insight repository request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSS Insight repository request returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			Rows []struct {
				Repository string `json:"repo_name"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOSSInsightRepositoryBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("OSS Insight repository response was invalid")
	}
	repositories := make([]string, 0, len(payload.Data.Rows))
	for _, row := range payload.Data.Rows {
		if name := strings.TrimSpace(row.Repository); name != "" {
			repositories = append(repositories, name)
		}
	}
	return repositories, nil
}

func candidateCollectionDecisions() map[string]collectionDecision {
	decisions := map[string]collectionDecision{}
	for _, screening := range CollectionScreenings() {
		if screening.Disposition == CollectionCandidate {
			decisions[screening.Collection] = collectionDecision{rationale: screening.Rationale}
		}
	}
	return decisions
}

func catalogRepositories() map[string]bool {
	known := map[string]bool{}
	for _, entry := range Entries() {
		parts := strings.Split(strings.TrimSuffix(entry.UpstreamURL, "/"), "/")
		if len(parts) >= 2 {
			known[strings.ToLower(parts[len(parts)-2]+"/"+parts[len(parts)-1])] = true
		}
	}
	return known
}

func ossInsightCollectionRepositoriesURL(id string) string {
	return "https://api.ossinsight.io/v1/collections/" + id + "/repos/"
}

func isDecimalID(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
