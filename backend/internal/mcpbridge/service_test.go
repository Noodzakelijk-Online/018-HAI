package mcpbridge

import (
	"testing"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

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
