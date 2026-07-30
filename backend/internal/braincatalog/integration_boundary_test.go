package braincatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEveryIntegratedCatalogProfileHasAConcreteHAIBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve catalog test source path")
	}
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	integrated := map[string]bool{}
	for _, entry := range Entries() {
		if entry.Status != StatusIntegrated {
			continue
		}
		integrated[entry.ID] = true
		if entry.Implementation == nil {
			t.Fatalf("integrated profile %q has no HAI implementation boundary", entry.ID)
		}
		boundary := entry.Implementation
		if strings.TrimSpace(boundary.Control) == "" || strings.TrimSpace(boundary.Route) == "" || strings.TrimSpace(boundary.Scope) == "" || !strings.HasPrefix(boundary.SourcePath, "backend/") {
			t.Fatalf("integrated profile %q has an invalid implementation boundary: %#v", entry.ID, boundary)
		}
		sourcePath := strings.TrimPrefix(boundary.SourcePath, "backend/")
		if _, err := os.Stat(filepath.Join(backendRoot, filepath.FromSlash(sourcePath))); err != nil {
			t.Fatalf("integrated profile %q points to a missing source boundary %q: %v", entry.ID, boundary.SourcePath, err)
		}
	}
	if len(integrated) != len(integratedImplementationBoundaries) {
		t.Fatalf("integrated profiles=%d but implementation boundaries=%d; catalog and runtime evidence drifted", len(integrated), len(integratedImplementationBoundaries))
	}
	for id := range integratedImplementationBoundaries {
		if !integrated[id] {
			t.Fatalf("implementation boundary %q does not correspond to a StatusIntegrated catalog entry", id)
		}
	}
}
