package agentcoordination

import (
	"testing"
	"time"
)

func TestEvaluateMessageTimeoutProgressesFromWaitToReminderToEscalation(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	message := validMessage(t, now)
	policy := EscalationPolicy{
		AcknowledgmentTimeout: 5 * time.Minute,
		CompletionTimeout:     time.Hour,
		ReminderInterval:      5 * time.Minute,
		MaximumReminders:      2,
		MaximumEscalations:    1,
		EscalationRecipients:  []string{"human-operator"},
	}
	evaluation, err := EvaluateMessageTimeout(message, nil, policy, 0, 0, now.Add(time.Minute))
	if err != nil || evaluation.Action != TimeoutNone {
		t.Fatalf("initial evaluation = %#v, %v", evaluation, err)
	}
	evaluation, err = EvaluateMessageTimeout(message, nil, policy, 0, 0, now.Add(6*time.Minute))
	if err != nil || evaluation.Action != TimeoutRemind {
		t.Fatalf("reminder evaluation = %#v, %v", evaluation, err)
	}
	evaluation, err = EvaluateMessageTimeout(message, nil, policy, 2, 0, now.Add(20*time.Minute))
	if err != nil || evaluation.Action != TimeoutEscalate {
		t.Fatalf("escalation evaluation = %#v, %v", evaluation, err)
	}
	evaluation, err = EvaluateMessageTimeout(message, nil, policy, 2, 1, now.Add(25*time.Minute))
	if err != nil || evaluation.Action != TimeoutManualReview {
		t.Fatalf("manual review evaluation = %#v, %v", evaluation, err)
	}
}

func TestEvaluateDelegationTimeoutDoesNotEscalateTerminalWork(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	delegation := validDelegation(now)
	delegation.Status = DelegationCompleted
	evaluation, err := EvaluateDelegationTimeout(
		delegation,
		EscalationPolicy{
			AcknowledgmentTimeout: time.Minute,
			CompletionTimeout:     time.Minute,
			ReminderInterval:      time.Minute,
			MaximumReminders:      1,
			MaximumEscalations:    1,
			EscalationRecipients:  []string{"human-operator"},
		},
		0,
		now.Add(time.Hour),
	)
	if err != nil || evaluation.Action != TimeoutNone {
		t.Fatalf("terminal delegation evaluation = %#v, %v", evaluation, err)
	}
}
