package googleoauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

// DefaultGmailBaseURL is the Gmail REST API root; tests override it.
const DefaultGmailBaseURL = "https://gmail.googleapis.com/gmail/v1"

const (
	maxGmailBodyBytes                   = 512 << 10
	maxGmailAttachmentBytes             = 512 << 10
	maxGmailAttachmentRecords           = 20
	maxGmailAttachmentContentTotalBytes = 1 << 20
	maxGmailResponseBytes               = 8 << 20
)

var ErrHistoryCursorExpired = errors.New("gmail history cursor expired")

// GmailClient reads a mailbox over the Gmail REST API with a bearer access
// token. Message and attachment content is bounded to protect memory and token
// budgets; non-text attachments remain metadata-only.
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
	ID          string
	ThreadID    string
	HistoryID   string
	From        string
	To          string
	Subject     string
	Date        time.Time
	Snippet     string
	Body        string
	Attachments []GmailAttachment
}

type GmailAttachment struct {
	Filename string
	MimeType string
	Size     int64
	Content  string
	Fetched  bool
}

type messageListResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	NextPageToken string `json:"nextPageToken"`
}

type gmailMessagePart struct {
	PartID   string `json:"partId"`
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		AttachmentID string `json:"attachmentId"`
		Size         int64  `json:"size"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []gmailMessagePart `json:"parts"`
}

type messageResponse struct {
	ID           string           `json:"id"`
	ThreadID     string           `json:"threadId"`
	HistoryID    string           `json:"historyId"`
	Snippet      string           `json:"snippet"`
	InternalDate string           `json:"internalDate"`
	Payload      gmailMessagePart `json:"payload"`
}

type GmailMessageIDPage struct {
	IDs           []string
	NextPageToken string
}

type GmailHistoryPage struct {
	MessageIDs    []string
	NextPageToken string
	HistoryID     string
}

func (g GmailClient) GetProfileHistoryID(ctx context.Context) (string, error) {
	var response struct {
		HistoryID string `json:"historyId"`
	}
	if err := g.getJSON(ctx, "/users/me/profile", &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.HistoryID) == "" {
		return "", fmt.Errorf("gmail returned no profile historyId")
	}
	return response.HistoryID, nil
}

func (g GmailClient) ListMessageIDsPage(ctx context.Context, maxResults int, query, pageToken string) (GmailMessageIDPage, error) {
	if maxResults <= 0 || maxResults > 500 {
		maxResults = 50
	}
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(maxResults))
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		q.Set("q", trimmed)
	}
	if strings.TrimSpace(pageToken) != "" {
		q.Set("pageToken", pageToken)
	}
	var parsed messageListResponse
	if err := g.getJSON(ctx, "/users/me/messages?"+q.Encode(), &parsed); err != nil {
		return GmailMessageIDPage{}, err
	}
	page := GmailMessageIDPage{NextPageToken: parsed.NextPageToken, IDs: make([]string, 0, len(parsed.Messages))}
	for _, message := range parsed.Messages {
		if strings.TrimSpace(message.ID) != "" {
			page.IDs = append(page.IDs, message.ID)
		}
	}
	return page, nil
}

func (g GmailClient) ListHistoryPage(ctx context.Context, startHistoryID, pageToken string, maxResults int) (GmailHistoryPage, error) {
	if strings.TrimSpace(startHistoryID) == "" {
		return GmailHistoryPage{}, fmt.Errorf("gmail startHistoryId is required")
	}
	if maxResults <= 0 || maxResults > 500 {
		maxResults = 100
	}
	q := url.Values{}
	q.Set("startHistoryId", startHistoryID)
	q.Set("historyTypes", "messageAdded")
	q.Set("maxResults", strconv.Itoa(maxResults))
	if strings.TrimSpace(pageToken) != "" {
		q.Set("pageToken", pageToken)
	}
	var response struct {
		History []struct {
			MessagesAdded []struct {
				Message struct {
					ID string `json:"id"`
				} `json:"message"`
			} `json:"messagesAdded"`
		} `json:"history"`
		NextPageToken string `json:"nextPageToken"`
		HistoryID     string `json:"historyId"`
	}
	if err := g.getJSON(ctx, "/users/me/history?"+q.Encode(), &response); err != nil {
		var httpErr *gmailHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return GmailHistoryPage{}, ErrHistoryCursorExpired
		}
		return GmailHistoryPage{}, err
	}
	seen := map[string]bool{}
	page := GmailHistoryPage{NextPageToken: response.NextPageToken, HistoryID: response.HistoryID}
	for _, history := range response.History {
		for _, added := range history.MessagesAdded {
			id := strings.TrimSpace(added.Message.ID)
			if id != "" && !seen[id] {
				seen[id] = true
				page.MessageIDs = append(page.MessageIDs, id)
			}
		}
	}
	return page, nil
}

// ListRecentMessageIDs returns up to maxResults recent message IDs, newest
// first (Gmail's default order). A non-empty query is passed straight to
// Gmail's `q` search parameter, which is how incremental sync narrows the fetch
// to mail that arrived after the last cursor (e.g. "after:1750000000").
func (g GmailClient) ListRecentMessageIDs(ctx context.Context, maxResults int, query string) ([]string, error) {
	page, err := g.ListMessageIDsPage(ctx, maxResults, query, "")
	if err != nil {
		return nil, err
	}
	return page.IDs, nil
}

// GetMessageMetadata fetches one message's headers, bounded body text, and
// bounded textual attachments. The name is retained for API compatibility.
func (g GmailClient) GetMessageMetadata(ctx context.Context, id string) (GmailMessage, error) {
	path := "/users/me/messages/" + url.PathEscape(id) +
		"?format=full"
	var parsed messageResponse
	if err := g.getJSON(ctx, path, &parsed); err != nil {
		return GmailMessage{}, err
	}
	msg := GmailMessage{ID: parsed.ID, ThreadID: parsed.ThreadID, HistoryID: parsed.HistoryID, Snippet: parsed.Snippet}
	for _, h := range parsed.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			msg.From = h.Value
		case "to":
			msg.To = h.Value
		case "subject":
			msg.Subject = h.Value
		}
	}
	if ms, err := strconv.ParseInt(parsed.InternalDate, 10, 64); err == nil && ms > 0 {
		msg.Date = time.UnixMilli(ms).UTC()
	}
	plain, htmlBody := []string{}, []string{}
	attachments := []GmailAttachment{}
	attachmentContentBudget := maxGmailAttachmentContentTotalBytes
	g.collectPart(ctx, parsed.ID, parsed.Payload, &plain, &htmlBody, &attachments, &attachmentContentBudget)
	msg.Body = strings.TrimSpace(strings.Join(plain, "\n\n"))
	if msg.Body == "" {
		msg.Body = strings.TrimSpace(strings.Join(htmlBody, "\n\n"))
	}
	msg.Body = truncateText(msg.Body, maxGmailBodyBytes)
	msg.Attachments = attachments
	return msg, nil
}

func (g GmailClient) FetchMessageIDs(ctx context.Context, ids []string) []GmailMessage {
	out := make([]GmailMessage, 0, len(ids))
	for _, id := range ids {
		message, err := g.GetMessageMetadata(ctx, id)
		if err == nil {
			out = append(out, message)
		}
	}
	return out
}

// FetchRecent lists and then hydrates up to maxResults recent messages matching
// query (empty fetches the newest overall). A single message that fails to fetch
// is skipped rather than failing the whole sync, so one malformed item does not
// block ingestion.
func (g GmailClient) FetchRecent(ctx context.Context, maxResults int, query string) ([]GmailMessage, error) {
	ids, err := g.ListRecentMessageIDs(ctx, maxResults, query)
	if err != nil {
		return nil, err
	}
	return g.FetchMessageIDs(ctx, ids), nil
}

func (g GmailClient) collectPart(ctx context.Context, messageID string, part gmailMessagePart, plain, htmlBody *[]string, attachments *[]GmailAttachment, attachmentContentBudget *int) {
	if strings.TrimSpace(part.Filename) != "" {
		if len(*attachments) >= maxGmailAttachmentRecords {
			return
		}
		attachment := GmailAttachment{Filename: part.Filename, MimeType: part.MimeType, Size: part.Body.Size}
		if gmailTextMime(part.MimeType) && part.Body.Size <= maxGmailAttachmentBytes && *attachmentContentBudget > 0 && part.Body.Size <= int64(*attachmentContentBudget) {
			data := part.Body.Data
			if data == "" && part.Body.AttachmentID != "" {
				data, _ = g.fetchAttachmentData(ctx, messageID, part.Body.AttachmentID)
			}
			if decoded, err := decodeGmailData(data); err == nil && len(decoded) <= maxGmailAttachmentBytes && len(decoded) <= *attachmentContentBudget {
				attachment.Content = strings.TrimSpace(string(decoded))
				attachment.Fetched = attachment.Content != ""
				*attachmentContentBudget -= len(decoded)
			}
		}
		*attachments = append(*attachments, attachment)
		return
	}
	if decoded, err := decodeGmailData(part.Body.Data); err == nil && len(decoded) > 0 {
		text := strings.TrimSpace(string(decoded))
		switch strings.ToLower(strings.TrimSpace(strings.SplitN(part.MimeType, ";", 2)[0])) {
		case "text/plain":
			*plain = append(*plain, text)
		case "text/html":
			*htmlBody = append(*htmlBody, htmlToText(text))
		}
	}
	for _, child := range part.Parts {
		g.collectPart(ctx, messageID, child, plain, htmlBody, attachments, attachmentContentBudget)
	}
}

func (g GmailClient) fetchAttachmentData(ctx context.Context, messageID, attachmentID string) (string, error) {
	var response struct {
		Data string `json:"data"`
		Size int64  `json:"size"`
	}
	path := "/users/me/messages/" + url.PathEscape(messageID) + "/attachments/" + url.PathEscape(attachmentID)
	if err := g.getJSON(ctx, path, &response); err != nil {
		return "", err
	}
	if response.Size > maxGmailAttachmentBytes {
		return "", fmt.Errorf("gmail attachment exceeds the safety limit")
	}
	return response.Data, nil
}

func decodeGmailData(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return base64.RawURLEncoding.DecodeString(value)
}

func gmailTextMime(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	return strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" || mimeType == "application/xml"
}

func htmlToText(value string) string {
	tokenizer := xhtml.NewTokenizer(strings.NewReader(value))
	var output bytes.Buffer
	skipDepth := 0
	for {
		tokenType := tokenizer.Next()
		if tokenType == xhtml.ErrorToken {
			break
		}
		token := tokenizer.Token()
		switch tokenType {
		case xhtml.StartTagToken:
			if token.Data == "script" || token.Data == "style" {
				skipDepth++
			}
		case xhtml.EndTagToken:
			if (token.Data == "script" || token.Data == "style") && skipDepth > 0 {
				skipDepth--
			}
			if skipDepth == 0 && (token.Data == "p" || token.Data == "div" || token.Data == "br" || token.Data == "li") {
				output.WriteByte('\n')
			}
		case xhtml.TextToken:
			if skipDepth == 0 {
				output.WriteString(token.Data)
				output.WriteByte(' ')
			}
		}
	}
	return strings.Join(strings.Fields(output.String()), " ")
}

func truncateText(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "..."
}

type gmailHTTPError struct {
	StatusCode int
	Body       string
}

func (e *gmailHTTPError) Error() string {
	return fmt.Sprintf("gmail returned HTTP %d: %s", e.StatusCode, e.Body)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGmailResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxGmailResponseBytes {
		return fmt.Errorf("gmail response exceeded the %d byte safety limit", maxGmailResponseBytes)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gmail returned 401: access token is invalid or expired")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &gmailHTTPError{StatusCode: resp.StatusCode, Body: compact(body)}
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
