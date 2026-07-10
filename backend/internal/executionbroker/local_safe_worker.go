package executionbroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"automation-hub-backend/internal/pathsafety"
)

// LocalSafeWorkerID is the runtime id of the local safe worker.
const LocalSafeWorkerID = "hai-local-safe-worker"

const maxSafeOutput = 4096

// SafeWorkerInput is the safe worker's bounded payload (§10.15).
type SafeWorkerInput struct {
	ArtifactName string `json:"artifactName"`
	Marker       string `json:"marker"`
}

// SafeWorkerOutput is the safe worker's bounded result.
type SafeWorkerOutput struct {
	ArtifactPath  string `json:"artifactPath"`
	ArtifactHash  string `json:"artifactHash"`
	MarkerFound   bool   `json:"markerFound"`
	BoundedOutput string `json:"boundedOutput"`
}

// LocalSafeWorker creates a small text file in a configured workspace, reads it
// back, hashes it, and returns bounded output. It performs NO os/exec, network,
// arbitrary paths, symlink escape, deletion, home traversal, or account access.
type LocalSafeWorker struct {
	WorkspaceRoot string
}

// NewLocalSafeWorker builds a safe worker confined to workspaceRoot.
func NewLocalSafeWorker(workspaceRoot string) *LocalSafeWorker {
	return &LocalSafeWorker{WorkspaceRoot: workspaceRoot}
}

func (w *LocalSafeWorker) ID() string          { return LocalSafeWorkerID }
func (w *LocalSafeWorker) DisplayName() string  { return "HAI Local Safe Worker" }

func (w *LocalSafeWorker) ClaimLevel(ctx context.Context) ClaimLevel {
	if strings.TrimSpace(w.WorkspaceRoot) == "" {
		return ClaimContractDefined
	}
	return ClaimExercisedSafeTask
}

func (w *LocalSafeWorker) HealthCheck(ctx context.Context) RuntimeHealth {
	if strings.TrimSpace(w.WorkspaceRoot) == "" {
		return RuntimeHealth{Status: RuntimeNotConfigured, Detail: "workspace root not configured", Claim: ClaimContractDefined}
	}
	return RuntimeHealth{Status: RuntimeReady, Detail: "workspace configured", Claim: ClaimExercisedSafeTask}
}

func (w *LocalSafeWorker) DryRun(ctx context.Context, payload map[string]any) (DryRunResult, error) {
	in, err := parseSafeWorkerInput(payload)
	if err != nil {
		return DryRunResult{OK: false, Summary: err.Error()}, err
	}
	if _, err := w.resolvePath(in.ArtifactName); err != nil {
		return DryRunResult{OK: false, Summary: err.Error()}, err
	}
	return DryRunResult{OK: true, Summary: "will write, read back, and hash " + in.ArtifactName}, nil
}

func (w *LocalSafeWorker) Execute(ctx context.Context, payload map[string]any) (RuntimeResult, error) {
	out, err := w.Run(parseSafeWorkerInputOrZero(payload))
	if err != nil {
		return RuntimeResult{OK: false, Error: err.Error()}, err
	}
	return RuntimeResult{OK: true, BoundedOutput: out.BoundedOutput}, nil
}

// Run performs the safe workspace-only task and returns the bounded output.
func (w *LocalSafeWorker) Run(in SafeWorkerInput) (SafeWorkerOutput, error) {
	if strings.TrimSpace(in.ArtifactName) == "" {
		return SafeWorkerOutput{}, fmt.Errorf("safe worker: artifactName required")
	}
	if strings.TrimSpace(in.Marker) == "" {
		return SafeWorkerOutput{}, fmt.Errorf("safe worker: marker required")
	}
	full, err := w.resolvePath(in.ArtifactName)
	if err != nil {
		return SafeWorkerOutput{}, err
	}
	if err := os.MkdirAll(w.WorkspaceRoot, 0o755); err != nil {
		return SafeWorkerOutput{}, err
	}
	if err := os.WriteFile(full, []byte(in.Marker), 0o600); err != nil {
		return SafeWorkerOutput{}, err
	}
	read, err := os.ReadFile(full)
	if err != nil {
		return SafeWorkerOutput{}, err
	}
	sum := sha256.Sum256(read)
	return SafeWorkerOutput{
		ArtifactPath:  full,
		ArtifactHash:  hex.EncodeToString(sum[:]),
		MarkerFound:   strings.Contains(string(read), in.Marker),
		BoundedOutput: boundOutput(string(read), maxSafeOutput),
	}, nil
}

// resolvePath validates artifactName (basename only, no separators/dot-dot/
// absolute) and confines it inside the workspace root.
func (w *LocalSafeWorker) resolvePath(artifactName string) (string, error) {
	if strings.TrimSpace(w.WorkspaceRoot) == "" {
		return "", fmt.Errorf("safe worker: workspace root not configured")
	}
	if filepath.Base(artifactName) != artifactName {
		return "", fmt.Errorf("safe worker: artifactName must be a basename with no path separators")
	}
	if !pathsafety.IsSafeRelative(artifactName) {
		return "", fmt.Errorf("safe worker: unsafe artifactName %q", artifactName)
	}
	full, err := pathsafety.SafeJoin(w.WorkspaceRoot, artifactName)
	if err != nil {
		return "", fmt.Errorf("safe worker: %w", err)
	}
	return full, nil
}

// VerifySafeWorker checks the postconditions of a safe worker run (§10.15):
// file exists, path inside workspace, hash matches, marker found, output bounded.
type SafeWorkerVerification struct {
	FileExists      bool `json:"fileExists"`
	InsideWorkspace bool `json:"insideWorkspace"`
	HashMatches     bool `json:"hashMatches"`
	MarkerFound     bool `json:"markerFound"`
	OutputBounded   bool `json:"outputBounded"`
	Passed          bool `json:"passed"`
}

func (w *LocalSafeWorker) Verify(in SafeWorkerInput, out SafeWorkerOutput) SafeWorkerVerification {
	v := SafeWorkerVerification{}
	data, err := os.ReadFile(out.ArtifactPath)
	v.FileExists = err == nil

	if abs, err := filepath.Abs(w.WorkspaceRoot); err == nil {
		v.InsideWorkspace = strings.HasPrefix(out.ArtifactPath, abs) || strings.HasPrefix(out.ArtifactPath, w.WorkspaceRoot)
	}
	if v.FileExists {
		sum := sha256.Sum256(data)
		v.HashMatches = hex.EncodeToString(sum[:]) == out.ArtifactHash
		v.MarkerFound = strings.Contains(string(data), in.Marker)
	}
	v.OutputBounded = len(out.BoundedOutput) <= maxSafeOutput
	v.Passed = v.FileExists && v.InsideWorkspace && v.HashMatches && v.MarkerFound && v.OutputBounded
	return v
}

func parseSafeWorkerInput(payload map[string]any) (SafeWorkerInput, error) {
	in := parseSafeWorkerInputOrZero(payload)
	if strings.TrimSpace(in.ArtifactName) == "" || strings.TrimSpace(in.Marker) == "" {
		return in, fmt.Errorf("safe worker: artifactName and marker required")
	}
	return in, nil
}

func parseSafeWorkerInputOrZero(payload map[string]any) SafeWorkerInput {
	in := SafeWorkerInput{}
	if v, ok := payload["artifactName"].(string); ok {
		in.ArtifactName = v
	}
	if v, ok := payload["marker"].(string); ok {
		in.Marker = v
	}
	return in
}

func boundOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
