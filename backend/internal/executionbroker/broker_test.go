package executionbroker

import (
	"context"
	"strings"
	"testing"
)

const marker = "HAI_LOCAL_SAFE_WORKER_OK"

func TestLocalSafeWorkerSuccess(t *testing.T) {
	ws := t.TempDir()
	b := NewBroker(ws)
	res, err := b.ExecuteLocalSafeWorker(context.Background(), SafeWorkerInput{ArtifactName: "safe-worker-result.txt", Marker: marker})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.OK || !res.Verification.Passed {
		t.Fatalf("safe worker should pass verification: %+v", res.Verification)
	}
	if !res.Verification.FileExists || !res.Verification.HashMatches || !res.Verification.MarkerFound || !res.Verification.InsideWorkspace {
		t.Fatalf("verification postconditions not met: %+v", res.Verification)
	}
	if res.Output.BoundedOutput != marker {
		t.Fatalf("bounded output = %q", res.Output.BoundedOutput)
	}
}

func TestLocalSafeWorkerRejectsTraversalAndSeparatorsAndAbsolute(t *testing.T) {
	w := NewLocalSafeWorker(t.TempDir())
	for _, name := range []string{"../escape.txt", "sub/dir.txt", "/etc/passwd", "..", "a/../b.txt"} {
		if _, err := w.Run(SafeWorkerInput{ArtifactName: name, Marker: marker}); err == nil {
			t.Fatalf("artifactName %q must be rejected", name)
		}
	}
}

func TestLocalSafeWorkerRequiresMarker(t *testing.T) {
	w := NewLocalSafeWorker(t.TempDir())
	if _, err := w.Run(SafeWorkerInput{ArtifactName: "x.txt", Marker: ""}); err == nil {
		t.Fatalf("missing marker must be rejected")
	}
}

func TestVerifyFailsWhenMarkerAbsent(t *testing.T) {
	ws := t.TempDir()
	w := NewLocalSafeWorker(ws)
	in := SafeWorkerInput{ArtifactName: "x.txt", Marker: marker}
	out, err := w.Run(in)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Verifying against a different expected marker must fail.
	v := w.Verify(SafeWorkerInput{ArtifactName: "x.txt", Marker: "DIFFERENT_MARKER"}, out)
	if v.Passed {
		t.Fatalf("verification must fail when the expected marker is not in the file")
	}
}

func TestBrokerBlocksRuntimeNotConfigured(t *testing.T) {
	b := NewBroker("") // no workspace configured -> not_configured -> not executable
	if _, err := b.ExecuteLocalSafeWorker(context.Background(), SafeWorkerInput{ArtifactName: "x.txt", Marker: marker}); err == nil {
		t.Fatalf("broker must refuse to execute an unconfigured runtime")
	}
	if b.SafeWorker().HealthCheck(context.Background()).Status.CanExecute() {
		t.Fatalf("unconfigured runtime must not be executable")
	}
}

func TestBrokerBoundsOutput(t *testing.T) {
	w := NewLocalSafeWorker(t.TempDir())
	big := strings.Repeat("A", maxSafeOutput*2)
	out, err := w.Run(SafeWorkerInput{ArtifactName: "big.txt", Marker: big})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out.BoundedOutput) > maxSafeOutput {
		t.Fatalf("output must be bounded to %d, got %d", maxSafeOutput, len(out.BoundedOutput))
	}
}
