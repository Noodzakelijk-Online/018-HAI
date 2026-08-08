package task

import (
	"fmt"

	"automation-hub-backend/internal/sourceevidence"
)

// WithSourceEvidenceRepository replaces the repository derived from the
// connected-source service. It exists for composition roots and focused tests;
// production must use an owner-scoped durable repository.
func WithSourceEvidenceRepository(base Service, repository sourceevidence.Repository) (Service, error) {
	implementation, ok := base.(*service)
	if !ok {
		return nil, fmt.Errorf("source evidence resolution requires the built-in task service")
	}
	if repository == nil {
		return nil, fmt.Errorf("source evidence repository is required")
	}
	implementation.sourceEvidence = repository
	return implementation, nil
}
