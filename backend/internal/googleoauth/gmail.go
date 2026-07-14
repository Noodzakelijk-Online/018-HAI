package googleoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultGmailBaseURL is the Gmail REST API root; tests override it.
const DefaultGmailBaseURL = "https://gmail.googleapis.com/gmail/v1"

// GmailClient reads a mailbox over the Gmail REST API with a bearer access
// token. It fetches metadata only (headers + snippet), never bodies or
// attachments, matching the read-only, minimal-footprint intent of ingestion.
type GmailClient struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
}

func (g GmailClient) baseURL() string {
	if strings.TrimSpace(g.BaseURL) != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return DefaultGmailBaseURL
}

func (g GmailClient) httpClient() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// GmailMessage is the metadata this connector ingests for one message.
type GmailMessage struct {
	ID      string
	From    string
	Subject string
	Date    time.Time
	Snippet string
}

type messageListResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	NextPageToken string `json:"nextPageToken"`
}

type messageResponse struct {
	ID           string `json:"id"`
	Snippet      string `json:"snippet"`
	InternalDate string `json:"internalDate"`
	Payload      struct {
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	} `json:"payload"`
}

// ListRecentMessageIDs returns up to maxResults recent message IDs, newest
// first (Gmail's default order).
func (g GmailClient) ListRecentMessageIDs(ctx context.Context, maxResults int) ([]string, error) {
	if maxResults <= 0 {
		maxResults = 25
	}
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(maxResults))
	var parsed messageListResponse
	if err := g.getJSON(ctx, "/users/me/messages?"+q.Encode(), &parsed); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parsed.Messages))
	for _, m := range parsed.Messages {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// GetMessageMetadata fetches one message's headers and snippet.
func (g GmailClient) GetMessageMetadata(ctx context.Context, id string) (GmailMessage, error) {
	path := "/users/me/messages/" + url.PathEscape(id) +
		"?format=metadata&metadataHeaders=From&metadataHeaders=Subject&metadataHeaders=Date"
	var parsed messageResponse
	if err := g.getJSON(ctx, path, &parsed); err != nil {
		return GmailMessage{}, err
	}
	msg := GmailMessage{ID: parsed.ID, Snippet: parsed.Snippet}
	for _, h := range parsed.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			msg.From = h.Value
		case "subject":
			msg.Subject = h.Value
		}
	}
	if ms, err := strconv.ParseInt(parsed.InternalDate, 10, 64); err == nil && ms > 0 {
		msg.Date = time.UnixMilli(ms).UTC()
	}
	return msg, nil
}

// FetchRecent lists and then hydrates up to maxResults recent messages. A single
// message that fails to fetch is skipped rather than failing the whole sync, so
// one malformed item does not block ingestion.
func (g GmailClient) FetchRecent(ctx context.Context, maxResults int) ([]GmailMessage, error) {
	ids, err := g.ListRecentMessageIDs(ctx, maxResults)
	if err != nil {
		return nil, err
	}
	out := make([]GmailMessage, 0, len(ids))
	for _, id := range ids {
		msg, err := g.GetMessageMetadata(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

func (g GmailClient) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("gmail request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gmail returned 401: access token is invalid or expired")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gmail returned HTTP %d: %s", resp.StatusCode, compact(body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("gmail returned unparseable JSON: %w", err)
	}
	return nil
}

func compact(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
