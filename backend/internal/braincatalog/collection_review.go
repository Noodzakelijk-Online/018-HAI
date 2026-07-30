package braincatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	ossInsightCollectionsURL        = "https://api.ossinsight.io/v1/collections/"
	maxOSSInsightCollectionsBytes   = 256 << 10
	ossInsightCollectionReviewAgent = "HAI-BrainCatalog/1.0"
)

// OSSInsightCollectionReviewer checks the one fixed public collection list.
// It deliberately does not accept a user-provided URL or fetch repository
// code: this endpoint exists only to make catalog drift visible to an admin.
type OSSInsightCollectionReviewer interface {
	ReviewCollections() (OSSInsightCollectionReview, error)
}

// OSSInsightCollectionReview reports source freshness without turning source
// discovery into an adoption or activation decision.
type OSSInsightCollectionReview struct {
	CheckedAt       string   `json:"checkedAt"`
	SourceURL       string   `json:"sourceUrl"`
	Available       bool     `json:"available"`
	ExpectedTotal   int      `json:"expectedTotal"`
	CurrentTotal    int      `json:"currentTotal"`
	NewCollections  []string `json:"newCollections,omitempty"`
	MissingExpected []string `json:"missingExpected,omitempty"`
	Message         string   `json:"message"`
}

type ossInsightCollectionReviewer struct {
	client *http.Client
	now    func() time.Time
}

func NewOSSInsightCollectionReviewer(client *http.Client) OSSInsightCollectionReviewer {
	if client == nil {
		client = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &ossInsightCollectionReviewer{client: client, now: time.Now}
}

func (r *ossInsightCollectionReviewer) ReviewCollections() (OSSInsightCollectionReview, error) {
	expected := expectedOSSInsightCollections()
	review := OSSInsightCollectionReview{
		CheckedAt: r.now().UTC().Format(time.RFC3339), SourceURL: ossInsightCollectionsURL, ExpectedTotal: len(expected),
	}
	req, err := http.NewRequest(http.MethodGet, ossInsightCollectionsURL, nil)
	if err != nil {
		return review, fmt.Errorf("could not prepare OSS Insight collection request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ossInsightCollectionReviewAgent)
	resp, err := r.client.Do(req)
	if err != nil {
		return review, fmt.Errorf("OSS Insight collection request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return review, fmt.Errorf("OSS Insight collection request returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data struct {
			Rows []struct {
				Name string `json:"name"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOSSInsightCollectionsBytes)).Decode(&payload); err != nil {
		return review, fmt.Errorf("OSS Insight collection response was invalid")
	}
	current := map[string]struct{}{}
	for _, row := range payload.Data.Rows {
		if name := strings.TrimSpace(row.Name); name != "" {
			current[name] = struct{}{}
		}
	}
	if len(current) == 0 {
		return review, fmt.Errorf("OSS Insight collection response contained no collection names")
	}

	review.Available = true
	review.CurrentTotal = len(current)
	for name := range current {
		if _, ok := expected[name]; !ok {
			review.NewCollections = append(review.NewCollections, name)
		}
	}
	for name := range expected {
		if _, ok := current[name]; !ok {
			review.MissingExpected = append(review.MissingExpected, name)
		}
	}
	sort.Strings(review.NewCollections)
	sort.Strings(review.MissingExpected)
	if len(review.NewCollections) == 0 && len(review.MissingExpected) == 0 {
		review.Message = "The OSS Insight collection list matches HAI's recorded snapshot. This check did not install, enable, or approve any project."
	} else {
		review.Message = "OSS Insight collection drift was detected. Review the changed categories before changing HAI's catalog; this check did not install, enable, approve, or execute any project."
	}
	return review, nil
}

func expectedOSSInsightCollections() map[string]struct{} {
	expected := map[string]struct{}{}
	for _, entry := range CollectionScreenings() {
		expected[entry.Collection] = struct{}{}
	}
	return expected
}
