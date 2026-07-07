// Package factories builds valid test entities with sensible defaults and
// optional overrides, so tests and fixtures do not hand-assemble models (and
// accidentally create invalid ones). Pure; intended for tests and seed data.
package factories

import (
	"fmt"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

// Memory returns a valid ContextMemory with defaults, applying any overrides.
func Memory(overrides ...func(*models.ContextMemory)) models.ContextMemory {
	m := models.ContextMemory{
		ID:         uuid.New(),
		ProjectKey: "test-project",
		Kind:       "note",
		Content:    "example memory content",
		Summary:    "example",
		Tags:       "example",
		Confidence: 0.8,
	}
	for _, override := range overrides {
		override(&m)
	}
	return m
}

// Memories returns n valid memories with distinct content and ids.
func Memories(n int) []models.ContextMemory {
	out := make([]models.ContextMemory, 0, n)
	for i := 0; i < n; i++ {
		i := i
		out = append(out, Memory(func(m *models.ContextMemory) {
			m.Content = fmt.Sprintf("example memory %d", i)
		}))
	}
	return out
}
