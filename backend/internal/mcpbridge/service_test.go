package mcpbridge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

type githubSourceProvider struct{ sources []models.ConnectedSource }

func (p githubSourceProvider) Sources(bool) ([]models.ConnectedSource, error) { return p.sources, nil }

type modelMaintenanceProvider struct{ records []llm.ModelMaintenanceResult }

func (p modelMaintenanceProvider) ModelMaintenanceHistory(int) ([]llm.ModelMaintenanceResult, error) {
	return p.records, nil
}

type dashboardProvider struct {
	dashboard *workflow.WorkflowDashboard
	owner     string
}

func (d *dashboardProvider) DashboardForOwner(owner string) (*workflow.WorkflowDashboard, error) {
	d.owner = owner
	return d.dashboard, nil
}

func TestMCPBridgeIsDisabledWithoutExplicitOwnerAndToken(t *testing.T) {
	service := NewService(Config{Enabled: false}, &dashboardProvider{})
	if service.Status().Configured || service.Authorize("anything") {
		t.Fatalf("disabled bridge must not be configured or authorized: %#v", service.Status())
	}
	misconfigured := NewService(Config{Enabled: true, Token: "short", OwnerID: "owner"}, &dashboardProvider{})
	if misconfigured.Status().Configured || misconfigured.Status().ConfigError == "" {
		t.Fatalf("short bridge token must fail closed: %#v", misconfigured.Status())
	}
}

func TestMCPBridgeReturnsOnlyBoundedActionableSummary(t *testing.T) {
	first := models.WorkflowItem{ID: uuid.New(), Title: "Send lawyer material", Description: "secret raw description", CurrentState: workflow.StateNeedsApproval, RiskLevel: "high", PriorityScore: 98, RequiresApproval: true, ApprovalStatus: "pending", ApprovalReason: "legal message", NextAction: "ask Robert to approve"}
	second := models.WorkflowItem{ID: uuid.New(), Title: "Clear low-risk admin work", CurrentState: workflow.StateReady, RiskLevel: "low", PriorityScore: 20, NextAction: "run after review"}
	provider := &dashboardProvider{dashboard: &workflow.WorkflowDashboard{
		Counts:        map[string]int64{"approvals": 1, "ready": 1},
		ApprovalItems: []models.WorkflowItem{first},
		ReadyItems:    []models.WorkflowItem{second},
	}}
	token := "12345678901234567890123456789012"
	service := NewService(Config{Enabled: true, Token: token, OwnerID: "robert@example.test"}, provider)
	if !service.Authorize(token) || service.Authorize(token+"x") {
		t.Fatal("bridge authorization must use the configured exact token")
	}
	overview, err := service.Overview()
	if err != nil || overview.Counts["approvals"] != 1 || provider.owner != "robert@example.test" {
		t.Fatalf("unexpected scoped overview: %#v %v", overview, err)
	}
	items, err := service.Actionable(1)
	if err != nil || len(items) != 1 || items[0].ID != first.ID.String() || items[0].Title != first.Title {
		t.Fatalf("unexpected actionable summary: %#v %v", items, err)
	}
	if items[0].NextAction != "ask Robert to approve" || items[0].RequiresApproval != true {
		t.Fatalf("summary lost guarded operational fields: %#v", items[0])
	}
}

func TestMCPBridgeReturnsOnlyOwnerScopedGitHubRepositoryFreshness(t *testing.T) {
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	provider := &dashboardProvider{dashboard: &workflow.WorkflowDashboard{Counts: map[string]int64{}}}
	sources := githubSourceProvider{sources: []models.ConnectedSource{
		{OwnerIdentity: "robert@example.test", ConnectorKey: "github", SyncTarget: "Noodzakelijk-Online/018-HAI", DefaultProjectKey: "018-HAI", Enabled: true, Status: "active", SyncFrequency: "6h", LastSyncedAt: &now},
		{OwnerIdentity: "someone-else", ConnectorKey: "github", SyncTarget: "other/private-repo", Enabled: true, Status: "active"},
		{OwnerIdentity: "robert@example.test", ConnectorKey: "github", SyncTarget: "https://github.com/not-a-slug", Enabled: true, Status: "active"},
		{OwnerIdentity: "robert@example.test", ConnectorKey: "email", SyncTarget: "not-a-repository", Enabled: true, Status: "active"},
	}}
	service := NewService(Config{Enabled: true, Token: "12345678901234567890123456789012", OwnerID: "robert@example.test"}, provider, sources)
	repositories, err := service.GitHubRepositories(8)
	if err != nil || len(repositories) != 1 {
		t.Fatalf("repository context = %#v, %v", repositories, err)
	}
	if got := repositories[0]; got.Repository != "Noodzakelijk-Online/018-HAI" || got.ProjectKey != "018-HAI" || got.LastSyncedAt == nil || !got.LastSyncedAt.Equal(now) {
		t.Fatalf("unexpected bounded context: %#v", got)
	}
}

func TestGitHubRepositorySlugRejectsURLsAndUnsafeValues(t *testing.T) {
	if slug, ok := githubRepositorySlug("owner/repository"); !ok || slug != "owner/repository" {
		t.Fatalf("valid slug = %q, %t", slug, ok)
	}
	for _, value := range []string{"", "owner", "https://github.com/owner/repository", "owner/repo/extra", "owner/repo?token=secret", "owner/repo\nnext"} {
		if _, ok := githubRepositorySlug(value); ok {
			t.Fatalf("unsafe repository slug accepted: %q", value)
		}
	}
}

func TestMCPBridgeReturnsOnlyBoundedModelMaintenanceReadiness(t *testing.T) {
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	next := now.Add(24 * time.Hour)
	provider := &dashboardProvider{dashboard: &workflow.WorkflowDashboard{Counts: map[string]int64{}}}
	maintenance := modelMaintenanceProvider{records: []llm.ModelMaintenanceResult{
		{ProviderID: "ollama", ProviderName: "Ollama", ModelID: "qwen2.5:7b", ModelName: "Qwen", Status: "current", CheckedAt: now, NextCheckDueAt: &next, CurrentDigest: "secret-digest", Reason: "secret maintenance response"},
		{ProviderID: "ollama", ProviderName: "Ollama", ModelID: "qwen2.5:7b", ModelName: "Qwen", Status: "failed", BlocksExecution: true, CheckedAt: now.Add(time.Hour), CurrentDigest: "another-secret", Reason: "private failure details"},
	}}
	service := NewService(Config{Enabled: true, Token: "12345678901234567890123456789012", OwnerID: "robert@example.test"}, provider).WithModelMaintenance(maintenance)
	readiness, err := service.ModelMaintenanceReadiness(8)
	if err != nil || len(readiness) != 1 {
		t.Fatalf("model readiness = %#v, %v", readiness, err)
	}
	if !readiness[0].BlocksExecution || readiness[0].Status != "failed" || !readiness[0].CheckedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("latest blocked maintenance record was not retained: %#v", readiness[0])
	}
	payload, err := json.Marshal(readiness)
	if err != nil || strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "private") {
		t.Fatalf("maintenance readiness leaked detail: %s %v", payload, err)
	}
}
