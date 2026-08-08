package pursuit

import (
	"context"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/resourceplanner"

	"github.com/google/uuid"
)

type pursuitAcceptedPlanResolverFunc func(context.Context, string, plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error)

func (resolve pursuitAcceptedPlanResolverFunc) ResolveAccepted(
	ctx context.Context,
	ownerIdentity string,
	reference plangraph.AcceptedRevisionReference,
) (*plangraph.AcceptedRevisionBinding, error) {
	return resolve(ctx, ownerIdentity, reference)
}

func TestResolvePursuitCoordinationPlanRequiresExactPursuitNode(t *testing.T) {
	pursuitID := uuid.New()
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 3, Digest: strings.Repeat("a", 64), NodeID: "pursuit-node",
	}
	resolver := pursuitAcceptedPlanResolverFunc(func(_ context.Context, owner string, got plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
		if owner != "alice" || got != reference {
			t.Fatalf("resolver owner/reference = %q %#v", owner, got)
		}
		return &plangraph.AcceptedRevisionBinding{
			PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
			NodeID: reference.NodeID, Node: plangraph.Node{
				ID: reference.NodeID, Bindings: plangraph.Bindings{PursuitID: pursuitID.String()},
			}, CanExecute: false,
		}, nil
	})
	svc := &service{acceptedPlanResolver: resolver}

	binding, err := svc.resolvePursuitCoordinationPlan("alice", pursuitID, reference)
	if err != nil || binding == nil || binding.CanExecute {
		t.Fatalf("binding=%#v err=%v", binding, err)
	}
	if _, err := svc.resolvePursuitCoordinationPlan("alice", uuid.New(), reference); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("mismatched pursuit error = %v", err)
	}
}

func TestResolvePursuitCoordinationPlanFailsClosedWithoutResolver(t *testing.T) {
	svc := &service{}
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 1, Digest: strings.Repeat("b", 64), NodeID: "node",
	}
	if _, err := svc.resolvePursuitCoordinationPlan("alice", uuid.New(), reference); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing resolver error = %v", err)
	}
}

func TestResolvePortfolioCoordinationPlanRequiresEveryPursuit(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 5, Digest: strings.Repeat("c", 64), NodeID: "portfolio",
	}
	resolver := pursuitAcceptedPlanResolverFunc(func(_ context.Context, _ string, _ plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
		return &plangraph.AcceptedRevisionBinding{
			PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
			NodeID: reference.NodeID, Node: plangraph.Node{ID: reference.NodeID},
			Nodes: []plangraph.Node{
				{ID: "first", Bindings: plangraph.Bindings{PursuitID: first.String()}},
				{ID: "second", Bindings: plangraph.Bindings{PursuitID: second.String()}},
			}, CanExecute: false,
		}, nil
	})
	svc := &service{acceptedPlanResolver: resolver}
	inputs := []PortfolioPursuitPlanningInput{{PursuitID: first}, {PursuitID: second}}

	binding, err := svc.resolvePortfolioCoordinationPlan("alice", inputs, reference)
	if err != nil || binding == nil || binding.CanExecute {
		t.Fatalf("binding=%#v err=%v", binding, err)
	}
	inputs = append(inputs, PortfolioPursuitPlanningInput{PursuitID: uuid.New()})
	if _, err := svc.resolvePortfolioCoordinationPlan("alice", inputs, reference); err == nil || !strings.Contains(err.Error(), "does not contain pursuit") {
		t.Fatalf("missing pursuit error = %v", err)
	}
}

func TestResolvePursuitCoordinationPlanRejectsExecutableBinding(t *testing.T) {
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 2, Digest: strings.Repeat("d", 64), NodeID: "node",
	}
	resolver := pursuitAcceptedPlanResolverFunc(func(_ context.Context, _ string, _ plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
		return &plangraph.AcceptedRevisionBinding{CanExecute: true}, nil
	})
	if _, err := (&service{acceptedPlanResolver: resolver}).resolvePursuitCoordinationPlan("alice", uuid.New(), reference); err == nil || !strings.Contains(err.Error(), "advisory-only") {
		t.Fatalf("executable binding error = %v", err)
	}
}

func TestPlanForOwnerPropagatesExactAcceptedPlanToWorkflow(t *testing.T) {
	repo := newFakeRepo()
	workflowIntake := &fakeWorkflowIntake{repo: repo}
	value := NewService(repo, workflowIntake)
	pursuit, err := value.Create(CreateRequest{OwnerIdentity: "alice", Title: "Prepare governed evidence"})
	if err != nil {
		t.Fatal(err)
	}
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 7, Digest: strings.Repeat("e", 64), NodeID: "pursuit-plan",
	}
	resolver := pursuitAcceptedPlanResolverFunc(func(_ context.Context, _ string, got plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
		return &plangraph.AcceptedRevisionBinding{
			PlanID: got.PlanID, Revision: got.Revision, Digest: got.Digest, NodeID: got.NodeID,
			Node:       plangraph.Node{ID: got.NodeID, Bindings: plangraph.Bindings{PursuitID: pursuit.ID.String()}},
			CanExecute: false,
		}, nil
	})
	value, err = WithAcceptedPlanResolver(value, resolver)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := value.PlanForOwner("alice", pursuit.ID, PlanRequest{Input: "Build the first governed workflow", CoordinationPlan: reference}); err != nil {
		t.Fatalf("PlanForOwner: %v", err)
	}
	if workflowIntake.calls != 1 || workflowIntake.received.CoordinationPlan != reference {
		t.Fatalf("workflow coordination reference = %#v calls=%d", workflowIntake.received.CoordinationPlan, workflowIntake.calls)
	}
}

func TestPlanPortfolioForOwnerExposesAdvisoryAcceptedPlanBinding(t *testing.T) {
	repo := newFakeRepo()
	value := NewService(repo, nil)
	pursuit, err := value.Create(CreateRequest{OwnerIdentity: "alice", Title: "Schedule governed work"})
	if err != nil {
		t.Fatal(err)
	}
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 4, Digest: strings.Repeat("f", 64), NodeID: "portfolio-root",
	}
	resolver := pursuitAcceptedPlanResolverFunc(func(_ context.Context, _ string, got plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
		return &plangraph.AcceptedRevisionBinding{
			PlanID: got.PlanID, Revision: got.Revision, Digest: got.Digest, NodeID: got.NodeID,
			Node:       plangraph.Node{ID: got.NodeID},
			Nodes:      []plangraph.Node{{ID: "work", Bindings: plangraph.Bindings{PursuitID: pursuit.ID.String()}}},
			CanExecute: false,
		}, nil
	})
	value, err = WithAcceptedPlanResolver(value, resolver)
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	result, err := value.PlanPortfolioForOwner("alice", PortfolioPlanningRequest{
		PlanID: "coordination-backed-portfolio", AsOf: asOf,
		HorizonStart: asOf, HorizonEnd: asOf.Add(2 * time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(2 * time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{{
			PursuitID: pursuit.ID, Duration: portfolioDuration(15, 30, 45), Factors: portfolioFactors(50),
		}},
		Budget:           resourceplanner.Budget{MaxCostMicros: portfolioInt64(0)},
		CoordinationPlan: reference,
	})
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if result.CoordinationPlan == nil || result.CoordinationPlan.CanExecute || result.CanExecute || result.Authority != "advisory_only" {
		t.Fatalf("portfolio coordination authority = %#v", result)
	}
}
