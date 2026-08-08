package llm

import (
	"context"
	"strings"
	"testing"
)

func trustedTestEffectContext() *EffectContext {
	return &EffectContext{
		OwnerIdentity:         "owner:test-robert",
		ActorIdentity:         "actor:test-hai",
		ActorKind:             "system",
		TaskID:                "task:test-llm-effect",
		ProjectKey:            "project:test-018-hai",
		ApprovalSourceID:      "approval:test-reviewed",
		ApprovalBindingDigest: strings.Repeat("a", 64),
	}
}

func withTrustedTestEffect(request GenerateRequest) GenerateRequest {
	request.EffectContext = trustedTestEffectContext()
	return request
}

func withTrustedTestFinalEffects(t *testing.T, service *Service) *Service {
	t.Helper()
	effectContext := trustedTestEffectContext()
	service.WithMaintenanceEffectContext(*effectContext)
	return service.WithFinalEffectAuthorization(
		FinalEffectAuthorizerFunc(func(_ context.Context, request FinalEffectAuthorizationRequest) error {
			t.Helper()
			if request.OwnerIdentity != effectContext.OwnerIdentity ||
				request.ActorIdentity != effectContext.ActorIdentity ||
				request.TaskID != effectContext.TaskID ||
				request.ProjectKey != effectContext.ProjectKey {
				t.Fatalf("authorization received untrusted identity binding: %#v", request)
			}
			if strings.TrimSpace(request.ProviderID) == "" ||
				strings.TrimSpace(request.ModelID) == "" ||
				strings.TrimSpace(request.EndpointKey) == "" ||
				strings.TrimSpace(request.PayloadDigest) == "" ||
				strings.TrimSpace(request.EffectDigest) == "" {
				t.Fatalf("authorization received an incomplete final-effect binding: %#v", request)
			}
			if request.RequiresApproval && request.ApprovalSourceID != effectContext.ApprovalSourceID {
				t.Fatalf("approval-gated effect was not bound to the reviewed approval source: %#v", request)
			}
			return nil
		}),
		EmergencyStopEvaluatorFunc(func(context.Context) (EmergencyStopState, error) {
			return EmergencyStopState{}, nil
		}),
	)
}
