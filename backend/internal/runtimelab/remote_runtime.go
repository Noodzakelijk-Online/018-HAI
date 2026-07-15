package runtimelab

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"automation-hub-backend/internal/executionbroker"
)

// remoteRuntime is a truthful adapter for an external agent runtime (Hermes,
// OpenClaw, Odysseus). HAI does not rebuild these runtimes and does not
// auto-install them. The adapter reports not_configured with exact setup steps
// until an endpoint is configured, probes health truthfully, and NEVER claims
// execution — running a real task requires an operator-verified integration.
type remoteRuntime struct {
	id         string
	name       string
	baseURLEnv string
	healthPath string
	baseURL    string
	configErr  string
	httpClient *http.Client
}

func newRemoteRuntime(id, name, baseURLEnv string) *remoteRuntime {
	r := &remoteRuntime{
		id:         id,
		name:       name,
		baseURLEnv: baseURLEnv,
		healthPath: envDefault(strings.ToUpper(id)+"_HEALTH_PATH", "/health"),
		baseURL:    strings.TrimSpace(os.Getenv(baseURLEnv)),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	if r.baseURL == "" {
		r.configErr = baseURLEnv + " not set"
	} else if err := validateURL(r.baseURL); err != nil {
		r.configErr = err.Error()
	}
	return r
}

func (r *remoteRuntime) configured() bool { return r.baseURL != "" && r.configErr == "" }

func (r *remoteRuntime) Info() RuntimeInfo {
	return RuntimeInfo{
		ID:          r.id,
		DisplayName: r.name,
		Kind:        KindAgentRuntime,
		Description: r.name + " is an external agent runtime orchestrated by HAI as the control plane; HAI does not rebuild or auto-install it.",
	}
}

func (r *remoteRuntime) Capabilities() []string {
	// Declared capabilities only; none are claimed as exercised until a real,
	// operator-verified task runs.
	return []string{"declared:agent_task_execution", "declared:health_check"}
}

func (r *remoteRuntime) SetupRequirements() []SetupRequirement {
	return []SetupRequirement{
		{Step: "Provision the runtime", Detail: "Install/run " + r.name + " yourself; HAI never downloads or installs third-party runtimes automatically."},
		{Step: "Configure the endpoint", Detail: "Set " + r.baseURLEnv + " to a reachable " + r.name + " base URL (http/https, non-metadata host)."},
		{Step: "Pass the health probe", Detail: "POST /runtime-lab/" + r.id + "/probe must return active before any execution is considered."},
		{Step: "Operator-verify a real task", Detail: "A real " + r.name + " task must be executed and operator-verified before execution is enabled; HAI will not fake execution."},
	}
}

func (r *remoteRuntime) HealthCheck(ctx context.Context) Health {
	if !r.configured() {
		return Health{
			Status:            executionbroker.RuntimeNotConfigured,
			Detail:            r.configErr,
			Claim:             executionbroker.ClaimContractDefined,
			SetupRequirements: r.SetupRequirements(),
		}
	}
	// Configured but not yet probed in this call: report configured, not active.
	return Health{
		Status:            executionbroker.RuntimeBlocked,
		Detail:            "endpoint configured; probe required before use, execution not enabled (no fake execution)",
		Claim:             executionbroker.ClaimConfigured,
		SetupRequirements: r.SetupRequirements()[2:],
	}
}

func (r *remoteRuntime) Probe(ctx context.Context, now time.Time) ProbeResult {
	if !r.configured() {
		return ProbeResult{RuntimeID: r.id, Status: executionbroker.RuntimeNotConfigured, Detail: r.configErr, CheckedAt: now}
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.baseURL, "/")+r.healthPath, nil)
	if err != nil {
		return ProbeResult{RuntimeID: r.id, Status: executionbroker.RuntimeFailed, Detail: err.Error(), CheckedAt: now}
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return ProbeResult{RuntimeID: r.id, Status: executionbroker.RuntimeUnavailable, Detail: err.Error(), CheckedAt: now}
	}
	defer resp.Body.Close()
	dur := time.Since(start).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		return ProbeResult{RuntimeID: r.id, Status: executionbroker.RuntimeUnavailable, DurationMs: dur, Detail: fmt.Sprintf("health HTTP %d", resp.StatusCode), CheckedAt: now}
	}
	// Reachable — but reachability is not permission to execute (no fake exec).
	return ProbeResult{RuntimeID: r.id, Status: executionbroker.RuntimeReady, DurationMs: dur, Detail: "health probe ok; execution still requires operator verification", CheckedAt: now}
}

func (r *remoteRuntime) Execute(ctx context.Context, payload map[string]any) (executionbroker.RuntimeResult, error) {
	// Never fake execution. Even when the health probe passes, HAI has no
	// operator-verified integration for this runtime in this phase.
	msg := r.name + " execution is not enabled: complete the runtime setup requirements and operator-verify a real task first"
	return executionbroker.RuntimeResult{OK: false, Error: msg}, fmt.Errorf("runtimelab: %s", msg)
}

func (r *remoteRuntime) Stop(ctx context.Context) error { return nil }

// validateURL rejects invalid, unspecified, link-local, and metadata hosts.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http/https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL host is empty")
	}
	if strings.Contains(strings.ToLower(host), "metadata") {
		return fmt.Errorf("metadata host not allowed")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsLoopback():
			return nil
		case ip.IsUnspecified():
			return fmt.Errorf("unspecified host not allowed")
		case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
			return fmt.Errorf("link-local host not allowed")
		case ip.String() == "169.254.169.254":
			return fmt.Errorf("metadata host not allowed")
		}
	}
	return nil
}

func envDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
