// Package mcpbridge exposes a small, read-only context surface for a reviewed
// local MCP server. It is deliberately separate from HAI's normal user API:
// an MCP client gets a distinct, explicit token and cannot use this bridge to
// create, approve, execute, retrieve source content, or alter memory.
package mcpbridge

import (
	"crypto/subtle"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/llm"
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

// GitHubRepositoryContext is intentionally a configuration-and-freshness
// summary, not repository content. It gives a reviewed local MCP client enough
// context to ask HAI's owner-facing screens the right question without
// disclosing an issue, pull request, file, commit, source URI, or credential.
type GitHubRepositoryContext struct {
	Repository    string     `json:"repository"`
	ProjectKey    string     `json:"projectKey,omitempty"`
	Enabled       bool       `json:"enabled"`
	Status        string     `json:"status"`
	SyncFrequency string     `json:"syncFrequency"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt,omitempty"`
}

// ModelMaintenanceContext is a bounded freshness signal for a local MCP
// client. It intentionally omits endpoint, digest, token, prompt, output,
// cost, quota, and provider payload data. The policy router remains the only
// authority that can route, refresh, or use a model.
type ModelMaintenanceContext struct {
	ProviderID      string     `json:"providerId"`
	ProviderName    string     `json:"providerName"`
	ModelID         string     `json:"modelId"`
	ModelName       string     `json:"modelName"`
	Status          string     `json:"status"`
	BlocksExecution bool       `json:"blocksExecution"`
	CheckedAt       time.Time  `json:"checkedAt"`
	NextCheckDueAt  *time.Time `json:"nextCheckDueAt,omitempty"`
}

// DashboardProvider is deliberately limited to read-only workflow summaries.
type DashboardProvider interface {
	DashboardForOwner(ownerIdentity string) (*workflow.WorkflowDashboard, error)
}

// GitHubSourceProvider intentionally exposes only the existing source
// registry. The bridge maps its records into GitHubRepositoryContext itself;
// it does not receive extraction, raw-item, OAuth-token, sync-job, or search
// methods.
type GitHubSourceProvider interface {
	Sources(includeDisabled bool) ([]models.ConnectedSource, error)
}

// ModelMaintenanceProvider exposes persisted aggregate freshness evidence,
// rather than provider configuration or raw maintenance responses.
type ModelMaintenanceProvider interface {
	ModelMaintenanceHistory(limit int) ([]llm.ModelMaintenanceResult, error)
}

type Service struct {
	config        Config
	flows         DashboardProvider
	githubSources GitHubSourceProvider
	maintenance   ModelMaintenanceProvider
	err           string
}

func DefaultConfig() Config {
	return Config{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		Token:   strings.TrimSpace(os.Getenv(tokenEnv)),
		OwnerID: strings.TrimSpace(os.Getenv(ownerEnv)),
	}
}

func NewService(config Config, flows DashboardProvider, githubSources ...GitHubSourceProvider) *Service {
	s := &Service{config: config, flows: flows}
	if len(githubSources) > 0 {
		s.githubSources = githubSources[0]
	}
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

func NewServiceFromEnv(flows DashboardProvider, githubSources ...GitHubSourceProvider) *Service {
	return NewService(DefaultConfig(), flows, githubSources...)
}

// WithModelMaintenance adds the existing HAI-owned history reader to the
// bridge. It does not enable the bridge, grant a model operation, or expose
// provider configuration.
func (s *Service) WithModelMaintenance(provider ModelMaintenanceProvider) *Service {
	s.maintenance = provider
	return s
}

func (s *Service) Status() Status {
	return Status{
		Enabled:     s.config.Enabled,
		Configured:  s.configured(),
		Provider:    "FastMCP local read-only HAI bridge",
		ConfigError: s.err,
		Capabilities: []string{
			"authenticated aggregate workflow overview",
			"authenticated bounded actionable-work summary",
			"authenticated bounded GitHub repository sync context",
			"authenticated bounded daily model-maintenance readiness",
			"separate server-to-server and MCP-client tokens",
		},
		Restrictions: []string{
			"no task creation, workflow transition, approval, execution, source-content retrieval, memory write, policy change, model refresh, model route, or model generation",
			"no raw intake, source URI, issue, pull request, commit, file, evidence, audit event, credential, secret, provider endpoint, model digest, prompt, completion, token, quota, or cost exposure",
			"disabled unless an owner, a 32-character bridge token, and a separately configured MCP client token are supplied",
		},
		Scope: "A local FastMCP server may inspect one configured owner's bounded operational summary, GitHub sync freshness, and persisted model-maintenance readiness. HAI remains the only authority for model maintenance, routing, planning, approval, execution, source content, memory, and audit.",
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

// GitHubRepositories returns a small owner-scoped inventory of HAI's existing
// GitHub source connections. It deliberately returns only repository slugs and
// sync freshness; repository data stays behind HAI's normal source APIs.
func (s *Service) GitHubRepositories(limit int) ([]GitHubRepositoryContext, error) {
	if !s.configured() || s.githubSources == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 8 {
		limit = 8
	}
	sources, err := s.githubSources.Sources(true)
	if err != nil {
		return nil, err
	}
	result := make([]GitHubRepositoryContext, 0, limit)
	for _, source := range sources {
		if strings.TrimSpace(source.OwnerIdentity) != s.config.OwnerID || !strings.EqualFold(strings.TrimSpace(source.ConnectorKey), "github") {
			continue
		}
		repository, ok := githubRepositorySlug(source.SyncTarget)
		if !ok {
			continue
		}
		result = append(result, GitHubRepositoryContext{
			Repository: repository, ProjectKey: bounded(source.DefaultProjectKey, 120), Enabled: source.Enabled,
			Status: bounded(source.Status, 80), SyncFrequency: bounded(source.SyncFrequency, 80), LastSyncedAt: source.LastSyncedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].LastSyncedAt, result[j].LastSyncedAt
		if left == nil && right != nil {
			return false
		}
		if left != nil && right == nil {
			return true
		}
		if left != nil && right != nil && !left.Equal(*right) {
			return left.After(*right)
		}
		return result[i].Repository < result[j].Repository
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// ModelMaintenanceReadiness returns at most eight latest per-model freshness
// records. It is informational only: an MCP client cannot use it to override
// the policy router, run a refresh, or treat a record as model quality proof.
func (s *Service) ModelMaintenanceReadiness(limit int) ([]ModelMaintenanceContext, error) {
	if !s.configured() || s.maintenance == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 8 {
		limit = 8
	}
	records, err := s.maintenance.ModelMaintenanceHistory(64)
	if err != nil {
		return nil, err
	}
	latest := map[string]llm.ModelMaintenanceResult{}
	for _, record := range records {
		key := record.ProviderID + "\x00" + record.ModelID
		current, ok := latest[key]
		if !ok || record.CheckedAt.After(current.CheckedAt) {
			latest[key] = record
		}
	}
	result := make([]ModelMaintenanceContext, 0, len(latest))
	for _, record := range latest {
		result = append(result, ModelMaintenanceContext{
			ProviderID: bounded(record.ProviderID, 80), ProviderName: bounded(record.ProviderName, 120),
			ModelID: bounded(record.ModelID, 160), ModelName: bounded(record.ModelName, 160),
			Status: bounded(record.Status, 80), BlocksExecution: record.BlocksExecution,
			CheckedAt: record.CheckedAt, NextCheckDueAt: record.NextCheckDueAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].BlocksExecution != result[j].BlocksExecution {
			return result[i].BlocksExecution
		}
		if result[i].NextCheckDueAt == nil && result[j].NextCheckDueAt != nil {
			return false
		}
		if result[i].NextCheckDueAt != nil && result[j].NextCheckDueAt == nil {
			return true
		}
		if result[i].NextCheckDueAt != nil && result[j].NextCheckDueAt != nil && !result[i].NextCheckDueAt.Equal(*result[j].NextCheckDueAt) {
			return result[i].NextCheckDueAt.Before(*result[j].NextCheckDueAt)
		}
		return result[i].ModelID < result[j].ModelID
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

func githubRepositorySlug(value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || !validGitHubSlugPart(parts[0]) || !validGitHubSlugPart(parts[1]) {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

func validGitHubSlugPart(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 100 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}
