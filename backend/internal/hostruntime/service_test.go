package hostruntime

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceLeasesOneApprovedJobAndRejectsStaleCompletion(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	service := NewService(repository, WithClock(func() time.Time { return now }))

	job, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test",
		RuntimeID:     "deepseek-harness",
		TaskID:        "task-1",
		Prompt:        "Summarize the approved local workspace changes.",
		WorkspaceKey:  "hai",
		Approved:      true,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.Status != StatusPending {
		t.Fatalf("new job status = %q, want %q", job.Status, StatusPending)
	}

	lease, err := service.Lease("bridge-01", "deepseek-harness")
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if lease == nil || lease.Job.ID != job.ID || lease.Token == "" {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	if second, err := service.Lease("bridge-02", "deepseek-harness"); err != nil || second != nil {
		t.Fatalf("second lease = %#v, %v; want no job", second, err)
	}

	if _, err := service.Complete("bridge-01", job.ID, "wrong-token", Completion{ExitCode: 0, Output: "done"}); err == nil {
		t.Fatal("stale lease token completed host job")
	}
	completed, err := service.Complete("bridge-01", job.ID, lease.Token, Completion{ExitCode: 0, Output: "done"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if completed.Status != StatusCompleted || completed.Output != "done" || completed.WorkerID != "bridge-01" {
		t.Fatalf("unexpected completed job: %#v", completed)
	}
}

func TestServiceRejectsUnapprovedAndRedactsResult(t *testing.T) {
	service := NewService(newMemoryRepository())
	if _, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test",
		RuntimeID:     "deepseek-harness",
		TaskID:        "task-1",
		Prompt:        "Inspect workspace.",
		WorkspaceKey:  "hai",
	}); err == nil {
		t.Fatal("unapproved job was accepted")
	}

	job, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test",
		RuntimeID:     "deepseek-harness",
		TaskID:        "task-2",
		Prompt:        "Inspect workspace.",
		WorkspaceKey:  "hai",
		Approved:      true,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	lease, err := service.Lease("bridge-01", "deepseek-harness")
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	completed, err := service.Complete("bridge-01", job.ID, lease.Token, Completion{
		ExitCode: 1,
		Output:   "Authorization: Bearer secret-value",
		Error:    strings.Repeat("x", 20_000),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if strings.Contains(completed.Output, "secret-value") || len(completed.Error) > maxResultBytes {
		t.Fatalf("completion was not safely bounded/redacted: %#v", completed)
	}
}

func TestServiceReclaimsExpiredLeaseAndRejectsTheOldToken(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	service := NewService(
		newMemoryRepository(),
		WithClock(func() time.Time { return now }),
		WithLeaseDuration(5*time.Minute),
	)
	job, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-reclaimed",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	first, err := service.Lease("bridge-old", "deepseek-harness")
	if err != nil || first == nil {
		t.Fatalf("first lease = %#v, %v", first, err)
	}
	now = now.Add(6 * time.Minute)
	second, err := service.Lease("bridge-new", "deepseek-harness")
	if err != nil || second == nil || second.Job.ID != job.ID || second.Token == first.Token {
		t.Fatalf("reclaimed lease = %#v, %v", second, err)
	}
	if _, err := service.Complete("bridge-old", job.ID, first.Token, Completion{ExitCode: 0}); err != ErrStaleLease {
		t.Fatalf("old lease completion error = %v, want %v", err, ErrStaleLease)
	}
}

func TestServiceEmergencyStopPreventsQueuedJobLease(t *testing.T) {
	service := NewService(newMemoryRepository())
	if _, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-stopped",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	if lease, err := service.Lease("bridge-01", "deepseek-harness"); !errors.Is(err, ErrEmergencyStopped) || lease != nil {
		t.Fatalf("Lease while stopped = %#v, %v; want no lease and emergency-stop error", lease, err)
	}
	t.Setenv("HAI_EMERGENCY_STOP", "false")
	if lease, err := service.Lease("bridge-01", "deepseek-harness"); err != nil || lease == nil {
		t.Fatalf("Lease after stop cleared = %#v, %v; want queued job", lease, err)
	}
}

func TestServiceConfirmLeaseChecksEmergencyStopAndTokenFreshness(t *testing.T) {
	service := NewService(newMemoryRepository())
	job, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-confirm",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	lease, err := service.Lease("bridge-01", "deepseek-harness")
	if err != nil || lease == nil {
		t.Fatalf("Lease = %#v, %v", lease, err)
	}
	if err := service.ConfirmLease("bridge-01", job.ID, "wrong-token"); !errors.Is(err, ErrStaleLease) {
		t.Fatalf("ConfirmLease wrong token = %v, want stale lease", err)
	}
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	if err := service.ConfirmLease("bridge-01", job.ID, lease.Token); !errors.Is(err, ErrEmergencyStopped) {
		t.Fatalf("ConfirmLease while stopped = %v, want emergency stop", err)
	}
	t.Setenv("HAI_EMERGENCY_STOP", "false")
	if err := service.ConfirmLease("bridge-01", job.ID, lease.Token); err != nil {
		t.Fatalf("ConfirmLease valid lease: %v", err)
	}
}

func TestConfiguredLeaseOutlivesMaximumAllowedHarnessRun(t *testing.T) {
	t.Setenv("DEEPSEEK_HARNESS_TIMEOUT_SECONDS", "900")
	t.Setenv("HAI_HOST_RUNTIME_LEASE_SECONDS", "60")
	if got, want := configuredLeaseDuration(), 16*time.Minute; got != want {
		t.Fatalf("configuredLeaseDuration = %s, want %s", got, want)
	}

	t.Setenv("HAI_HOST_RUNTIME_LEASE_SECONDS", "1200")
	if got, want := configuredLeaseDuration(), 20*time.Minute; got != want {
		t.Fatalf("configuredLeaseDuration = %s, want %s", got, want)
	}
}
