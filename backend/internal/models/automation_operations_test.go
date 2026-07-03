package models

import "testing"

func TestAutomationLaunchEventAuditEventsRoundTrip(t *testing.T) {
	event := &AutomationLaunchEvent{
		AuditEvents: []string{
			"launch requested",
			"runtime registry policy blocked execution",
		},
		RuntimeRouteTrace: &AutomationRuntimeRouteTrace{
			RuntimeID:         "openclaw",
			Intent:            "software engineering and repository workflow",
			ExecutionMode:     "read-only planning plus approved low-risk local actions",
			RiskLevel:         "medium",
			RecommendedSkills: []string{"autoreview", "gitcrawl"},
			VisibleProviders:  []string{"ollama"},
			VisibleTools:      []string{"browser"},
			BlockedSurfaces:   []string{"outbound message sending"},
		},
	}
	if err := event.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave: %v", err)
	}
	if event.AuditLog == "" {
		t.Fatalf("expected audit log to be encoded")
	}
	if event.RuntimeRouteTraceLog == "" {
		t.Fatalf("expected runtime route trace to be encoded")
	}

	loaded := &AutomationLaunchEvent{AuditLog: event.AuditLog, RuntimeRouteTraceLog: event.RuntimeRouteTraceLog}
	if err := loaded.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind: %v", err)
	}
	if len(loaded.AuditEvents) != 2 || loaded.AuditEvents[1] != "runtime registry policy blocked execution" {
		t.Fatalf("audit events did not round-trip: %#v", loaded.AuditEvents)
	}
	if loaded.RuntimeRouteTrace == nil || loaded.RuntimeRouteTrace.RuntimeID != "openclaw" || loaded.RuntimeRouteTrace.RecommendedSkills[1] != "gitcrawl" {
		t.Fatalf("runtime route trace did not round-trip: %#v", loaded.RuntimeRouteTrace)
	}
}
