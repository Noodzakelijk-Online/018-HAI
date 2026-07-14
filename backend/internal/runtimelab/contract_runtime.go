package runtimelab

import (
	"context"
	"fmt"
	"time"

	"automation-hub-backend/internal/executionbroker"
)

// contractRuntime is a documented runtime contract that is NOT yet an executor
// (browser runtime, local script runtime, future MCP tools). It publishes its
// capability boundary and setup requirements but always refuses execution — the
// forbidden-by-default boundary is enforced, not simulated.
type contractRuntime struct {
	id        string
	name      string
	kind      RuntimeKind
	desc      string
	allowed   []string
	forbidden []string
	setup     []SetupRequirement
}

func (c *contractRuntime) Info() RuntimeInfo {
	return RuntimeInfo{ID: c.id, DisplayName: c.name, Kind: c.kind, Description: c.desc}
}

func (c *contractRuntime) Capabilities() []string {
	caps := make([]string, 0, len(c.allowed)+len(c.forbidden))
	for _, a := range c.allowed {
		caps = append(caps, "allowed:"+a)
	}
	for _, f := range c.forbidden {
		caps = append(caps, "forbidden:"+f)
	}
	return caps
}

func (c *contractRuntime) SetupRequirements() []SetupRequirement { return c.setup }

func (c *contractRuntime) HealthCheck(ctx context.Context) Health {
	return Health{
		Status:            executionbroker.RuntimeNotConfigured,
		Detail:            c.name + " is a defined contract only; no executor is wired and none is faked",
		Claim:             executionbroker.ClaimContractDefined,
		SetupRequirements: c.setup,
	}
}

func (c *contractRuntime) Probe(ctx context.Context, now time.Time) ProbeResult {
	return ProbeResult{RuntimeID: c.id, Status: executionbroker.RuntimeNotConfigured, Detail: "contract only; nothing to probe", CheckedAt: now}
}

func (c *contractRuntime) Execute(ctx context.Context, payload map[string]any) (executionbroker.RuntimeResult, error) {
	msg := c.name + " has no executor: this is a defined contract only (HAI will not fake execution)"
	return executionbroker.RuntimeResult{OK: false, Error: msg}, fmt.Errorf("runtimelab: %s", msg)
}

func (c *contractRuntime) Stop(ctx context.Context) error { return nil }

// newBrowserContract builds the browser runtime contract with the §15 allowed
// early actions and forbidden-by-default boundary.
func newBrowserContract() *contractRuntime {
	return &contractRuntime{
		id:   "browser-runtime",
		name: "Browser Runtime",
		kind: KindBrowser,
		desc: "Bounded browser automation contract: read-only/draft actions inside an allowlisted domain; state-changing actions are forbidden by default.",
		allowed: []string{
			"open_allowlisted_page", "read_visible_page_text", "capture_screenshot",
			"fill_draft_fields_without_submitting", "download_file_into_allowlisted_folder",
			"extract_url_title_body", "stop_at_login_captcha_payment_account_settings",
		},
		forbidden: []string{
			"send_message", "submit_form", "delete", "pay", "accept_contract",
			"change_account_settings", "bypass_captcha", "scrape_outside_domain",
			"navigate_unknown_websites", "access_passwords_cookies_localstorage", "public_posting",
		},
		setup: []SetupRequirement{
			{Step: "Configure a domain allowlist", Detail: "Only allowlisted domains may be opened; unknown websites are blocked."},
			{Step: "Wire a real, bounded browser executor", Detail: "A screenshot-before/after, approval-gated executor must be provided; none is faked."},
			{Step: "Operator-verify read-only actions first", Detail: "State-changing actions remain forbidden by default until explicitly approved per action."},
		},
	}
}

// newLocalScriptContract builds the local script runtime contract.
func newLocalScriptContract() *contractRuntime {
	return &contractRuntime{
		id:        "local-script-runtime",
		name:      "Local Script Runtime",
		kind:      KindLocalScript,
		desc:      "Bounded local script execution contract; no executor is wired in this phase and arbitrary command execution is forbidden.",
		allowed:   []string{"declared:run_allowlisted_bounded_script"},
		forbidden: []string{"arbitrary_command_execution", "network_egress", "filesystem_access_outside_workspace"},
		setup: []SetupRequirement{
			{Step: "Define a script allowlist", Detail: "Only explicitly allowlisted, bounded scripts may run; arbitrary execution is forbidden."},
			{Step: "Confine to a workspace", Detail: "Execution must be confined like the local safe worker (no path escape, no network)."},
			{Step: "Operator-verify before enabling", Detail: "A real bounded run must be operator-verified before execution is enabled."},
		},
	}
}
