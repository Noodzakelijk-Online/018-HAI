package autogencompat

import (
	"strings"
	"testing"
)

func TestPreviewMapsOpenLoopsWithoutExecutingOrPersisting(t *testing.T) {
	preview, err := DefaultService().Preview(PreviewRequest{WorkloadID: "legal-review-export", Events: []Event{
		{ID: "1", Type: "message", Agent: "triage", Summary: "Need a source-backed reply."},
		{ID: "2", Type: "tool_call", Agent: "research", CorrelationID: "search-1", Summary: "Search the legal folder using token=secret-value."},
		{ID: "3", Type: "handoff", Agent: "research", CorrelationID: "draft-1", Summary: "Hand off draft review to legal agent."},
		{ID: "4", Type: "task_completed", Agent: "legal", Summary: "Draft completed upstream."},
	}})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.ExecutionAllowed || preview.PersistenceAllowed || !preview.RequiresReview {
		t.Fatalf("preview must be review-only: %#v", preview)
	}
	loopKinds := map[string]bool{}
	for _, loop := range preview.OpenLoops {
		loopKinds[loop.Kind] = true
	}
	if len(preview.OpenLoops) != 2 || !loopKinds["unresolved_handoff"] || !loopKinds["unverified_tool_call"] {
		t.Fatalf("open loops = %#v", preview.OpenLoops)
	}
	if !strings.Contains(preview.Events[1].Summary, "[REDACTED]") || strings.Contains(preview.Events[1].Summary, "secret-value") {
		t.Fatalf("event summary was not redacted: %#v", preview.Events[1])
	}
	if !strings.Contains(preview.CompletionVerification, "No imported event") || len(preview.RecommendedControls) < 3 {
		t.Fatalf("control mapping = %#v", preview)
	}
}

func TestPreviewRejectsUnknownOrOversizedEvents(t *testing.T) {
	service := DefaultService()
	for _, request := range []PreviewRequest{
		{WorkloadID: "workload", Events: []Event{{ID: "1", Type: "tool_execute", Summary: "unsupported"}}},
		{WorkloadID: "workload", Events: []Event{{ID: "1", Type: "message", Summary: strings.Repeat("x", maxSummaryRunes+1)}}},
		{WorkloadID: "", Events: []Event{{ID: "1", Type: "message", Summary: "missing workload"}}},
	} {
		if _, err := service.Preview(request); err == nil {
			t.Fatalf("Preview(%#v) succeeded", request)
		}
	}
}

func TestMigrationPlanMapsControlsWithoutStartingAgentFramework(t *testing.T) {
	plan, err := DefaultService().MigrationPlan(MigrationRequest{
		Target: "microsoft_agent_framework",
		PreviewRequest: PreviewRequest{WorkloadID: "legacy-team", Events: []Event{
			{ID: "1", Type: "message", Summary: "Assess intake."},
			{ID: "2", Type: "tool_call", CorrelationID: "search", Summary: "Search a folder."},
			{ID: "3", Type: "task_completed", Summary: "Reported done upstream."},
		}},
	})
	if err != nil {
		t.Fatalf("MigrationPlan: %v", err)
	}
	if plan.Target != "microsoft-agent-framework" || plan.ExecutionAllowed || plan.FrameworkRuntimeDetected {
		t.Fatalf("plan must be a non-executing migration plan: %#v", plan)
	}
	if len(plan.Steps) != 4 || len(plan.BlockedUntil) < 3 {
		t.Fatalf("plan steps = %#v", plan)
	}
	if got := plan.Steps[2].RequiredEvents; len(got) != 1 || got[0] != "tool_call" {
		t.Fatalf("runtime mapping = %#v", got)
	}
	if !strings.Contains(plan.Scope, "did not install") {
		t.Fatalf("scope must state the no-install boundary: %q", plan.Scope)
	}
}

func TestMigrationPlanRejectsAnyOtherTarget(t *testing.T) {
	_, err := DefaultService().MigrationPlan(MigrationRequest{
		Target:         "autogen",
		PreviewRequest: PreviewRequest{WorkloadID: "legacy", Events: []Event{{ID: "1", Type: "message", Summary: "hello"}}},
	})
	if err == nil {
		t.Fatal("expected non-Agent-Framework target to be rejected")
	}
}
