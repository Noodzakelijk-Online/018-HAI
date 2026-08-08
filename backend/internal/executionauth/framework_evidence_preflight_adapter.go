package executionauth

import (
	"context"
	"fmt"

	"automation-hub-backend/internal/frameworkevidence"
)

type frameworkEvidencePreflightRepository interface {
	Resolve(
		context.Context,
		string,
		string,
		string,
		string,
	) (frameworkevidence.Record, error)
}

type repositoryFrameworkEvidencePreflightResolver struct {
	repository frameworkEvidencePreflightRepository
}

func NewFrameworkEvidencePreflightResolver(
	repository frameworkEvidencePreflightRepository,
) (FrameworkEvidencePreflightResolver, error) {
	if repository == nil {
		return nil, fmt.Errorf("framework evidence repository is required")
	}
	return &repositoryFrameworkEvidencePreflightResolver{repository: repository}, nil
}

func (r *repositoryFrameworkEvidencePreflightResolver) ResolveFrameworkEvidencePreflight(
	ctx context.Context,
	ownerIdentity string,
	taskPlanID string,
	frameworkSelectionID string,
	preflightDigest string,
) (FrameworkEvidencePreflightSnapshot, error) {
	record, err := r.repository.Resolve(
		ctx,
		ownerIdentity,
		taskPlanID,
		frameworkSelectionID,
		preflightDigest,
	)
	if err != nil {
		return FrameworkEvidencePreflightSnapshot{}, err
	}
	return FrameworkEvidencePreflightSnapshot{
		OwnerIdentity:        record.OwnerIdentity,
		TaskPlanID:           record.TaskPlanID,
		FrameworkSelectionID: record.FrameworkSelectionID,
		PreflightDigest:      record.PreflightDigest,
		Status:               string(record.Status),
		AssertionsJSON:       append([]byte(nil), record.AssertionsJSON...),
	}, nil
}
