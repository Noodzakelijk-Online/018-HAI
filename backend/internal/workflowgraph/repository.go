package workflowgraph

import "context"

// DefinitionRepository stores immutable, versioned graph definitions.
type DefinitionRepository interface {
	CreateDefinition(ctx context.Context, definition Definition) error
	GetDefinition(ctx context.Context, id string, version uint64) (Definition, error)
	GetLatestDefinition(ctx context.Context, id string) (Definition, error)
	ListDefinitionVersions(ctx context.Context, id string) ([]Definition, error)
}

// RunRepository stores mutable runs. UpdateRun must atomically compare
// expectedRevision and return ErrRevisionConflict when another worker won.
type RunRepository interface {
	CreateRun(ctx context.Context, run Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, run Run, expectedRevision uint64) error
	ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)
}

type Repository interface {
	DefinitionRepository
	RunRepository
}
