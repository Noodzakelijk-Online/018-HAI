package agentregistry

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepositoryClonesMutableState(t *testing.T) {
	repository := NewMemoryRepository()
	agent := testAgent("alice@example.com", "worker")
	agent.ContractVersion = ContractVersion
	agent.State = StateRegistered
	agent.Revision = 1
	agent.CreatedAt = testNow
	agent.UpdatedAt = testNow
	created, err := repository.Create(context.Background(), agent)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.Capabilities[0].Operations[0] = "mutated"
	created.ToolAllowlist[0] = "mutated"

	stored, err := repository.Get(context.Background(), agent.OwnerIdentity, agent.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Capabilities[0].Operations[0] == "mutated" || stored.ToolAllowlist[0] == "mutated" {
		t.Fatal("repository returned mutable internal state")
	}
}

func TestMemoryRepositoryCompareAndSwapRejectsWrongRevision(t *testing.T) {
	repository := NewMemoryRepository()
	agent := testAgent("alice@example.com", "worker")
	agent.ContractVersion = ContractVersion
	agent.State = StateRegistered
	agent.Revision = 1
	agent.CreatedAt = testNow
	agent.UpdatedAt = testNow
	if _, err := repository.Create(context.Background(), agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	agent.Revision = 3
	if _, err := repository.CompareAndSwap(context.Background(), agent, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("CAS error = %v, want ErrConflict", err)
	}
}
