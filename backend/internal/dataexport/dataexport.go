// Package dataexport builds portable user-data exports and safe deletion plans
// for context memories. It is pure: callers load records, build the export or
// deletion manifest, then persist/serialize the result.
package dataexport

import "automation-hub-backend/internal/models"

const (
	exportFormat  = "018-hai-user-data"
	exportVersion = 1
)

// Export is a portable snapshot of a user's memories.
type Export struct {
	Format   string                 `json:"format"`
	Version  int                    `json:"version"`
	Count    int                    `json:"count"`
	Memories []models.ContextMemory `json:"memories"`
}

// BuildMemoryExport wraps memories in a versioned, self-describing envelope.
func BuildMemoryExport(memories []models.ContextMemory) Export {
	items := make([]models.ContextMemory, 0, len(memories))
	items = append(items, memories...)
	return Export{Format: exportFormat, Version: exportVersion, Count: len(items), Memories: items}
}

// DeletionManifest is the result of planning a deletion request: which requested
// IDs exist and can be deleted, and which were not found.
type DeletionManifest struct {
	Requested int      `json:"requested"`
	Deletable []string `json:"deletable"`
	NotFound  []string `json:"notFound"`
}

// PlanDeletion splits requested IDs into those present in memories (deletable)
// and those absent (not found), so a caller never silently "deletes" nothing.
func PlanDeletion(memories []models.ContextMemory, requestedIDs []string) DeletionManifest {
	present := make(map[string]bool, len(memories))
	for _, m := range memories {
		present[m.ID.String()] = true
	}
	manifest := DeletionManifest{Requested: len(requestedIDs), Deletable: []string{}, NotFound: []string{}}
	for _, id := range requestedIDs {
		if present[id] {
			manifest.Deletable = append(manifest.Deletable, id)
		} else {
			manifest.NotFound = append(manifest.NotFound, id)
		}
	}
	return manifest
}
