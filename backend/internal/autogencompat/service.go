// Package autogencompat turns a reviewed subset of exported AutoGen-style
// events into HAI-owned migration review signals. It deliberately has no
// AutoGen dependency, network client, runtime, tool, or workflow mutation.
package autogencompat

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"automation-hub-backend/internal/safety"
)

const (
	maxEvents       = 100
	maxSummaryRunes = 1200
	maxIDRunes      = 160
)

var ErrInvalidInput = errors.New("AutoGen compatibility input is invalid")

type Status struct {
	Available    bool     `json:"available"`
	Provider     string   `json:"provider"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type Event struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Agent         string    `json:"agent,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurredAt,omitempty"`
}

type PreviewRequest struct {
	WorkloadID string  `json:"workloadId"`
	Events     []Event `json:"events"`
}

type NormalizedEvent struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Agent         string    `json:"agent,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurredAt,omitempty"`
	HAIHandling   string    `json:"haiHandling"`
}

type OpenLoop struct {
	Kind          string `json:"kind"`
	CorrelationID string `json:"correlationId,omitempty"`
	Summary       string `json:"summary"`
	Recovery      string `json:"recovery"`
}

type ControlRecommendation struct {
	Control string `json:"control"`
	Reason  string `json:"reason"`
}

type Preview struct {
	WorkloadID             string                  `json:"workloadId"`
	Events                 []NormalizedEvent       `json:"events"`
	OpenLoops              []OpenLoop              `json:"openLoops"`
	RecommendedControls    []ControlRecommendation `json:"recommendedControls"`
	RequiresReview         bool                    `json:"requiresReview"`
	ExecutionAllowed       bool                    `json:"executionAllowed"`
	PersistenceAllowed     bool                    `json:"persistenceAllowed"`
	CompletionVerification string                  `json:"completionVerification"`
	Scope                  string                  `json:"scope"`
}

// MigrationRequest selects the one framework target HAI has reviewed as an
// AutoGen successor. The request remains a transient planning input.
type MigrationRequest struct {
	PreviewRequest
	Target string `json:"target"`
}

type MigrationStep struct {
	Order              int      `json:"order"`
	HAIControl         string   `json:"haiControl"`
	AgentFrameworkRole string   `json:"agentFrameworkRole"`
	RequiredEvents     []string `json:"requiredEvents,omitempty"`
	Gate               string   `json:"gate"`
}

// MigrationPlan is a non-executable implementation plan. It never asserts
// that a framework installation, workflow, task, or checkpoint exists.
type MigrationPlan struct {
	Target                   string          `json:"target"`
	Preview                  *Preview        `json:"preview"`
	Steps                    []MigrationStep `json:"steps"`
	BlockedUntil             []string        `json:"blockedUntil"`
	ExecutionAllowed         bool            `json:"executionAllowed"`
	FrameworkRuntimeDetected bool            `json:"frameworkRuntimeDetected"`
	Scope                    string          `json:"scope"`
}

type Service struct{}

func DefaultService() *Service { return &Service{} }

func (s *Service) Status() Status {
	return Status{
		Available: true,
		Provider:  "AutoGen compatibility migration preview",
		Capabilities: []string{
			"bounded event-envelope normalization",
			"open-loop detection for handoffs, approvals, and tool calls",
			"HAI control and verification recommendations",
			"Microsoft Agent Framework migration-plan translation",
		},
		Restrictions: []string{
			"no AutoGen package, process, remote bridge, model provider, MCP server, tool, code executor, or workflow is started",
			"no event input is persisted as a source, memory, workflow, task, audit record, or completion claim",
			"every imported completion signal remains unverified until HAI independently verifies it",
		},
		Scope: "Use this owner-authenticated review endpoint to map a small redacted export from an existing AutoGen workload into HAI-native review controls before a separate migration or runtime-adapter decision.",
	}
}

// MigrationPlan converts a reviewed AutoGen event sample into a staged
// Microsoft Agent Framework migration plan. It cannot contact, install, or
// configure that framework; HAI remains the task, policy, and execution owner.
func (s *Service) MigrationPlan(request MigrationRequest) (*MigrationPlan, error) {
	if normalizeTarget(request.Target) != "microsoft-agent-framework" {
		return nil, ErrInvalidInput
	}
	preview, err := s.Preview(request.PreviewRequest)
	if err != nil {
		return nil, err
	}

	eventTypes := map[string]bool{}
	for _, event := range preview.Events {
		eventTypes[event.Type] = true
	}
	steps := []MigrationStep{
		{
			Order:              1,
			HAIControl:         "task intake and source-linked workflow state",
			AgentFrameworkRole: "map event ingress and handoff concepts to a fixed workflow schema",
			RequiredEvents:     matchingEventTypes(eventTypes, "message", "handoff", "task_started"),
			Gate:               "a reviewer must define the HAI task schema, owner, success criteria, and recovery state before any workflow is created",
		},
		{
			Order:              2,
			HAIControl:         "local-first model router and provider budget policy",
			AgentFrameworkRole: "configure framework provider middleware to call only HAI-approved local model routes",
			Gate:               "no framework provider, cloud credential, or paid model setting may bypass HAI's EUR 0 policy, model maintenance, or approval rules",
		},
		{
			Order:              3,
			HAIControl:         "runtime registry, tool allowlist, and approval queue",
			AgentFrameworkRole: "translate reviewed tool intent into named HAI runtime proposals",
			RequiredEvents:     matchingEventTypes(eventTypes, "tool_call", "tool_result", "approval_request"),
			Gate:               "each tool needs a reviewed local adapter, explicit workspace and network scope, and the existing HAI risk decision before it can run",
		},
		{
			Order:              4,
			HAIControl:         "verification, audit, and durable follow-up state",
			AgentFrameworkRole: "map checkpoints and terminal events to HAI verification candidates and recovery records",
			RequiredEvents:     matchingEventTypes(eventTypes, "task_completed", "task_failed", "termination"),
			Gate:               "framework checkpoint or completion output is non-authoritative until HAI independently verifies evidence and records a state transition",
		},
	}

	return &MigrationPlan{
		Target:                   "microsoft-agent-framework",
		Preview:                  preview,
		Steps:                    steps,
		BlockedUntil:             []string{"owner approves a narrow local bridge", "a reviewed framework version and license are pinned", "provider, tool, workspace, network, redaction, audit, rollback, and emergency-stop controls are configured and tested"},
		ExecutionAllowed:         false,
		FrameworkRuntimeDetected: false,
		Scope:                    "Migration planning only. HAI did not install, probe, contact, configure, or execute Microsoft Agent Framework, a model, an MCP server, a tool, or a workflow.",
	}, nil
}

func (s *Service) Preview(request PreviewRequest) (*Preview, error) {
	workloadID := normalizeID(request.WorkloadID)
	if workloadID == "" || len(request.Events) == 0 || len(request.Events) > maxEvents {
		return nil, ErrInvalidInput
	}

	result := &Preview{
		WorkloadID:             workloadID,
		Events:                 make([]NormalizedEvent, 0, len(request.Events)),
		OpenLoops:              []OpenLoop{},
		RecommendedControls:    []ControlRecommendation{},
		RequiresReview:         true,
		ExecutionAllowed:       false,
		PersistenceAllowed:     false,
		CompletionVerification: "No imported event can complete HAI work. Attach source-backed evidence and run HAI verification before any task, workflow, memory, or action changes state.",
		Scope:                  "Compatibility preview only. HAI did not install or call AutoGen, contact a model/provider, invoke MCP, run a tool, create a task, persist an event, request approval, or execute work.",
	}

	pendingTools := map[string]NormalizedEvent{}
	pendingHandoffs := map[string]NormalizedEvent{}
	seenControls := map[string]bool{}
	addControl := func(control, reason string) {
		if !seenControls[control] {
			seenControls[control] = true
			result.RecommendedControls = append(result.RecommendedControls, ControlRecommendation{Control: control, Reason: reason})
		}
	}

	for _, raw := range request.Events {
		event, err := normalizeEvent(raw)
		if err != nil {
			return nil, ErrInvalidInput
		}
		result.Events = append(result.Events, event)
		switch event.Type {
		case "tool_call":
			pendingTools[eventKey(event)] = event
			addControl("runtime safety review", "Imported tool intent needs HAI tool, workspace, network, and approval review before any execution.")
		case "tool_result":
			delete(pendingTools, eventKey(event))
			addControl("verification review", "Imported tool results are non-authoritative evidence until HAI checks the claimed outcome.")
		case "handoff":
			pendingHandoffs[eventKey(event)] = event
			addControl("workflow assignment review", "Imported delegation needs an HAI-owned assignee, next action, and approval boundary.")
		case "approval_request":
			addControl("approval queue", "An imported approval request must be recreated through HAI's approval policy; it is not automatically approved.")
		case "task_completed":
			addControl("completion verification", "An upstream completion claim requires HAI evidence, validation, and verification before closure.")
		case "task_failed":
			addControl("recovery review", "An imported failure needs a HAI recovery decision rather than silent retry.")
		}
	}

	for _, event := range sortedEvents(pendingTools) {
		result.OpenLoops = append(result.OpenLoops, OpenLoop{Kind: "unverified_tool_call", CorrelationID: event.CorrelationID, Summary: event.Summary, Recovery: "Review the intended tool action in HAI. Do not replay it automatically."})
	}
	for _, event := range sortedEvents(pendingHandoffs) {
		result.OpenLoops = append(result.OpenLoops, OpenLoop{Kind: "unresolved_handoff", CorrelationID: event.CorrelationID, Summary: event.Summary, Recovery: "Create a HAI-owned assignment only after a reviewer confirms the objective, owner, and next action."})
	}
	return result, nil
}

func normalizeEvent(raw Event) (NormalizedEvent, error) {
	identifier := normalizeID(raw.ID)
	typeName := strings.ToLower(strings.TrimSpace(raw.Type))
	summary := boundedSummary(raw.Summary)
	if identifier == "" || !allowedEventType(typeName) || summary == "" {
		return NormalizedEvent{}, ErrInvalidInput
	}
	agent := normalizeOptionalID(raw.Agent)
	correlationID := normalizeOptionalID(raw.CorrelationID)
	return NormalizedEvent{ID: identifier, Type: typeName, Agent: agent, CorrelationID: correlationID, Summary: safety.RedactSecrets(summary), OccurredAt: raw.OccurredAt.UTC(), HAIHandling: handlingFor(typeName)}, nil
}

func allowedEventType(value string) bool {
	switch value {
	case "message", "handoff", "tool_call", "tool_result", "approval_request", "task_started", "task_completed", "task_failed", "termination":
		return true
	default:
		return false
	}
}

func handlingFor(eventType string) string {
	switch eventType {
	case "tool_call":
		return "review as a proposed runtime action; do not replay"
	case "tool_result":
		return "treat as unverified evidence"
	case "handoff":
		return "review before creating a HAI assignment"
	case "approval_request":
		return "re-evaluate through HAI approval policy"
	case "task_completed":
		return "verify independently before HAI completion"
	case "task_failed":
		return "review recovery and retry limits"
	default:
		return "retain only in this preview as non-authoritative context"
	}
}

func eventKey(event NormalizedEvent) string {
	if event.CorrelationID != "" {
		return event.CorrelationID
	}
	return event.ID
}

func sortedEvents(values map[string]NormalizedEvent) []NormalizedEvent {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]NormalizedEvent, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func normalizeID(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || utf8.RuneCountInString(value) > maxIDRunes || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func normalizeOptionalID(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return normalizeID(value)
}

func boundedSummary(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || utf8.RuneCountInString(value) > maxSummaryRunes {
		return ""
	}
	return value
}

func normalizeTarget(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func matchingEventTypes(observed map[string]bool, supported ...string) []string {
	result := make([]string, 0, len(supported))
	for _, eventType := range supported {
		if observed[eventType] {
			result = append(result, eventType)
		}
	}
	return result
}
