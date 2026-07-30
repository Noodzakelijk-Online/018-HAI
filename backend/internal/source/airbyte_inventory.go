package source

// Airbyte stays outside HAI's connector and credential authority. This adapter
// reads a bounded inventory from one operator-configured local Airbyte API; it
// never creates, changes, starts, stops, or deletes a source or connection.

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
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

const (
	airbyteInventoryConnectorKey  = "airbyte-inventory"
	airbyteInventoryCursor        = "airbyte-inventory-v1"
	airbyteInventoryLimit         = 100
	airbyteInventoryMaxWorkspaces = 10
	airbyteInventoryMaxResponse   = 512 << 10
)

type airbyteInventoryConfig struct {
	baseURL      *url.URL
	apiKey       string
	workspaceIDs []string
}

type airbyteSourceRecord struct {
	SourceID    string `json:"sourceId"`
	Name        string `json:"name"`
	SourceType  string `json:"sourceType"`
	WorkspaceID string `json:"workspaceId"`
}

type airbyteConnectionRecord struct {
	ConnectionID  string `json:"connectionId"`
	Name          string `json:"name"`
	SourceID      string `json:"sourceId"`
	DestinationID string `json:"destinationId"`
	WorkspaceID   string `json:"workspaceId"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"createdAt"`
	Schedule      struct {
		ScheduleType string `json:"scheduleType"`
	} `json:"schedule"`
}

type airbyteListResponse[T any] struct {
	Data []T    `json:"data"`
	Next string `json:"next"`
}

func airbyteInventoryConfigFromEnv() (airbyteInventoryConfig, error) {
	if !envBool("HAI_AIRBYTE_ENABLED") {
		return airbyteInventoryConfig{}, fmt.Errorf("HAI_AIRBYTE_ENABLED is false")
	}
	baseURL, err := parseAirbyteInventoryBaseURL(os.Getenv("HAI_AIRBYTE_BASE_URL"))
	if err != nil {
		return airbyteInventoryConfig{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv("HAI_AIRBYTE_API_KEY"))
	if apiKey == "" {
		return airbyteInventoryConfig{}, fmt.Errorf("HAI_AIRBYTE_API_KEY is not set")
	}
	workspaceIDs, err := airbyteWorkspaceAllowlist(os.Getenv("HAI_AIRBYTE_WORKSPACE_IDS"))
	if err != nil {
		return airbyteInventoryConfig{}, err
	}
	return airbyteInventoryConfig{baseURL: baseURL, apiKey: apiKey, workspaceIDs: workspaceIDs}, nil
}

func parseAirbyteInventoryBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("HAI_AIRBYTE_BASE_URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("HAI_AIRBYTE_BASE_URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("HAI_AIRBYTE_BASE_URL must not contain credentials, query, or fragments")
	}
	if !isAirbyteLocalHost(u.Hostname()) {
		return nil, fmt.Errorf("HAI_AIRBYTE_BASE_URL must target a local Airbyte host")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

func isAirbyteLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "host.docker.internal" || host == "airbyte" || host == "airbyte-server" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && (address.IsLoopback() || address.IsPrivate())
}

func airbyteWorkspaceAllowlist(raw string) ([]string, error) {
	values := splitOdooValues(raw)
	if len(values) == 0 {
		return nil, fmt.Errorf("HAI_AIRBYTE_WORKSPACE_IDS must contain at least one approved workspace UUID")
	}
	if len(values) > airbyteInventoryMaxWorkspaces {
		return nil, fmt.Errorf("HAI_AIRBYTE_WORKSPACE_IDS exceeds the %d-workspace safety limit", airbyteInventoryMaxWorkspaces)
	}
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return nil, fmt.Errorf("HAI_AIRBYTE_WORKSPACE_IDS contains an invalid UUID")
		}
	}
	return values, nil
}

func fetchAirbyteInventory(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	config, err := airbyteInventoryConfigFromEnv()
	if err != nil {
		return nil, "", err
	}
	if !source.LocalOnly {
		return nil, "", fmt.Errorf("Airbyte inventory must remain local-only")
	}
	sources, err := airbyteList[airbyteSourceRecord](ctx, config, "sources")
	if err != nil {
		return nil, "", err
	}
	connections, err := airbyteList[airbyteConnectionRecord](ctx, config, "connections")
	if err != nil {
		return nil, "", err
	}
	items := make([]ImportItem, 0, len(sources)+len(connections))
	for _, record := range sources {
		if !airbyteAllowedWorkspace(config.workspaceIDs, record.WorkspaceID) || !validAirbyteUUID(record.SourceID) || strings.TrimSpace(record.Name) == "" || strings.TrimSpace(record.SourceType) == "" {
			continue
		}
		items = append(items, airbyteSourceImport(config, source, record))
	}
	for _, record := range connections {
		if !airbyteAllowedWorkspace(config.workspaceIDs, record.WorkspaceID) || !validAirbyteUUID(record.ConnectionID) || !validAirbyteUUID(record.SourceID) || !validAirbyteUUID(record.DestinationID) || strings.TrimSpace(record.Name) == "" || !validAirbyteConnectionStatus(record.Status) {
			continue
		}
		items = append(items, airbyteConnectionImport(config, source, record))
	}
	if len(items) == 0 && (len(sources) > 0 || len(connections) > 0) {
		return nil, "", fmt.Errorf("Airbyte returned no valid records for the approved workspace allowlist")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ExternalID < items[j].ExternalID })
	return items, airbyteInventoryCursor, nil
}

func airbyteList[T any](ctx context.Context, config airbyteInventoryConfig, resource string) ([]T, error) {
	endpoint := *config.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + resource
	query := endpoint.Query()
	query.Set("workspaceIds", strings.Join(config.workspaceIDs, ","))
	query.Set("includeDeleted", "false")
	query.Set("limit", "100")
	query.Set("offset", "0")
	endpoint.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Airbyte %s inventory request: %w", resource, err)
	}
	request.Header.Set("Authorization", "Bearer "+config.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Airbyte-Inventory/1.0")
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read Airbyte %s inventory: %w", resource, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, airbyteInventoryMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read Airbyte %s response: %w", resource, err)
	}
	if len(body) > airbyteInventoryMaxResponse {
		return nil, fmt.Errorf("Airbyte %s response exceeds the %d-byte safety limit", resource, airbyteInventoryMaxResponse)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Airbyte %s inventory returned HTTP %d", resource, response.StatusCode)
	}
	var payload airbyteListResponse[T]
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Airbyte %s response: %w", resource, err)
	}
	if payload.Next != "" {
		return nil, fmt.Errorf("Airbyte %s inventory exceeds HAI's fixed one-page limit", resource)
	}
	return payload.Data, nil
}

func airbyteSourceImport(config airbyteInventoryConfig, source *models.ConnectedSource, record airbyteSourceRecord) ImportItem {
	return ImportItem{
		ExternalID: "airbyte-source:" + record.SourceID,
		Title:      "Airbyte source: " + compact(record.Name, 180),
		Content:    "Airbyte source inventory. Name: " + compact(record.Name, 180) + ". Type: " + compact(record.SourceType, 100) + ". Workspace: " + record.WorkspaceID + ". Credentials, connector configuration, records, and sync execution are not read by HAI.",
		SourceURI:  airbyteURI(config, "sources", record.SourceID),
		ItemType:   "airbyte_source_inventory",
		ProjectKey: source.DefaultProjectKey,
		Metadata:   "connector=airbyte-inventory;read_only=true;configuration=excluded;credentials=excluded;sync_execution=disabled",
	}
}

func airbyteConnectionImport(config airbyteInventoryConfig, source *models.ConnectedSource, record airbyteConnectionRecord) ImportItem {
	content := "Airbyte connection inventory. Name: " + compact(record.Name, 180) + ". Status: " + record.Status + ". Schedule: " + firstNonEmpty(record.Schedule.ScheduleType, "unknown") + ". Workspace: " + record.WorkspaceID + ". Source: " + record.SourceID + ". Destination: " + record.DestinationID + ". Connection configuration, selected fields, credentials, sync records, and sync execution are not read by HAI."
	return ImportItem{
		ExternalID: "airbyte-connection:" + record.ConnectionID,
		Title:      "Airbyte connection: " + compact(record.Name, 180),
		Content:    content,
		SourceURI:  airbyteURI(config, "connections", record.ConnectionID),
		ItemType:   "airbyte_connection_inventory",
		ProjectKey: source.DefaultProjectKey,
		Metadata:   "connector=airbyte-inventory;read_only=true;configuration=excluded;credentials=excluded;sync_execution=disabled;status=" + record.Status,
	}
}

func airbyteURI(config airbyteInventoryConfig, resource, id string) string {
	uri := *config.baseURL
	uri.Path = strings.TrimRight(uri.Path, "/") + "/" + resource + "/" + id
	uri.RawQuery = ""
	uri.Fragment = ""
	return uri.String()
}

func airbyteAllowedWorkspace(allowed []string, workspaceID string) bool {
	for _, value := range allowed {
		if value == workspaceID {
			return true
		}
	}
	return false
}

func validAirbyteUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func validAirbyteConnectionStatus(value string) bool {
	switch value {
	case "active", "inactive", "deprecated":
		return true
	default:
		return false
	}
}
