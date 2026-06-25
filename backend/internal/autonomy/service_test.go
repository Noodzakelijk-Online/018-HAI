package autonomy

import (
	"testing"
	"time"

	"automation-hub-backend/internal/models"
)

func TestValidateActionGuards(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		action ActionEnvelope
		want   string
	}{
		{
			name:   "approval",
			action: ActionEnvelope{InterfaceType: InterfaceToolCall, ActionType: "send_email", RequiresApproval: true, StaleAfter: now.Add(time.Minute)},
			want:   "blocked_approval",
		},
		{
			name:   "stale",
			action: ActionEnvelope{InterfaceType: InterfaceSkillCall, ActionType: "classify", StaleAfter: now.Add(-time.Second)},
			want:   "blocked_stale_state",
		},
		{
			name:   "prompt injection",
			action: ActionEnvelope{InterfaceType: InterfaceToolCall, ActionType: "read", StaleAfter: now.Add(time.Minute), UntrustedInput: true, PolicyOverride: true},
			want:   "blocked_prompt_injection",
		},
		{
			name:   "allowed",
			action: ActionEnvelope{InterfaceType: InterfaceActionChunk, ActionType: "checklist", StaleAfter: now.Add(time.Minute)},
			want:   "allowed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidateAction(test.action, now); got != test.want {
				t.Fatalf("ValidateAction() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAggregateSeparatesRawAndPolicyCompletion(t *testing.T) {
	result := aggregate([]models.AutonomyEvaluation{
		{RawCompletion: true, CompletionUnderPolicy: true, RecoveryAttempt: true, Recovered: true, LatencyMilliseconds: 100},
		{RawCompletion: true, CompletionUnderPolicy: false, RiskViolation: true, HumanIntervention: true, LatencyMilliseconds: 300},
	})
	if result.RawCompletions != 2 || result.CompletionUnderPolicy != 1 {
		t.Fatalf("unexpected completion metrics: %+v", result)
	}
	if result.AverageLatencyMillis != 200 || result.RecoveryRate != 1 {
		t.Fatalf("unexpected derived metrics: %+v", result)
	}
}
