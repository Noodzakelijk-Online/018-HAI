package temporalbridge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestStatusKeepsTemporalDisabledUntilExplicitlyEnabled(t *testing.T) {
	service := NewService(nil, nil, false, "", "", "")
	status := service.Status()
	if status.Enabled || status.Configured || status.WorkerStarted {
		t.Fatalf("disabled Temporal status = %+v", status)
	}
	if status.ConfigError != "" {
		t.Fatalf("disabled Temporal should not report a configuration error: %q", status.ConfigError)
	}
}

func TestLocalConfigRejectsRemoteTemporalEndpoint(t *testing.T) {
	service := NewService(nil, nil, true, "agents.example.test:7233", "default", "hai-governed-follow-up")
	status := service.Status()
	if status.Configured || status.ConfigError == "" {
		t.Fatalf("remote Temporal endpoint must be rejected: %+v", status)
	}
}

func TestGovernedFollowUpWorkflowOnlyInvokesNamedActivity(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	started := time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)
	env.SetStartTime(started)
	env.RegisterActivityWithOptions(func(_ context.Context, _ FollowUpInput) (FollowUpResult, error) {
		return FollowUpResult{Checked: 2, Triggered: 1, Summary: "proposal-only check completed"}, nil
	}, activity.RegisterOptions{Name: "Run"})

	env.ExecuteWorkflow(GovernedFollowUpWorkflow, FollowUpInput{RunID: "00000000-0000-0000-0000-000000000001", RunAt: started, Limit: 5})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var result FollowUpResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, 2, result.Checked)
	require.Equal(t, 1, result.Triggered)
}

func TestValidateRequestRejectsOutOfRangeSchedule(t *testing.T) {
	now := time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)
	if err := validateRequest(now, FollowUpRequest{RunAt: now.Add(366 * 24 * time.Hour), Limit: 1}); err == nil {
		t.Fatal("expected schedule beyond the bounded horizon to be rejected")
	}
	if err := validateRequest(now, FollowUpRequest{RunAt: now.Add(time.Minute), Limit: maxFollowUpLimit + 1}); err == nil {
		t.Fatal("expected excessive follow-up limit to be rejected")
	}
}
