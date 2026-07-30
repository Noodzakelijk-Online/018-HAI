package agentcoordination

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeTransport struct {
	calls   int
	receipt DeliveryReceipt
	err     error
}

func (transport *fakeTransport) Deliver(
	_ context.Context,
	message Message,
) (DeliveryReceipt, error) {
	transport.calls++
	if transport.err != nil {
		return DeliveryReceipt{}, transport.err
	}
	receipt := transport.receipt
	if receipt.MessageID == "" {
		receipt = DeliveryReceipt{
			MessageID:     message.ID,
			CorrelationID: message.CorrelationID,
			TransportID:   "transport-1",
			AcceptedAt:    message.CreatedAt.Add(time.Second),
		}
	}
	return receipt, nil
}

type memoryDispatchRecord struct {
	digest    string
	completed bool
	receipt   DeliveryReceipt
}

type memoryDispatchStore struct {
	records map[string]memoryDispatchRecord
}

func newMemoryDispatchStore() *memoryDispatchStore {
	return &memoryDispatchStore{records: map[string]memoryDispatchRecord{}}
}

func (store *memoryDispatchStore) Begin(
	_ context.Context,
	key string,
	digest string,
	_ time.Time,
) (DispatchClaim, error) {
	record, exists := store.records[key]
	if !exists {
		store.records[key] = memoryDispatchRecord{digest: digest}
		return DispatchClaim{Status: DispatchClaimAcquired}, nil
	}
	if record.digest != digest {
		return DispatchClaim{Status: DispatchClaimConflict}, nil
	}
	if record.completed {
		receipt := record.receipt
		return DispatchClaim{Status: DispatchClaimDuplicate, Receipt: &receipt}, nil
	}
	return DispatchClaim{Status: DispatchClaimDuplicate}, nil
}

func (store *memoryDispatchStore) Complete(
	_ context.Context,
	key string,
	digest string,
	receipt DeliveryReceipt,
) error {
	record, exists := store.records[key]
	if !exists || record.digest != digest {
		return fmt.Errorf("missing dispatch claim")
	}
	record.completed = true
	record.receipt = receipt
	store.records[key] = record
	return nil
}

func (store *memoryDispatchStore) Abandon(
	_ context.Context,
	key string,
	digest string,
) error {
	record, exists := store.records[key]
	if exists && record.digest == digest && !record.completed {
		delete(store.records, key)
	}
	return nil
}

func TestCoordinatorDispatchIsIdempotent(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	transport := &fakeTransport{}
	store := newMemoryDispatchStore()
	coordinator, err := NewCoordinator(
		DefaultValidationPolicy(),
		transport,
		store,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	message := validMessage(t, now)
	first, err := coordinator.Dispatch(context.Background(), message)
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	second, err := coordinator.Dispatch(context.Background(), message)
	if err != nil {
		t.Fatalf("duplicate dispatch: %v", err)
	}
	if transport.calls != 1 || first.Duplicate || !second.Duplicate {
		t.Fatalf(
			"idempotent dispatch failed: calls=%d first=%#v second=%#v",
			transport.calls,
			first,
			second,
		)
	}
}

func TestCoordinatorReleasesClaimAfterTransportFailure(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	transport := &fakeTransport{err: fmt.Errorf("offline")}
	store := newMemoryDispatchStore()
	coordinator, err := NewCoordinator(
		DefaultValidationPolicy(),
		transport,
		store,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	message := validMessage(t, now)
	if _, err := coordinator.Dispatch(context.Background(), message); err == nil {
		t.Fatal("transport failure was not returned")
	}
	transport.err = nil
	if _, err := coordinator.Dispatch(context.Background(), message); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if transport.calls != 2 {
		t.Fatalf("transport calls = %d, want 2", transport.calls)
	}
}

func TestCoordinatorRejectsIdempotencyKeyReuseWithDifferentContent(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	transport := &fakeTransport{}
	store := newMemoryDispatchStore()
	coordinator, err := NewCoordinator(
		DefaultValidationPolicy(),
		transport,
		store,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	message := validMessage(t, now)
	if _, err := coordinator.Dispatch(context.Background(), message); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	message.ID = "23da87aa-8ea7-4de4-8d61-869fb9b808ed"
	message.Payload.Subject = "different content"
	message = withDigest(t, message)
	if _, err := coordinator.Dispatch(context.Background(), message); err != ErrIdempotencyConflict {
		t.Fatalf("key reuse error = %v, want %v", err, ErrIdempotencyConflict)
	}
	if transport.calls != 1 {
		t.Fatalf("conflicting content reached transport")
	}
}
