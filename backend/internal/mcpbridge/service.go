// Package mcpbridge exposes a small, read-only context surface for a reviewed
// local MCP server. It is deliberately separate from HAI's normal user API:
// an MCP client gets a distinct, explicit token and cannot use this bridge to
// create, approve, execute, retrieve sources, or alter memory.
package mcpbridge

import (
	"crypto/subtle"
	"errors"
	"os"
	"sort"
	"strings"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"
	"github.com/google/uuid"
)

const (
	enabledEnv = "HAI_FASTMCP_BRIDGE_ENABLED"
	tokenEnv   = "HAI_FASTMCP_BRIDGE_TOKEN"
	ownerEnv   = "HAI_FASTMCP_BRIDGE_OWNER_ID"
)

var ErrUnavailable = errors.New("local MCP bridge is not configured")

// Config contains only the minimum server-to-server authority. The client
// bearer token is owned by the FastMCP process and never reaches this API.
type Config struct {
	Enabled bool
	Token   string
	OwnerID string
}

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type Overview struct {
	Counts map[string]int64 `json:"counts"`
	Scope  string           `json:"scope"`
}

// ActionableWorkflow is intentionally smaller than a workflow record. It has
// no description, source URI, sender, evidence, raw intake, audit log, or
// credential-bearing field.
type ActionableWorkflow struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	ProjectKey         string `json:"projectKey,omitempty"`
	CurrentState       string `json:"currentState"`
	RiskLevel          string `json:"riskLevel"`
	PriorityScore      int    `json:"priorityScore"`
	AutonomyLevel      string `json:"autonomyLevel"`
	RequiresApproval   bool   `json:"requiresApproval"`
	ApprovalStatus     string `json:"approvalStatus,omitempty"`
	ApprovalReason     string `json:"approvalReason,omitempty"`
	BlockedReason      string `json:"blockedReason,omitempty"`
	NextAction         string `json:"nextAction,omitempty"`
	VerificationStatus string `json:"verificationStatus,omitempty"`
}

// DashboardProvider is deliberately limited to read-only workflow summaries.
type DashboardProvider interface {
	DashboardForOwner(ownerIdentity string) (*workflow.WorkflowDashboard, error)
}

type Service struct {
	config Config
	flows  DashboardProvider
	err    string
}

func DefaultConfig() Config {
	return Config{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		Token:   strings.TrimSpace(os.Getenv(tokenEnv)),
		OwnerID: strings.TrimSpace(os.Getenv(ownerEnv)),
	}
}

func NewService(config Config, flows DashboardProvider) *Service {
	s := &Service{config: config, flows: flows}
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
	if flows == nil {
		s.err = "workflow dashboard provider is unavailable"
	}
	return s
}

func NewServiceFromEnv(flows DashboardProvider) *Service { return NewService(DefaultConfig(), flows) }

func (s *Service) Status() Status {
	return Status{
		Enabled:     s.config.Enabled,
		Configured:  s.configured(),
		Provider:    "FastMCP local read-only HAI bridge",
		ConfigError: s.err,
		Capabilities: []string{
			"authenticated aggregate workflow overview",
			"authenticated bounded actionable-work summary",
			"separate server-to-server and MCP-client tokens",
		},
		Restrictions: []string{
			"no task creation, workflow transition, approval, execution, source retrieval, memory write, or policy change",
			"no raw intake, source URI, evidence, audit event, credential, or secret exposure",
			"disabled unless an owner, a 32-character bridge token, and a separately configured MCP client token are supplied",
		},
		Scope: "A local FastMCP server may inspect one configured owner's bounded operational summary. HAI remains the only authority for planning, approval, execution, sources, memory, and audit.",
	}
}

func (s *Service) Authorize(token string) bool {
	if !s.configured() || len(token) != len(s.config.Token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

func (s *Service) Overview() (*Overview, error) {
	if !s.configured() {
		return nil, ErrUnavailable
	}
	dashboard, err := s.flows.DashboardForOwner(s.config.OwnerID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(dashboard.Counts))
	for key, value := range dashboard.Counts {
		counts[key] = value
	}
	return &Overview{Counts: counts, Scope: s.Status().Scope}, nil
}

func (s *Service) Actionable(limit int) ([]ActionableWorkflow, error) {
	if !s.configured() {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 8 {
		limit = 8
	}
	dashboard, err := s.flows.DashboardForOwner(s.config.OwnerID)
	if err != nil {
		return nil, err
	}
	items := map[string]ActionableWorkflow{}
	for _, group := range [][]models.WorkflowItem{
		dashboard.ApprovalItems,
		dashboard.BlockedItems,
		dashboard.ReadyItems,
		dashboard.HighRiskItems,
		dashboard.ItemsWithoutNextAction,
	} {
		for _, item := range group {
			if item.ID == uuid.Nil {
				continue
			}
			items[item.ID.String()] = ActionableWorkflow{
				ID: item.ID.String(), Title: bounded(item.Title, 180), ProjectKey: bounded(item.ProjectKey, 120),
				CurrentState: bounded(item.CurrentState, 80), RiskLevel: bounded(item.RiskLevel, 40), PriorityScore: item.PriorityScore,
				AutonomyLevel: bounded(item.AutonomyLevel, 80), RequiresApproval: item.RequiresApproval,
				ApprovalStatus: bounded(item.ApprovalStatus, 80), ApprovalReason: bounded(item.ApprovalReason, 240),
				BlockedReason: bounded(item.BlockedReason, 240), NextAction: bounded(item.NextAction, 240),
				VerificationStatus: bounded(item.VerificationStatus, 80),
			}
		}
	}
	result := make([]ActionableWorkflow, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PriorityScore == result[j].PriorityScore {
			return result[i].ID < result[j].ID
		}
		return result[i].PriorityScore > result[j].PriorityScore
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Service) configured() bool { return s.config.Enabled && s.err == "" && s.flows != nil }

func validOwner(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n")
}

func bounded(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	if max < 4 {
		return value[:max]
	}
	return strings.TrimSpace(value[:max-3]) + "..."
}
