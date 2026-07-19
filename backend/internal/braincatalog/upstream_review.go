package braincatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxGitHubMetadataBytes int64 = 256 << 10

// UpstreamReviewer rechecks a fixed catalog entry. Implementations must not
// resolve arbitrary user-provided URLs or fetch source archives.
type UpstreamReviewer interface {
	Review(Entry) (UpstreamReview, error)
}

type githubUpstreamReviewer struct {
	client *http.Client
	now    func() time.Time
}

func NewUpstreamReviewer(client *http.Client) UpstreamReviewer {
	if client == nil {
		client = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &githubUpstreamReviewer{client: client, now: time.Now}
}

func (r *githubUpstreamReviewer) Review(entry Entry) (UpstreamReview, error) {
	review := UpstreamReview{
		ID: entry.ID, Name: entry.Name, UpstreamURL: entry.UpstreamURL,
		CheckedAt: r.now().UTC().Format(time.RFC3339), Disposition: entry.Status,
	}
	owner, repo, err := githubRepositoryPath(entry.UpstreamURL)
	if err != nil {
		return review, err
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+owner+"/"+repo, nil)
	if err != nil {
		return review, fmt.Errorf("could not prepare upstream metadata request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "HAI-BrainCatalog/1.0")
	resp, err := r.client.Do(req)
	if err != nil {
		return review, fmt.Errorf("upstream metadata request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		review.Message = "GitHub did not find the configured upstream repository. HAI has not changed its catalog disposition."
		return review, nil
	}
	if resp.StatusCode != http.StatusOK {
		return review, fmt.Errorf("upstream metadata request returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Archived      bool   `json:"archived"`
		DefaultBranch string `json:"default_branch"`
		PushedAt      string `json:"pushed_at"`
		License       *struct {
			SPDXID string `json:"spdx_id"`
		} `json:"license"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGitHubMetadataBytes)).Decode(&payload); err != nil {
		return review, fmt.Errorf("upstream metadata response was invalid")
	}
	review.Available = true
	review.Archived = payload.Archived
	review.DefaultBranch = strings.TrimSpace(payload.DefaultBranch)
	review.PushedAt = strings.TrimSpace(payload.PushedAt)
	if payload.License != nil {
		review.License = strings.TrimSpace(payload.License.SPDXID)
	}
	if review.License == "" {
		review.License = "not reported"
	}
	if review.Archived {
		review.Message = "GitHub reports this upstream as archived. HAI has not changed its catalog disposition; review the adoption decision before any further work."
	} else {
		review.Message = "GitHub metadata was retrieved. This recheck does not install, enable, approve, or execute the project."
	}
	return review, nil
}

func githubRepositoryPath(rawURL string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("catalog upstream URL must be a plain HTTPS GitHub repository URL")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("catalog upstream URL must identify exactly one GitHub repository")
	}
	return parts[0], parts[1], nil
}
