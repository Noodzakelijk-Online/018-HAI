package source

// This adapter deliberately uses only Odoo's JSON-2 search_read endpoint. It
// never accepts a model, method, endpoint, or API key from an HTTP request, so
// a connected source cannot turn HAI into a generic Odoo RPC client.

import (
	"bytes"
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
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/models"
)

const (
	odooJSON2ConnectorKey = "odoo-json2"
	odooJSON2CursorPrefix = "odoo-json2-write-date:"
	odooJSON2MaxLimit     = 50
	odooJSON2MaxResponse  = 2 << 20
)

type odooJSON2Config struct {
	baseURL  *url.URL
	database string
	apiKey   string
	profiles []odooReadProfile
	limit    int
}

type odooReadProfile struct {
	model  string
	label  string
	fields []string
	domain []any
	risk   string
}

func odooJSON2ConfigFromEnv() (odooJSON2Config, error) {
	if !envBool("HAI_ODOO_ENABLED") {
		return odooJSON2Config{}, fmt.Errorf("HAI_ODOO_ENABLED is false")
	}
	baseURL, err := parseOdooJSON2BaseURL(os.Getenv("HAI_ODOO_BASE_URL"))
	if err != nil {
		return odooJSON2Config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv("HAI_ODOO_API_KEY"))
	if apiKey == "" {
		return odooJSON2Config{}, fmt.Errorf("HAI_ODOO_API_KEY is not set")
	}
	profiles, err := selectedOdooReadProfiles(os.Getenv("HAI_ODOO_ALLOWED_MODELS"))
	if err != nil {
		return odooJSON2Config{}, err
	}
	return odooJSON2Config{
		baseURL:  baseURL,
		database: strings.TrimSpace(os.Getenv("HAI_ODOO_DATABASE")),
		apiKey:   apiKey,
		profiles: profiles,
		limit:    boundedOdooLimit(os.Getenv("HAI_ODOO_SYNC_LIMIT")),
	}, nil
}

func parseOdooJSON2BaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("HAI_ODOO_BASE_URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("HAI_ODOO_BASE_URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("HAI_ODOO_BASE_URL must not contain credentials, a path, query, or fragment")
	}
	if u.Scheme == "http" && !isOdooLocalHost(u.Hostname()) {
		return nil, fmt.Errorf("HAI_ODOO_BASE_URL must use HTTPS unless it targets a local Odoo host")
	}
	u.Path = ""
	return u, nil
}

func isOdooLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "host.docker.internal" || host == "odoo" {
		return true
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return address.IsLoopback() || address.IsPrivate()
}

func selectedOdooReadProfiles(raw string) ([]odooReadProfile, error) {
	profiles := map[string]odooReadProfile{}
	for _, profile := range defaultOdooReadProfiles() {
		profiles[profile.model] = profile
	}
	requested := splitOdooValues(raw)
	if len(requested) == 0 {
		requested = []string{"crm.lead", "project.task"}
	}
	selected := make([]odooReadProfile, 0, len(requested))
	seen := map[string]bool{}
	for _, model := range requested {
		profile, ok := profiles[model]
		if !ok {
			return nil, fmt.Errorf("HAI_ODOO_ALLOWED_MODELS contains unsupported model %q", model)
		}
		if !seen[model] {
			selected = append(selected, profile)
			seen[model] = true
		}
	}
	return selected, nil
}

func defaultOdooReadProfiles() []odooReadProfile {
	return []odooReadProfile{
		{model: "crm.lead", label: "CRM lead", fields: []string{"id", "name", "stage_id", "expected_revenue", "activity_date_deadline", "write_date"}, risk: "medium"},
		{model: "project.task", label: "Project task", fields: []string{"id", "name", "project_id", "stage_id", "date_deadline", "priority", "write_date"}, risk: "medium"},
		{model: "sale.order", label: "Sales order", fields: []string{"id", "name", "state", "date_order", "validity_date", "amount_total", "partner_id", "write_date"}, risk: "medium"},
		{model: "res.partner", label: "Contact", fields: []string{"id", "name", "email", "write_date"}, risk: "medium"},
		{model: "account.move", label: "Invoice or bill", fields: []string{"id", "name", "move_type", "state", "invoice_date", "invoice_date_due", "amount_residual", "partner_id", "write_date"}, domain: []any{[]any{"move_type", "in", []string{"out_invoice", "in_invoice"}}}, risk: "high"},
	}
}

func fetchOdooJSON2Source(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	config, err := odooJSON2ConfigFromEnv()
	if err != nil {
		return nil, "", err
	}
	items := make([]ImportItem, 0, len(config.profiles)*config.limit)
	latest := time.Time{}
	for _, profile := range config.profiles {
		records, err := config.searchRead(ctx, source.Cursor, profile)
		if err != nil {
			return nil, "", err
		}
		for _, record := range records {
			item, updatedAt, err := config.importItem(profile, record, source.DefaultProjectKey)
			if err != nil {
				return nil, "", err
			}
			items = append(items, item)
			if updatedAt.After(latest) {
				latest = updatedAt
			}
		}
	}
	if latest.IsZero() {
		return items, source.Cursor, nil
	}
	return items, odooJSON2CursorPrefix + latest.UTC().Format(time.RFC3339), nil
}

func (config odooJSON2Config) searchRead(ctx context.Context, cursor string, profile odooReadProfile) ([]map[string]any, error) {
	domain := append([]any(nil), profile.domain...)
	if after, ok := parseOdooCursor(cursor); ok {
		// >= deliberately allows a harmless duplicate at the boundary. Raw-item
		// upsert then prevents an equal timestamp from being lost after a limit.
		domain = append(domain, []any{"write_date", ">=", after.UTC().Format("2006-01-02 15:04:05")})
	}
	payload := map[string]any{
		"domain": domain,
		"fields": profile.fields,
		"limit":  config.limit,
		"order":  "write_date asc,id asc",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Odoo %s read: %w", profile.model, err)
	}
	endpoint := *config.baseURL
	endpoint.Path = "/json/2/" + profile.model + "/search_read"
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Odoo %s read: %w", profile.model, err)
	}
	request.Header.Set("Authorization", "bearer "+config.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Odoo-JSON2-ReadOnly/1.0")
	if config.database != "" {
		request.Header.Set("X-Odoo-Database", config.database)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read Odoo %s: %w", profile.model, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, odooJSON2MaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read Odoo %s response: %w", profile.model, err)
	}
	if len(data) > odooJSON2MaxResponse {
		return nil, fmt.Errorf("Odoo %s response exceeds %d bytes", profile.model, odooJSON2MaxResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Odoo %s read returned HTTP %d", profile.model, response.StatusCode)
	}
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode Odoo %s response: %w", profile.model, err)
	}
	return records, nil
}

func (config odooJSON2Config) importItem(profile odooReadProfile, record map[string]any, projectKey string) (ImportItem, time.Time, error) {
	id := positiveOdooID(record["id"])
	if id == 0 {
		return ImportItem{}, time.Time{}, fmt.Errorf("Odoo %s returned a record without an id", profile.model)
	}
	updatedAt, _ := parseOdooWriteDate(stringOdooValue(record["write_date"]))
	content := make([]string, 0, len(profile.fields)+2)
	content = append(content, "Odoo "+profile.label+": "+stringOdooValue(record["name"]))
	for _, field := range profile.fields {
		if field == "id" || field == "name" || field == "write_date" {
			continue
		}
		content = append(content, field+": "+stringOdooValue(record[field]))
	}
	if !updatedAt.IsZero() {
		content = append(content, "updated: "+updatedAt.UTC().Format(time.RFC3339))
	}
	uri := *config.baseURL
	uri.Path = "/web"
	uri.RawQuery = ""
	uri.Fragment = fmt.Sprintf("id=%d&model=%s&view_type=form", id, profile.model)
	return ImportItem{
		ExternalID: "odoo-json2:" + profile.model + ":" + strconv.Itoa(id),
		Title:      "Odoo " + profile.label + ": " + stringOdooValue(record["name"]),
		Content:    strings.Join(content, "\n"),
		SourceURI:  uri.String(),
		ItemType:   "odoo_" + strings.ReplaceAll(profile.model, ".", "_"),
		ProjectKey: projectKey,
		Metadata:   "connector=odoo-json2;model=" + profile.model + ";read_only=true;risk=" + profile.risk + ";write_back=disabled",
	}, updatedAt, nil
}

func parseOdooCursor(cursor string) (time.Time, bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(cursor), odooJSON2CursorPrefix)
	if raw == strings.TrimSpace(cursor) || raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	return parsed, err == nil
}

func parseOdooWriteDate(raw string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, strings.TrimSpace(raw), time.UTC); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func positiveOdooID(value any) int {
	switch typed := value.(type) {
	case float64:
		if typed > 0 && typed == float64(int(typed)) {
			return int(typed)
		}
	case int:
		if typed > 0 {
			return typed
		}
	case json.Number:
		parsed, _ := typed.Int64()
		if parsed > 0 {
			return int(parsed)
		}
	}
	return 0
}

func stringOdooValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case []any:
		if len(typed) > 1 {
			return stringOdooValue(typed[1])
		}
		if len(typed) == 1 {
			return stringOdooValue(typed[0])
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func boundedOdooLimit(raw string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return 20
	}
	if parsed > odooJSON2MaxLimit {
		return odooJSON2MaxLimit
	}
	return parsed
}

func splitOdooValues(raw string) []string {
	set := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			set[item] = true
		}
	}
	values := make([]string, 0, len(set))
	for item := range set {
		values = append(values, item)
	}
	sort.Strings(values)
	return values
}
