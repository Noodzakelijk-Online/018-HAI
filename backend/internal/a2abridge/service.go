// Package a2abridge implements a deliberately small, local-only subset of the
// Agent2Agent protocol. It gives one named peer a non-executable HAI planning
// draft without exposing sources, memory, credentials, workflow mutation, or
// runtime controls.
package a2abridge

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"automation-hub-backend/internal/task"
)

const (
	enabledEnv = "HAI_A2A_BRIDGE_ENABLED"
	tokenEnv   = "HAI_A2A_BRIDGE_TOKEN"
	ownerEnv   = "HAI_A2A_BRIDGE_OWNER_ID"
	urlEnv     = "HAI_A2A_BRIDGE_URL"
)

var (
	ErrUnavailable  = errors.New("local A2A bridge is not configured")
	ErrInvalidInput = errors.New("A2A task input is invalid")
)

type Config struct {
	Enabled bool
	Token   string
	OwnerID string
	URL     string
}

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type AgentCard struct {
	ProtocolVersion      string                `json:"protocolVersion"`
	Name                 string                `json:"name"`
	Description          string                `json:"description"`
	URL                  string                `json:"url"`
	Version              string                `json:"version"`
	Capabilities         AgentCapabilities     `json:"capabilities"`
	DefaultInputModes    []string              `json:"defaultInputModes"`
	DefaultOutputModes   []string              `json:"defaultOutputModes"`
	Skills               []AgentSkill          `json:"skills"`
	SecuritySchemes      map[string]any        `json:"securitySchemes"`
	SecurityRequirements []map[string][]string `json:"security"`
}

type AgentCapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

type Proposal struct {
	TaskType         string         `json:"taskType"`
	RiskLevel        string         `json:"riskLevel"`
	NeedsApproval    bool           `json:"needsApproval"`
	SuccessCriteria  []string       `json:"successCriteria"`
	Steps            []ProposalStep `json:"steps"`
	NextAction       string         `json:"nextAction"`
	CompletionStatus string         `json:"completionStatus"`
	Scope            string         `json:"scope"`
}

type ProposalStep struct {
	Name             string `json:"name"`
	Purpose          string `json:"purpose"`
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
}

type Service struct {
	config  Config
	planner task.PreviewService
	err     string
}

func DefaultConfig() Config {
	return Config{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		Token:   strings.TrimSpace(os.Getenv(tokenEnv)),
		OwnerID: strings.TrimSpace(os.Getenv(ownerEnv)),
		URL:     strings.TrimSpace(os.Getenv(urlEnv)),
	}
}

func NewService(config Config, planner task.PreviewService) *Service {
	s := &Service{config: config, planner: planner}
	if !config.Enabled {
		return s
	}
	if len(config.Token) < 32 || strings.ContainsAny(config.Token, "\r\n") {
		s.err = tokenEnv + " must contain at least 32 non-newline characters"
		return s
	}
	if !validOwner(config.OwnerID) {
		s.err = ownerEnv + " must contain one configured owner identity"
		return s
	}
	if err := validateLocalURL(config.URL); err != nil {
		s.err = err.Error()
		return s
	}
	if planner == nil {
		s.err = "side-effect-free HAI planning is unavailable"
	}
	return s
}

func NewServiceFromEnv(planner task.PreviewService) *Service {
	return NewService(DefaultConfig(), planner)
}

func (s *Service) Status() Status {
	return Status{
		Enabled:     s.config.Enabled,
		Configured:  s.configured(),
		Provider:    "A2A local planning bridge",
		Endpoint:    s.config.URL,
		ConfigError: s.err,
		Capabilities: []string{
			"authenticated non-executable planning drafts",
			"A2A Agent Card capability advertisement",
			"one configured local peer token and owner scope",
		},
		Restrictions: []string{
			"no workflow, task, source, memory, approval, policy, or runtime mutation",
			"no source evidence, memory records, credentials, raw audit records, files, or tool inventory exposure",
			"no streaming, push notifications, peer discovery, remote URL, or external-agent invocation",
		},
		Scope: "A configured local peer can request a bounded planning draft for one configured owner. HAI retains the authoritative workflow, approval, execution, source, memory, verification, and audit paths.",
	}
}

func (s *Service) Authorize(token string) bool {
	if !s.configured() || len(token) != len(s.config.Token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

func (s *Service) AgentCard() (*AgentCard, error) {
	if !s.configured() {
		return nil, ErrUnavailable
	}
	return &AgentCard{
		ProtocolVersion:    "1.0",
		Name:               "HAI controlled planning",
		Description:        "Local, token-authenticated planning drafts only. This agent cannot execute, approve, mutate, or disclose HAI internals.",
		URL:                s.config.URL,
		Version:            "1.0.0",
		Capabilities:       AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"application/json"},
		Skills: []AgentSkill{{
			ID: "hai_controlled_planning", Name: "Controlled planning draft",
			Description: "Classifies a bounded text request and returns an advisory HAI planning proposal without taking operational action.",
			Tags:        []string{"planning", "local-first", "approval-gated", "read-only"},
			Examples:    []string{"Plan how to prepare a source-backed response without sending it."},
		}},
		SecuritySchemes: map[string]any{
			"haiLocalBearer": map[string]string{"type": "http", "scheme": "bearer", "description": "Configured local A2A bridge token."},
		},
		SecurityRequirements: []map[string][]string{{"haiLocalBearer": {}}},
	}, nil
}

func (s *Service) Draft(request string) (*Proposal, error) {
	if !s.configured() {
		return nil, ErrUnavailable
	}
	request = normalize(request)
	if request == "" || utf8.RuneCountInString(request) > 4096 {
		return nil, ErrInvalidInput
	}
	plan, err := s.planner.Preview(task.IntakeRequest{OwnerIdentity: s.config.OwnerID, Request: request})
	if err != nil {
		return nil, err
	}
	result := &Proposal{
		TaskType:         bounded(plan.Intake.TaskType, 80),
		RiskLevel:        bounded(plan.RiskAssessment.Level, 40),
		NeedsApproval:    plan.RiskAssessment.ApprovalRequired,
		SuccessCriteria:  boundedList(plan.Intake.SuccessCriteria, 6, 180),
		NextAction:       bounded(plan.ValidationResult.NextAction, 220),
		CompletionStatus: bounded(plan.CompletionStatus, 80),
		Scope:            "Planning draft only. It did not create a task, change a workflow, refresh a source, request approval, call a model, or execute a tool.",
	}
	for _, step := range plan.Steps {
		if len(result.Steps) == 8 {
			break
		}
		result.Steps = append(result.Steps, ProposalStep{
			Name: bounded(step.Name, 100), Purpose: bounded(step.Purpose, 180),
			RequiresApproval: step.RequiresApproval, Status: bounded(step.Status, 40),
		})
	}
	return result, nil
}

func (s *Service) configured() bool { return s.config.Enabled && s.err == "" && s.planner != nil }

func validOwner(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n")
}

func validateLocalURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New(urlEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "gateway" && net.ParseIP(host) == nil {
		return errors.New(urlEnv + " must resolve to localhost, host.docker.internal, gateway, or a literal local IP")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return errors.New(urlEnv + " must use a loopback or private-network IP")
	}
	return nil
}

func normalize(value string) string { return strings.Join(strings.Fields(value), " ") }

func bounded(value string, max int) string {
	value = normalize(value)
	if len(value) <= max {
		return value
	}
	if max < 4 {
		return value[:max]
	}
	return strings.TrimSpace(value[:max-3]) + "..."
}

func boundedList(values []string, limit, width int) []string {
	result := make([]string, 0, limit)
	for _, value := range values {
		value = bounded(value, width)
		if value == "" {
			continue
		}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}
