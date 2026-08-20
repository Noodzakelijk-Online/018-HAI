package source

// The ShareT adapter is deliberately read-only. Its base URL and connector
// token come only from the operator environment, never from an API request or
// connected-source record. It calls the fixed status and paginated share-list
// endpoints and cannot create, update, revoke, or comment on a share.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/models"
)

const (
	shareTConnectorKey    = "sharet"
	shareTCursorPrefix    = "sharet-updated-at:"
	shareTPageSize        = 100
	shareTDefaultMaxItems = 1000
	shareTHardMaxItems    = 5000
	shareTMaxResponse     = 2 << 20
)

type shareTConfig struct {
	baseURL  *url.URL
	token    string
	maxItems int
	client   *http.Client
}

type shareTStatusResponse struct {
	Success      bool `json:"success"`
	Capabilities struct {
		ShareRead bool `json:"shareRead"`
	} `json:"capabilities"`
}

type shareTPermissions struct {
	CanView       bool `json:"canView"`
	CanComment    bool `json:"canComment"`
	CanUpload     bool `json:"canUpload"`
	CanDownload   bool `json:"canDownload"`
	CanSetDueDate bool `json:"canSetDueDate"`
}

type shareTShare struct {
	ShareID       string            `json:"shareId"`
	CardID        string            `json:"cardId"`
	CardName      string            `json:"cardName"`
	BoardID       string            `json:"boardId"`
	BoardName     string            `json:"boardName"`
	Permissions   shareTPermissions `json:"permissions"`
	AllowedEmails []string          `json:"allowedEmails"`
	ExpiresAt     *string           `json:"expiresAt"`
	AccessCount   int               `json:"accessCount"`
	IsActive      bool              `json:"isActive"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
	HasPassword   bool              `json:"hasPassword"`
	HasGuestRelay bool              `json:"hasGuestRelay"`
}

type shareTListResponse struct {
	Success    bool          `json:"success"`
	Data       []shareTShare `json:"data"`
	Pagination struct {
		Total int `json:"total"`
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Pages int `json:"pages"`
	} `json:"pagination"`
}

func shareTConfigFromEnv() (shareTConfig, error) {
	if !envBool("HAI_SHARET_ENABLED") {
		return shareTConfig{}, fmt.Errorf("HAI_SHARET_ENABLED is false")
	}
	baseURL, err := parseShareTBaseURL(os.Getenv("HAI_SHARET_BASE_URL"))
	if err != nil {
		return shareTConfig{}, err
	}
	token := strings.TrimSpace(os.Getenv("HAI_SHARET_CONNECTOR_TOKEN"))
	if token == "" {
		return shareTConfig{}, fmt.Errorf("HAI_SHARET_CONNECTOR_TOKEN is not set")
	}
	if !strings.HasPrefix(token, "sharet_pat_") {
		return shareTConfig{}, fmt.Errorf("HAI_SHARET_CONNECTOR_TOKEN is not a ShareT connector token")
	}
	return shareTConfig{
		baseURL:  baseURL,
		token:    token,
		maxItems: boundedShareTLimit(os.Getenv("HAI_SHARET_SYNC_LIMIT")),
		client: &http.Client{
			Transport:     &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext},
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func parseShareTBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("HAI_SHARET_BASE_URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("HAI_SHARET_BASE_URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("HAI_SHARET_BASE_URL must not contain credentials, a path, query, or fragment")
	}
	if u.Scheme == "http" && !isShareTLocalHost(u.Hostname()) {
		return nil, fmt.Errorf("HAI_SHARET_BASE_URL must use HTTPS unless it targets a local ShareT host")
	}
	u.Path = ""
	return u, nil
}

func isShareTLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "host.docker.internal" || host == "sharet" {
		return true
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return address.IsLoopback() || address.IsPrivate()
}

func fetchShareTSource(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	config, err := shareTConfigFromEnv()
	if err != nil {
		return nil, "", err
	}
	if err := config.verifyReadAccess(ctx); err != nil {
		return nil, "", err
	}

	items := make([]ImportItem, 0, min(config.maxItems, shareTPageSize))
	seenShareIDs := map[string]bool{}
	latest := time.Time{}
	page := 1
	pages := 1
	total := 0
	for page <= pages {
		response, err := config.readSharePage(ctx, page)
		if err != nil {
			return nil, "", err
		}
		if page == 1 {
			total = response.Pagination.Total
			if total < 0 {
				return nil, "", fmt.Errorf("ShareT returned an invalid negative link count")
			}
			if response.Pagination.Total > config.maxItems {
				return nil, "", fmt.Errorf("ShareT reports %d links, exceeding configured completeness limit %d; raise HAI_SHARET_SYNC_LIMIT up to %d", response.Pagination.Total, config.maxItems, shareTHardMaxItems)
			}
			pages = response.Pagination.Pages
			if pages < 1 {
				pages = 1
			}
			expectedPages := max(1, (total+shareTPageSize-1)/shareTPageSize)
			if pages != expectedPages || response.Pagination.Limit != shareTPageSize {
				return nil, "", fmt.Errorf("ShareT returned inconsistent pagination metadata")
			}
			if pages > (config.maxItems+shareTPageSize-1)/shareTPageSize {
				return nil, "", fmt.Errorf("ShareT pagination exceeds configured completeness limit %d", config.maxItems)
			}
		}
		if response.Pagination.Page != page || response.Pagination.Limit != shareTPageSize || response.Pagination.Pages != pages || response.Pagination.Total != total {
			return nil, "", fmt.Errorf("ShareT pagination changed during synchronization; retry after the inventory stops changing")
		}
		for _, share := range response.Data {
			shareID := strings.TrimSpace(share.ShareID)
			if seenShareIDs[shareID] {
				return nil, "", fmt.Errorf("ShareT returned duplicate shareId across pages")
			}
			item, updatedAt, err := config.importItem(share, source.DefaultProjectKey)
			if err != nil {
				return nil, "", err
			}
			items = append(items, item)
			seenShareIDs[shareID] = true
			if len(items) > config.maxItems {
				return nil, "", fmt.Errorf("ShareT response exceeds configured completeness limit %d", config.maxItems)
			}
			if updatedAt.After(latest) {
				latest = updatedAt
			}
		}
		page++
	}
	if len(items) != total {
		return nil, "", fmt.Errorf("ShareT returned %d links while pagination promised %d; retry after the inventory stops changing", len(items), total)
	}
	if latest.IsZero() {
		return items, source.Cursor, nil
	}
	return items, shareTCursorPrefix + latest.UTC().Format(time.RFC3339), nil
}

func (config shareTConfig) verifyReadAccess(ctx context.Context) error {
	var response shareTStatusResponse
	if err := config.getJSON(ctx, "/api/connector/status", &response); err != nil {
		return fmt.Errorf("verify ShareT connector: %w", err)
	}
	if !response.Success || !response.Capabilities.ShareRead {
		return fmt.Errorf("ShareT connector token does not provide read access")
	}
	return nil
}

func (config shareTConfig) readSharePage(ctx context.Context, page int) (shareTListResponse, error) {
	var response shareTListResponse
	path := "/api/connector/shares?page=" + strconv.Itoa(page) + "&limit=" + strconv.Itoa(shareTPageSize)
	if err := config.getJSON(ctx, path, &response); err != nil {
		return shareTListResponse{}, fmt.Errorf("read ShareT links page %d: %w", page, err)
	}
	if !response.Success {
		return shareTListResponse{}, fmt.Errorf("ShareT links page %d was not successful", page)
	}
	return response, nil
}

func (config shareTConfig) getJSON(ctx context.Context, path string, destination any) error {
	relative, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(relative.Path, "/api/connector/") {
		return fmt.Errorf("invalid fixed ShareT endpoint")
	}
	endpoint := *config.baseURL
	endpoint.Path = relative.Path
	endpoint.RawQuery = relative.RawQuery
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-ShareT-ReadOnly/1.0")
	response, err := config.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, shareTMaxResponse+1))
	if err != nil {
		return err
	}
	if len(data) > shareTMaxResponse {
		return fmt.Errorf("response exceeds %d bytes", shareTMaxResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (config shareTConfig) importItem(share shareTShare, projectKey string) (ImportItem, time.Time, error) {
	shareID := strings.TrimSpace(share.ShareID)
	if shareID == "" {
		return ImportItem{}, time.Time{}, fmt.Errorf("ShareT returned a link without a shareId")
	}
	if strings.ContainsAny(shareID, "/?#") {
		return ImportItem{}, time.Time{}, fmt.Errorf("ShareT returned an invalid shareId")
	}
	updatedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(share.UpdatedAt))
	createdAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(share.CreatedAt))
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	title := boundedShareTText(share.CardName, 500)
	if title == "" {
		title = "ShareT link " + shareID
	}
	permissions := []string{}
	if share.Permissions.CanView {
		permissions = append(permissions, "view")
	}
	if share.Permissions.CanComment {
		permissions = append(permissions, "comment")
	}
	if share.Permissions.CanUpload {
		permissions = append(permissions, "upload")
	}
	if share.Permissions.CanDownload {
		permissions = append(permissions, "download")
	}
	if share.Permissions.CanSetDueDate {
		permissions = append(permissions, "set_due_date")
	}
	content := []string{
		"ShareT link: " + title,
		"board: " + boundedShareTText(share.BoardName, 500),
		"card id: " + boundedShareTText(share.CardID, 128),
		"active: " + strconv.FormatBool(share.IsActive),
		"permissions: " + strings.Join(permissions, ","),
		"recipient allowlist entries: " + strconv.Itoa(len(share.AllowedEmails)),
		"password protected: " + strconv.FormatBool(share.HasPassword),
		"guest relay configured: " + strconv.FormatBool(share.HasGuestRelay),
		"views: " + strconv.Itoa(max(0, share.AccessCount)),
	}
	if !createdAt.IsZero() {
		content = append(content, "created: "+createdAt.UTC().Format(time.RFC3339))
	}
	if !updatedAt.IsZero() {
		content = append(content, "updated: "+updatedAt.UTC().Format(time.RFC3339))
	}
	if share.ExpiresAt != nil {
		if expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*share.ExpiresAt)); err == nil {
			content = append(content, "expires: "+expiresAt.UTC().Format(time.RFC3339))
		}
	}
	uri := config.baseURL.JoinPath("shared", shareID)
	return ImportItem{
		ExternalID: "sharet:" + shareID,
		Title:      "ShareT: " + title,
		Content:    strings.Join(content, "\n"),
		SourceURI:  uri.String(),
		ItemType:   "sharet_link",
		ProjectKey: projectKey,
		Metadata:   "connector=sharet;read_only=true;write_back=disabled;participant_emails=excluded;board_id=" + boundedShareTText(share.BoardID, 128),
	}, updatedAt, nil
}

func boundedShareTLimit(raw string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return shareTDefaultMaxItems
	}
	if parsed > shareTHardMaxItems {
		return shareTHardMaxItems
	}
	return parsed
}

func boundedShareTText(value string, limit int) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
