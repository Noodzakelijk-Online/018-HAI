// Package runnermaintenance enforces the canonical HAI model-maintenance gate
// for optional local runners that invoke a model outside the main generation
// endpoint. It deliberately accepts only a runner's declared endpoint and
// model ID; the LLM service remains the authority for policy and refreshes.
package runnermaintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxHealthResponseBytes int64 = 4096

// Gate is implemented by the central LLM service. The runner cannot choose a
// provider or change a model: it must prove an exact configured pair first.
type Gate interface {
	EnsureConfiguredLocalModel(endpointURL, modelID string) error
}

// EnsureConfiguredLocalModel fetches a bounded local runner health record and
// passes its fixed model configuration to HAI's canonical policy gate. The
// caller's existing HTTP client owns timeout, local-host restrictions, and
// no-redirect behavior for the runner connection.
func EnsureConfiguredLocalModel(ctx context.Context, client *http.Client, baseURL *url.URL, userAgent, runnerName string, gate Gate) error {
	if gate == nil {
		return fmt.Errorf("central daily model maintenance gate is unavailable")
	}
	if client == nil || baseURL == nil {
		return fmt.Errorf("local %s runner is unavailable", runnerName)
	}
	endpoint := *baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/healthz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("could not create local %s maintenance request", runnerName)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("local %s runner is unavailable", runnerName)
	}
	defer response.Body.Close()

	var body struct {
		Status        string `json:"status"`
		Configured    bool   `json:"configured"`
		ModelID       string `json:"modelId"`
		ModelEndpoint string `json:"modelEndpoint"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, maxHealthResponseBytes)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || !boundedText(body.ModelID, 160) || !boundedText(body.ModelEndpoint, 512) {
		return fmt.Errorf("local %s runner did not disclose one fixed local model configuration", runnerName)
	}
	if err := gate.EnsureConfiguredLocalModel(body.ModelEndpoint, body.ModelID); err != nil {
		return fmt.Errorf("local %s model is not admitted: %w", runnerName, err)
	}
	return nil
}

func boundedText(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maximum && !strings.ContainsAny(value, "\r\n")
}
