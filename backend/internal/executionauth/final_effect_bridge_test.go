package executionauth

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"automation-hub-backend/internal/agentruntime"
)

func TestFinalEffectBridgeBindsAndAtomicallyExercisesMemoryReceipt(t *testing.T) {
	repository := NewMemoryRepository()
	service := newTestService(
		t,
		repository,
		permissiveConstitution(),
		nil,
		nil,
	)
	request, effectRequest := authorizedRuntimeRequest(t, "memory-final-effect")
	target, err := FinalEffectExecutionTarget(request.EffectDigest)
	if err != nil {
		t.Fatalf("FinalEffectExecutionTarget: %v", err)
	}
	receipt, err := service.AuthorizeAndConsume(
		context.Background(),
		request,
		"automation-runtime-handoff",
		target,
	)
	if err != nil {
		t.Fatalf("AuthorizeAndConsume: %v", err)
	}
	bridge, err := NewFinalEffectBridge(repository, fixedNow)
	if err != nil {
		t.Fatalf("NewFinalEffectBridge: %v", err)
	}
	binding, err := bridge.BindConsumedFinalEffect(
		context.Background(),
		effectRequest,
		receipt.ID,
	)
	if err != nil {
		t.Fatalf("BindConsumedFinalEffect: %v", err)
	}
	proof := proofFromBinding(t, binding, request.EffectDigest)

	start := make(chan struct{})
	results := make(chan error, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- bridge.VerifyFinalEffectProof(
				context.Background(),
				effectRequest,
				proof,
			)
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var successes atomic.Int32
	var replays atomic.Int32
	for result := range results {
		switch {
		case result == nil:
			successes.Add(1)
		case errors.Is(result, ErrAlreadyExercised):
			replays.Add(1)
		default:
			t.Fatalf("unexpected final effect result: %v", result)
		}
	}
	if successes.Load() != 1 || replays.Load() != 15 {
		t.Fatalf(
			"final effect results successes=%d replays=%d, want 1 and 15",
			successes.Load(),
			replays.Load(),
		)
	}
	exercise, err := repository.GetFinalEffectExercise(
		context.Background(),
		request.OwnerIdentity,
		receipt.ID,
	)
	if err != nil {
		t.Fatalf("GetFinalEffectExercise: %v", err)
	}
	if exercise.RuntimeID != request.RuntimeID ||
		exercise.TaskID != request.TaskID ||
		exercise.EffectDigest != request.EffectDigest ||
		exercise.ConsumptionTarget != target {
		t.Fatalf("stored exercise is not exact: %#v", exercise)
	}
}

func TestFinalEffectBridgeRejectsUnconsumedOrMismatchedProofs(t *testing.T) {
	repository := NewMemoryRepository()
	service := newTestService(
		t,
		repository,
		permissiveConstitution(),
		nil,
		nil,
	)
	request, effectRequest := authorizedRuntimeRequest(t, "mismatch-final-effect")
	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	bridge, err := NewFinalEffectBridge(repository, fixedNow)
	if err != nil {
		t.Fatalf("NewFinalEffectBridge: %v", err)
	}
	if _, err := bridge.BindConsumedFinalEffect(
		context.Background(),
		effectRequest,
		receipt.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unconsumed bind error = %v, want ErrNotFound", err)
	}

	target, _ := FinalEffectExecutionTarget(request.EffectDigest)
	if _, err := service.AuthorizeAndConsume(
		context.Background(),
		request,
		"automation-runtime-handoff",
		target,
	); err != nil {
		t.Fatalf("AuthorizeAndConsume: %v", err)
	}
	binding, err := bridge.BindConsumedFinalEffect(
		context.Background(),
		effectRequest,
		receipt.ID,
	)
	if err != nil {
		t.Fatalf("BindConsumedFinalEffect: %v", err)
	}
	valid := proofFromBinding(t, binding, request.EffectDigest)

	tests := []struct {
		name    string
		request agentruntime.FinalEffectAuthorizationRequest
		proof   agentruntime.FinalEffectAuthorizationProof
	}{
		{
			name: "owner",
			request: func() agentruntime.FinalEffectAuthorizationRequest {
				value := effectRequest
				value.OwnerIdentity = "bob"
				return value
			}(),
			proof: valid,
		},
		{
			name: "runtime",
			request: func() agentruntime.FinalEffectAuthorizationRequest {
				value := effectRequest
				value.RuntimeID = "odysseus"
				return value
			}(),
			proof: valid,
		},
		{
			name: "task",
			request: func() agentruntime.FinalEffectAuthorizationRequest {
				value := effectRequest
				value.TaskID = "task-other"
				return value
			}(),
			proof: valid,
		},
		{
			name:    "request digest",
			request: effectRequest,
			proof: func() agentruntime.FinalEffectAuthorizationProof {
				value := valid
				value.AuthorizationRequestDigest = digestForTest("wrong-request")
				return value
			}(),
		},
		{
			name:    "decision digest",
			request: effectRequest,
			proof: func() agentruntime.FinalEffectAuthorizationProof {
				value := valid
				value.DecisionDigest = digestForTest("wrong-decision")
				return value
			}(),
		},
		{
			name:    "runtime proof",
			request: effectRequest,
			proof: func() agentruntime.FinalEffectAuthorizationProof {
				value := valid
				value.RuntimeProof = digestForTest("wrong-effect")
				return value
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := bridge.VerifyFinalEffectProof(
				context.Background(),
				test.request,
				test.proof,
			)
			if err == nil {
				t.Fatal("mismatched final effect proof was accepted")
			}
			if _, lookupErr := repository.GetFinalEffectExercise(
				context.Background(),
				request.OwnerIdentity,
				receipt.ID,
			); !errors.Is(lookupErr, ErrNotFound) {
				t.Fatalf("rejected proof created exercise: %v", lookupErr)
			}
		})
	}
}

func TestBuildAgentRuntimeFinalEffectRequestMatchesRuntimeDigestContract(t *testing.T) {
	request, err := BuildAgentRuntimeFinalEffectRequest(
		"HERMES",
		"task-1",
		"alice",
		"project-1",
		"write the verified report",
		"",
		false,
	)
	if err != nil {
		t.Fatalf("BuildAgentRuntimeFinalEffectRequest: %v", err)
	}
	digest, err := FinalEffectDigest(request)
	if err != nil {
		t.Fatalf("FinalEffectDigest: %v", err)
	}
	if request.RuntimeID != "hermes" || !validDigest(digest) {
		t.Fatalf("runtime request was not normalized and digested: %#v %q", request, digest)
	}
	second, err := BuildAgentRuntimeFinalEffectRequest(
		"hermes",
		"task-1",
		"alice",
		"project-1",
		"write the verified report",
		"",
		false,
	)
	if err != nil || !reflect.DeepEqual(request, second) {
		t.Fatalf("runtime request is not deterministic: %#v %#v %v", request, second, err)
	}
}

func TestFinalEffectDigestMatchesAgentRuntimeRegistryBinding(t *testing.T) {
	task := agentruntime.Task{
		ID:               "task-1",
		Prompt:           "write the verified report",
		ProjectKey:       "project-1",
		OwnerIdentity:    "alice",
		ApprovalSourceID: "task-review:216967e4-d62e-4a73-ae3f-c62efcbf78f5",
	}
	request, err := BuildAgentRuntimeFinalEffectRequest(
		"hermes",
		task.ID,
		task.OwnerIdentity,
		task.ProjectKey,
		task.Prompt,
		task.ApprovalSourceID,
		true,
	)
	if err != nil {
		t.Fatalf("BuildAgentRuntimeFinalEffectRequest: %v", err)
	}
	effectDigest, err := FinalEffectDigest(request)
	if err != nil {
		t.Fatalf("FinalEffectDigest: %v", err)
	}
	registry := agentruntime.DefaultRegistry()
	bound, err := registry.BindConsumedAuthorizationProof(
		"hermes",
		task,
		"216967e4-d62e-4a73-ae3f-c62efcbf78f5",
		digestForTest("authorization-request"),
		digestForTest("decision"),
		effectDigest,
	)
	if err != nil {
		t.Fatalf("BindConsumedAuthorizationProof: %v", err)
	}
	if bound.FinalEffectProof.RuntimeRequestDigest != effectDigest ||
		bound.FinalEffectProof.RuntimeProof != effectDigest {
		t.Fatalf(
			"executionauth digest does not match agentruntime binding: %#v",
			bound.FinalEffectProof,
		)
	}
}

func authorizedRuntimeRequest(
	t *testing.T,
	key string,
) (Request, agentruntime.FinalEffectAuthorizationRequest) {
	t.Helper()
	finalRequest, err := BuildAgentRuntimeFinalEffectRequest(
		"hermes",
		"task-1",
		"alice",
		"project-1",
		"prepare a bounded result",
		"",
		false,
	)
	if err != nil {
		t.Fatalf("BuildAgentRuntimeFinalEffectRequest: %v", err)
	}
	effectDigest, err := FinalEffectDigest(finalRequest)
	if err != nil {
		t.Fatalf("FinalEffectDigest: %v", err)
	}
	request := baseRequest(key)
	request.Action = AgentRuntimeExecuteAction
	request.ResourceType = AgentRuntimeResourceType
	request.ResourceID = finalRequest.TaskID
	request.ProjectKey = finalRequest.ProjectKey
	request.RuntimeID = finalRequest.RuntimeID
	request.EffectDigest = effectDigest
	return request, finalRequest
}

func proofFromBinding(
	t *testing.T,
	binding FinalEffectBinding,
	effectDigest string,
) agentruntime.FinalEffectAuthorizationProof {
	t.Helper()
	if binding.ReceiptID == "" ||
		binding.AuthorizationRequestDigest == "" ||
		binding.DecisionDigest == "" ||
		binding.RuntimeProof != effectDigest {
		t.Fatalf("invalid final effect binding: %#v", binding)
	}
	return agentruntime.FinalEffectAuthorizationProof{
		ReceiptID:                  binding.ReceiptID,
		AuthorizationRequestDigest: binding.AuthorizationRequestDigest,
		DecisionDigest:             binding.DecisionDigest,
		RuntimeRequestDigest:       effectDigest,
		RuntimeProof:               binding.RuntimeProof,
	}
}

func digestForTest(value string) string {
	return fmt.Sprintf("%064x", value)
}
