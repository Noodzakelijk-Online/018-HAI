package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunProbesReportsReachableDependencyAsOK(t *testing.T) {
	checks := RunProbes(context.Background(), time.Second, []Probe{
		{Name: "database.connection", Critical: true, Run: func(context.Context) error { return nil }},
	})
	if len(checks) != 1 {
		t.Fatalf("got %d checks, want 1", len(checks))
	}
	if checks[0].Severity != SeverityOK {
		t.Fatalf("severity = %q, want ok (detail: %s)", checks[0].Severity, checks[0].Detail)
	}
}

// A failing critical dependency must be a hard failure, because that is what
// drives /readyz to 503 and takes the process out of rotation.
func TestRunProbesCriticalFailureIsFail(t *testing.T) {
	checks := RunProbes(context.Background(), time.Second, []Probe{
		{Name: "database.connection", Critical: true, Run: func(context.Context) error {
			return errors.New("connection refused")
		}},
	})
	if checks[0].Severity != SeverityFail {
		t.Fatalf("severity = %q, want fail", checks[0].Severity)
	}
	if !strings.Contains(checks[0].Detail, "connection refused") {
		t.Fatalf("detail = %q, want it to carry the underlying cause", checks[0].Detail)
	}
}

// Kafka and the LLM provider are degradations, not outages: the service still
// serves without them, so they must not make it unready.
func TestRunProbesNonCriticalFailureIsWarn(t *testing.T) {
	checks := RunProbes(context.Background(), time.Second, []Probe{
		{Name: "kafka.connection", Critical: false, Run: func(context.Context) error {
			return errors.New("no reachable brokers")
		}},
	})
	if checks[0].Severity != SeverityWarn {
		t.Fatalf("severity = %q, want warn", checks[0].Severity)
	}
}

// A hung dependency must not hang readiness itself.
func TestRunProbesTimesOutInsteadOfBlocking(t *testing.T) {
	start := time.Now()
	checks := RunProbes(context.Background(), 50*time.Millisecond, []Probe{
		{Name: "database.connection", Critical: true, Run: func(ctx context.Context) error {
			<-ctx.Done() // dependency never answers
			return ctx.Err()
		}},
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("RunProbes took %s, want it bounded by the timeout", elapsed)
	}
	if checks[0].Severity != SeverityFail {
		t.Fatalf("severity = %q, want fail on timeout", checks[0].Severity)
	}
}

// A probe that ignores its context must not hang readiness or leak the caller.
func TestRunProbesTimesOutEvenWhenProbeIgnoresContext(t *testing.T) {
	start := time.Now()
	checks := RunProbes(context.Background(), 50*time.Millisecond, []Probe{
		{Name: "stubborn", Critical: true, Run: func(context.Context) error {
			time.Sleep(2 * time.Second)
			return nil
		}},
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("RunProbes took %s, want it to give up at the timeout", elapsed)
	}
	if checks[0].Severity != SeverityFail {
		t.Fatalf("severity = %q, want fail", checks[0].Severity)
	}
}

func TestRunProbesPreservesOrderAcrossConcurrentProbes(t *testing.T) {
	checks := RunProbes(context.Background(), time.Second, []Probe{
		{Name: "first", Run: func(context.Context) error { time.Sleep(30 * time.Millisecond); return nil }},
		{Name: "second", Run: func(context.Context) error { return nil }},
		{Name: "third", Run: func(context.Context) error { time.Sleep(10 * time.Millisecond); return nil }},
	})
	for i, want := range []string{"first", "second", "third"} {
		if checks[i].Name != want {
			t.Fatalf("checks[%d].Name = %q, want %q", i, checks[i].Name, want)
		}
	}
}

func TestMergeCombinesConfigAndLiveChecks(t *testing.T) {
	configured := Report{Checks: []Check{{Name: "database.host", Severity: SeverityOK}}}
	merged := configured.Merge(Check{Name: "database.connection", Severity: SeverityFail})

	if len(merged.Checks) != 2 {
		t.Fatalf("got %d checks, want 2", len(merged.Checks))
	}
	if !merged.HasFailures() {
		t.Fatal("merged report should surface the live failure")
	}
	if configured.HasFailures() {
		t.Fatal("Merge must not mutate the receiver")
	}
}

// The exact scenario observed on the running stack: every DB_* value is a
// valid non-empty string, yet Postgres is down. Configuration alone says
// "ready"; only the live probe tells the truth.
func TestValidConfigWithDeadDatabaseIsNotReady(t *testing.T) {
	configured := Diagnose(healthyConfig())
	if configured.HasFailures() {
		t.Fatal("precondition: the configuration itself is valid")
	}

	live := RunProbes(context.Background(), time.Second, []Probe{
		{Name: "database.connection", Critical: true, Run: func(context.Context) error {
			return errors.New("dial tcp 172.18.0.4:5432: connect: connection refused")
		}},
	})

	if !configured.Merge(live...).HasFailures() {
		t.Fatal("a process with an unreachable database must not report itself ready")
	}
}

func TestIsPlaceholderSecret(t *testing.T) {
	placeholders := []string{
		"change-this-locally-please",
		"CHANGE-THIS",
		"changeme",
		"your-secret-here",
		"replace-me",
		"placeholder-key",
		"TODO",
		"xxxxx",
	}
	for _, value := range placeholders {
		if !IsPlaceholderSecret(value) {
			t.Errorf("IsPlaceholderSecret(%q) = false, want true", value)
		}
	}

	real := []string{
		"9f2b7c1ad4e83f5061bc9e7a2d4f8b13c6e05a97fd2b4e18c73a9056be1f2d4c",
		"s3cure-random-value",
		"",
	}
	for _, value := range real {
		if IsPlaceholderSecret(value) {
			t.Errorf("IsPlaceholderSecret(%q) = true, want false", value)
		}
	}
}

// A shipped placeholder is a default credential. Reporting it as OK is how it
// reaches production, so in production it is a failure, not a note.
func TestDiagnoseFlagsPlaceholderSecrets(t *testing.T) {
	cfg := healthyConfig()
	cfg.BackendAPIKey = "change-this-locally"
	cfg.RunMode = "production"

	check, found := find(Diagnose(cfg), "security.backendApiKey")
	if !found {
		t.Fatal("security.backendApiKey check missing")
	}
	if check.Severity != SeverityFail {
		t.Fatalf("severity = %q, want fail for a placeholder secret in production", check.Severity)
	}
}

// Locally the same placeholder should be visible but must not brick the stack.
func TestDiagnosePlaceholderSecretIsWarnOutsideProduction(t *testing.T) {
	cfg := healthyConfig()
	cfg.BackendAPIKey = "change-this-locally"
	cfg.RunMode = "demo"

	check, _ := find(Diagnose(cfg), "security.backendApiKey")
	if check.Severity != SeverityWarn {
		t.Fatalf("severity = %q, want warn outside production", check.Severity)
	}
}
