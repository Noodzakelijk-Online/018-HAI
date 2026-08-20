package executionapproval

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"automation-hub-backend/internal/executionauth"
)

var ErrUnsupportedApprovalReference = errors.New("unsupported execution approval reference")

// CompositeResolver dispatches an approval reference to exactly one durable
// source. It never tries another resolver after the selected resolver rejects
// the evidence, preventing an unavailable or invalid approval from being
// weakened by fallback behavior.
type CompositeResolver struct {
	taskReview executionauth.ApprovalResolver
	workflow   executionauth.ApprovalResolver
	portfolio  executionauth.ApprovalResolver
	opsControl executionauth.ApprovalResolver
}

// WithOpsControlReviewResolver adds the dedicated exact-effect resolver used
// for safety-control changes. Existing approval sources retain their dispatch
// behavior and never fall back to this resolver.
func (r *CompositeResolver) WithOpsControlReviewResolver(
	resolver executionauth.ApprovalResolver,
) (*CompositeResolver, error) {
	if r == nil || isNilApprovalResolver(resolver) {
		return nil, fmt.Errorf("%w: ops-control review resolver is required", ErrInvalidRequest)
	}
	r.opsControl = resolver
	return r, nil
}

var _ executionauth.ApprovalResolver = (*CompositeResolver)(nil)

func NewCompositeResolver(
	taskReview executionauth.ApprovalResolver,
	workflow executionauth.ApprovalResolver,
	portfolio executionauth.ApprovalResolver,
) (*CompositeResolver, error) {
	if isNilApprovalResolver(taskReview) {
		return nil, fmt.Errorf("%w: task review resolver is required", ErrInvalidRequest)
	}
	if isNilApprovalResolver(workflow) {
		return nil, fmt.Errorf("%w: workflow approval resolver is required", ErrInvalidRequest)
	}
	if isNilApprovalResolver(portfolio) {
		return nil, fmt.Errorf("%w: portfolio approval resolver is required", ErrInvalidRequest)
	}
	return &CompositeResolver{
		taskReview: taskReview,
		workflow:   workflow,
		portfolio:  portfolio,
	}, nil
}

func (r *CompositeResolver) Resolve(
	ctx context.Context,
	ownerIdentity string,
	sourceID string,
	bindingDigest string,
) (executionauth.ResolvedApproval, error) {
	if r == nil ||
		isNilApprovalResolver(r.taskReview) ||
		isNilApprovalResolver(r.workflow) ||
		isNilApprovalResolver(r.portfolio) {
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: approval resolver composite is not configured",
			ErrInvalidRequest,
		)
	}
	switch {
	case strings.HasPrefix(sourceID, taskReviewPrefix):
		return r.taskReview.Resolve(ctx, ownerIdentity, sourceID, bindingDigest)
	case strings.HasPrefix(sourceID, workflowDecisionPrefix):
		return r.workflow.Resolve(ctx, ownerIdentity, sourceID, bindingDigest)
	case strings.HasPrefix(sourceID, portfolioDecisionPrefix):
		return r.portfolio.Resolve(ctx, ownerIdentity, sourceID, bindingDigest)
	case strings.HasPrefix(sourceID, OpsControlReviewPrefix):
		if isNilApprovalResolver(r.opsControl) {
			return executionauth.ResolvedApproval{}, ErrUnsupportedApprovalReference
		}
		return r.opsControl.Resolve(ctx, ownerIdentity, sourceID, bindingDigest)
	default:
		return executionauth.ResolvedApproval{}, ErrUnsupportedApprovalReference
	}
}

const portfolioDecisionPrefix = "portfolio-decision:"

func isNilApprovalResolver(resolver executionauth.ApprovalResolver) bool {
	if resolver == nil {
		return true
	}
	value := reflect.ValueOf(resolver)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
