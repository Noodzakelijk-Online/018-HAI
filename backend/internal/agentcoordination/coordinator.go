package agentcoordination

import (
	"context"
	"fmt"
	"time"
)

type Clock func() time.Time

type Coordinator struct {
	policy    ValidationPolicy
	transport Transport
	store     DispatchStore
	clock     Clock
}

func NewCoordinator(
	policy ValidationPolicy,
	transport Transport,
	store DispatchStore,
	clock Clock,
) (*Coordinator, error) {
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, fmt.Errorf("agent coordination transport is required")
	}
	if store == nil {
		return nil, fmt.Errorf("agent coordination dispatch store is required")
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Coordinator{
		policy:    policy,
		transport: transport,
		store:     store,
		clock:     clock,
	}, nil
}

// Dispatch validates before acquiring an idempotency claim. A successful
// delivery only proves transport acceptance; it does not prove task execution.
func (coordinator *Coordinator) Dispatch(
	ctx context.Context,
	message Message,
) (DeliveryReceipt, error) {
	now := coordinator.clock().UTC()
	if err := ValidateMessage(coordinator.policy, message, now); err != nil {
		return DeliveryReceipt{}, err
	}
	digest, err := ComputeMessageDigest(message)
	if err != nil {
		return DeliveryReceipt{}, fmt.Errorf("compute dispatch digest: %w", err)
	}
	claim, err := coordinator.store.Begin(
		ctx,
		message.IdempotencyKey,
		digest,
		message.ExpiresAt.UTC(),
	)
	if err != nil {
		return DeliveryReceipt{}, fmt.Errorf("begin idempotent dispatch: %w", err)
	}
	switch claim.Status {
	case DispatchClaimDuplicate:
		if claim.Receipt == nil {
			return DeliveryReceipt{}, fmt.Errorf("%w: receipt unavailable", ErrDuplicateDispatch)
		}
		receipt := *claim.Receipt
		receipt.Duplicate = true
		return receipt, nil
	case DispatchClaimConflict:
		return DeliveryReceipt{}, ErrIdempotencyConflict
	case DispatchClaimAcquired:
	default:
		return DeliveryReceipt{}, fmt.Errorf("dispatch store returned an invalid claim state")
	}

	receipt, err := coordinator.transport.Deliver(ctx, message)
	if err != nil {
		if abandonErr := coordinator.store.Abandon(
			ctx,
			message.IdempotencyKey,
			digest,
		); abandonErr != nil {
			return DeliveryReceipt{}, fmt.Errorf(
				"deliver message: %v; abandon idempotency claim: %w",
				err,
				abandonErr,
			)
		}
		return DeliveryReceipt{}, fmt.Errorf("deliver message: %w", err)
	}
	if receipt.MessageID != message.ID ||
		receipt.CorrelationID != message.CorrelationID ||
		receipt.AcceptedAt.IsZero() {
		_ = coordinator.store.Abandon(ctx, message.IdempotencyKey, digest)
		return DeliveryReceipt{}, fmt.Errorf("transport returned an invalid receipt")
	}
	if err := coordinator.store.Complete(
		ctx,
		message.IdempotencyKey,
		digest,
		receipt,
	); err != nil {
		return DeliveryReceipt{}, fmt.Errorf("complete idempotent dispatch: %w", err)
	}
	return receipt, nil
}
