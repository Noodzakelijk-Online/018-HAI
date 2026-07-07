package auditevent

import (
	"testing"
	"time"
)

func TestNewNormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("X", 3600)
	e := New(time.Date(2026, 1, 1, 12, 0, 0, 0, loc), "operator", "approve", "workflow/1", "success")
	if e.At.Location() != time.UTC {
		t.Fatalf("timestamp not normalized to UTC")
	}
	if e.Actor != "operator" || e.Action != "approve" {
		t.Fatalf("fields wrong: %+v", e)
	}
}

func TestSensitiveDetailsAreRedacted(t *testing.T) {
	e := New(time.Now(), "a", "b", "c", "ok").
		WithDetail("reason", "manual approval").
		WithDetail("api_token", "sk-secret-value").
		WithDetail("Authorization", "Bearer abc")

	if e.Details["reason"] != "manual approval" {
		t.Fatalf("non-sensitive detail should be preserved")
	}
	if e.Details["api_token"] != redacted || e.Details["Authorization"] != redacted {
		t.Fatalf("sensitive details not redacted: %+v", e.Details)
	}
}
