package dataexport

import (
	"testing"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

func TestBuildMemoryExportIsVersioned(t *testing.T) {
	mems := []models.ContextMemory{{ID: uuid.New(), Content: "a"}, {ID: uuid.New(), Content: "b"}}
	exp := BuildMemoryExport(mems)
	if exp.Format != exportFormat || exp.Version != exportVersion {
		t.Fatalf("envelope wrong: %+v", exp)
	}
	if exp.Count != 2 || len(exp.Memories) != 2 {
		t.Fatalf("count/memories wrong: %+v", exp)
	}
}

func TestPlanDeletionSplitsPresentAndMissing(t *testing.T) {
	a := uuid.New()
	mems := []models.ContextMemory{{ID: a, Content: "x"}}
	manifest := PlanDeletion(mems, []string{a.String(), "does-not-exist"})
	if manifest.Requested != 2 {
		t.Fatalf("requested = %d, want 2", manifest.Requested)
	}
	if len(manifest.Deletable) != 1 || manifest.Deletable[0] != a.String() {
		t.Fatalf("deletable wrong: %+v", manifest.Deletable)
	}
	if len(manifest.NotFound) != 1 || manifest.NotFound[0] != "does-not-exist" {
		t.Fatalf("notFound wrong: %+v", manifest.NotFound)
	}
}
