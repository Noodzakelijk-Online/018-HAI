package source

// The LARO adapter is a read-only, operator-configured bridge. LARO owns the
// credential and case authorization boundary; HAI only receives minimized,
// incremental records and never writes back or stores the bearer token.

import (
	"automation-hub-backend/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	laroConnectorKey       = "laro"
	laroFeedPath           = "/api/integrations/hai/feed"
	laroDefaultLimit       = 50
	laroMaxLimit           = 100
	laroMaxResponseBytes   = 1 << 20
	laroConnectorUserAgent = "HAI-LARO-Connector/1.0"
)

type laroConfig struct {
	baseURL *url.URL
	token   string
	limit   int
}

func laroConfigFromEnv() (laroConfig, error) {
	if !envBool("HAI_LARO_ENABLED") {
		return laroConfig{}, fmt.Errorf("HAI_LARO_ENABLED is false")
	}
	baseURL, err := parseLAROBaseURL(os.Getenv("HAI_LARO_BASE_URL"))
	if err != nil {
		return laroConfig{}, err
	}
	token := strings.TrimSpace(os.Getenv("HAI_LARO_CONNECTOR_TOKEN"))
	if !strings.HasPrefix(token, "laro_hai_") || len(token) > 128 {
		return laroConfig{}, fmt.Errorf("HAI_LARO_CONNECTOR_TOKEN is missing or invalid")
	}
	return laroConfig{baseURL: baseURL, token: token, limit: boundedLAROLimit(os.Getenv("HAI_LARO_SYNC_LIMIT"))}, nil
}

func parseLAROBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("HAI_LARO_BASE_URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("HAI_LARO_BASE_URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("HAI_LARO_BASE_URL must not contain credentials, query, or fragments")
	}
	if u.Scheme == "http" && !isLAROLocalHost(u.Hostname()) {
		return nil, fmt.Errorf("HAI_LARO_BASE_URL must use HTTPS unless it targets a local LARO runtime")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

func isLAROLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "host.docker.internal" || host == "laro" || host == "laro-server" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && (address.IsLoopback() || address.IsPrivate())
}

func boundedLAROLimit(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return laroDefaultLimit
	}
	if value > laroMaxLimit {
		return laroMaxLimit
	}
	return value
}

func fetchLAROSource(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	config, err := laroConfigFromEnv()
	if err != nil {
		return nil, "", err
	}
	endpoint := *config.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + laroFeedPath
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(config.limit))
	if strings.TrimSpace(source.Cursor) != "" {
		query.Set("cursor", source.Cursor)
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("create LARO feed request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+config.token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", laroConnectorUserAgent)
	client := &http.Client{
		Timeout:   sourceHTTPTimeout(),
		Transport: sourceHTTPTransport(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("fetch LARO feed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, laroMaxResponseBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read LARO feed: %w", err)
	}
	if len(body) > laroMaxResponseBytes {
		return nil, "", fmt.Errorf("LARO feed response exceeds the %d-byte safety limit", laroMaxResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized {
			return nil, "", fmt.Errorf("LARO rejected the connector credential; create a new credential in LARO Settings")
		}
		return nil, "", fmt.Errorf("LARO feed returned HTTP %d", response.StatusCode)
	}
	var envelope jsonFeedEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(body), &envelope); err != nil {
		return nil, "", fmt.Errorf("decode LARO feed: %w", err)
	}
	if len(envelope.Items) > config.limit {
		return nil, "", fmt.Errorf("LARO feed returned %d items above the requested %d-item limit", len(envelope.Items), config.limit)
	}
	items := make([]ImportItem, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		if !strings.HasPrefix(item.ExternalID, "laro-") || (item.ItemType != "laro_case" && item.ItemType != "laro_legal_analysis") {
			return nil, "", fmt.Errorf("LARO feed returned an unsupported record contract")
		}
		item.ProjectKey = firstNonEmpty(strings.TrimSpace(item.ProjectKey), source.DefaultProjectKey)
		items = append(items, item)
	}
	return items, firstNonEmpty(strings.TrimSpace(envelope.NextCursor), source.Cursor), nil
}
