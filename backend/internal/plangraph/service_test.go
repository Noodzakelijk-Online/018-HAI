package plangraph

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var fixedNow = time.Date(2026, 8, 5, 12, 0, 0, 123456789, time.UTC)

func validNode(id string) Node {
	return Node{
		ID: id, Type: "task", Title: "Complete " + id, Owner: "hai",
		Status: NodePlanned, EstimatedMinutes: 30, EstimatedCostEUR: 0,
		Risk: RiskLow, ApprovalState: ApprovalNotRequired,
	}
}

func previewRequest(key string) PreviewRequest {
	return PreviewRequest{
		IdempotencyKey: key, Title: "Safe advisory plan", CreatedBy: "robert",
		Nodes: []Node{validNode("collect"), validNode("review")},
		Edges: []Edge{{ID: "collect-review", From: "collect", To: "review", Type: "finish_to_start"}},
	}
}

func newTestService() (*Service, *MemoryRepository) {
	repository := NewMemoryRepository()
	service := NewService(repository, func() time.Time { return fixedNow })
	service.newID = func() uuid.UUID { return uuid.MustParse("11111111-1111-4111-8111-111111111111") }
	return service, repository
}

func TestPreviewIsDeterministicIdempotentAndAdvisory(t *testing.T) {
	service, _ := newTestService()
	request := previewRequest("preview-1")
	first, err := service.Preview(context.Background(), "owner-a", request)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	request.Nodes[0], request.Nodes[1] = request.Nodes[1], request.Nodes[0]
	second, err := service.Preview(context.Background(), "owner-a", request)
	if err != nil {
		t.Fatalf("idempotent preview: %v", err)
	}
	if first.ID != second.ID || first.Revision != second.Revision || first.Digest != second.Digest {
		t.Fatalf("idempotent preview changed identity: first=%+v second=%+v", first, second)
	}
	if first.CanExecute || second.CanExecute {
		t.Fatal("plan graph must never grant execution authority")
	}
	if !validDigest(first.Digest) || !validDigest(first.RequestDigest) {
		t.Fatalf("expected deterministic SHA-256 digests, got %q and %q", first.Digest, first.RequestDigest)
	}

	conflict := previewRequest("preview-1")
	conflict.Title = "Different request"
	if _, err := service.Preview(context.Background(), "owner-a", conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestPreviewRejectsCyclesAndInvalidTemporalBounds(t *testing.T) {
	service, _ := newTestService()
	request := previewRequest("cycle")
	request.Edges = append(request.Edges, Edge{ID: "review-collect", From: "review", To: "collect", Type: "finish_to_start"})
	if _, err := service.Preview(context.Background(), "owner-a", request); err == nil || !contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle rejection, got %v", err)
	}

	request = previewRequest("time")
	early := fixedNow.Add(2 * time.Hour)
	deadline := fixedNow.Add(time.Hour)
	request.Nodes[0].EarliestStart = &early
	request.Nodes[0].Deadline = &deadline
	if _, err := service.Preview(context.Background(), "owner-a", request); err == nil || !contains(err.Error(), "deadline") {
		t.Fatalf("expected temporal rejection, got %v", err)
	}
}

func TestOwnerIsolationRevisionConflictsAndAcceptedImmutability(t *testing.T) {
	service, repository := newTestService()
	draft, err := service.Preview(context.Background(), "owner-a", previewRequest("owner-a-preview"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := service.Get(context.Background(), "owner-b", draft.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read should be hidden, got %v", err)
	}
	if _, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{ExpectedRevision: 99, ExpectedDigest: draft.Digest, AcceptedBy: "robert"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
	if _, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{ExpectedRevision: 1, ExpectedDigest: strings.Repeat("0", 64), AcceptedBy: "robert"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected stale digest conflict, got %v", err)
	}
	accepted, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{ExpectedRevision: 1, ExpectedDigest: draft.Digest, AcceptedBy: "robert"})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Status != StatusAccepted || accepted.Revision != 2 || accepted.ParentDigest != draft.Digest || accepted.CanExecute {
		t.Fatalf("unexpected accepted revision: %+v", accepted)
	}
	historical, err := repository.GetRevision(context.Background(), "owner-a", draft.ID, 1)
	if err != nil {
		t.Fatalf("get historical draft: %v", err)
	}
	if historical.Status != StatusDraft || historical.Digest != draft.Digest {
		t.Fatalf("accepted transition mutated prior revision: %+v", historical)
	}
	replayed, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{ExpectedRevision: 1, ExpectedDigest: draft.Digest, AcceptedBy: "robert"})
	if err != nil || replayed.Digest != accepted.Digest {
		t.Fatalf("accept replay was not idempotent: replayed=%+v err=%v", replayed, err)
	}

	duplicate := clonePlan(*accepted)
	if err := repository.CreateRevision(context.Background(), duplicate, 1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected immutable duplicate revision conflict, got %v", err)
	}
}

func TestReplanBindsRepairProvenanceAndIsIdempotent(t *testing.T) {
	service, _ := newTestService()
	draft, err := service.Preview(context.Background(), "owner-a", previewRequest("initial"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	request := ReplanRequest{
		ExpectedRevision: draft.Revision, ExpectedDigest: draft.Digest, IdempotencyKey: "repair-1",
		Title: "Repaired plan", Nodes: []Node{validNode("safe-next")},
		Reason: "dependency unavailable", Trigger: "operator_review", CreatedBy: "robert",
	}
	replanned, err := service.Replan(context.Background(), "owner-a", draft.ID, request)
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if replanned.Revision != 2 || replanned.Status != StatusDraft || replanned.ParentDigest != draft.Digest || replanned.Repair == nil {
		t.Fatalf("unexpected replanned revision: %+v", replanned)
	}
	if replanned.Repair.PreviousRevision != draft.Revision || replanned.Repair.PreviousDigest != draft.Digest {
		t.Fatalf("repair provenance does not bind parent: %+v", replanned.Repair)
	}
	retry, err := service.Replan(context.Background(), "owner-a", draft.ID, request)
	if err != nil || retry.Digest != replanned.Digest {
		t.Fatalf("idempotent replan failed: retry=%+v err=%v", retry, err)
	}
	request.Title = "different repair"
	if _, err := service.Replan(context.Background(), "owner-a", draft.ID, request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected changed idempotent request conflict, got %v", err)
	}
}

func TestResolveAcceptedRequiresExactLatestAcceptedRevisionAndNode(t *testing.T) {
	service, _ := newTestService()
	draft, err := service.Preview(context.Background(), "owner-a", previewRequest("resolve"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	draftReference := AcceptedRevisionReference{PlanID: draft.ID, Revision: draft.Revision, Digest: draft.Digest, NodeID: "collect"}
	if _, err := service.ResolveAccepted(context.Background(), "owner-a", draftReference); !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("draft must not resolve as accepted, got %v", err)
	}
	accepted, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{
		ExpectedRevision: draft.Revision, ExpectedDigest: draft.Digest, AcceptedBy: "robert",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	reference := AcceptedRevisionReference{PlanID: accepted.ID, Revision: accepted.Revision, Digest: accepted.Digest, NodeID: "collect"}
	binding, err := service.ResolveAccepted(context.Background(), "owner-a", reference)
	if err != nil {
		t.Fatalf("resolve accepted: %v", err)
	}
	if binding.CanExecute || binding.Node.ID != "collect" || binding.Digest != accepted.Digest {
		t.Fatalf("unexpected advisory binding: %+v", binding)
	}
	if _, err := service.ResolveAccepted(context.Background(), "owner-b", reference); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner reference should be hidden, got %v", err)
	}
	wrongNode := reference
	wrongNode.NodeID = "missing"
	if _, err := service.ResolveAccepted(context.Background(), "owner-a", wrongNode); !errors.Is(err, ErrReferenceInvalid) {
		t.Fatalf("missing node should be rejected, got %v", err)
	}

	replanned, err := service.Replan(context.Background(), "owner-a", accepted.ID, ReplanRequest{
		ExpectedRevision: accepted.Revision, ExpectedDigest: accepted.Digest,
		IdempotencyKey: "resolve-replan", Title: "New revision",
		Nodes: []Node{validNode("collect")}, Reason: "changed conditions", Trigger: "review", CreatedBy: "robert",
	})
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if replanned.Revision == reference.Revision {
		t.Fatal("expected a newer revision")
	}
	if _, err := service.ResolveAccepted(context.Background(), "owner-a", reference); !errors.Is(err, ErrReferenceStale) {
		t.Fatalf("superseded accepted revision must fail closed, got %v", err)
	}
}

func TestResolveAcceptedRevisionRetainsHistoricalProvenanceWithoutAuthority(t *testing.T) {
	service, _ := newTestService()
	draft, err := service.Preview(context.Background(), "owner-a", previewRequest("historical-recovery"))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{
		ExpectedRevision: draft.Revision, ExpectedDigest: draft.Digest, AcceptedBy: "robert",
	})
	if err != nil {
		t.Fatal(err)
	}
	reference := AcceptedRevisionReference{
		PlanID: accepted.ID, Revision: accepted.Revision, Digest: accepted.Digest, NodeID: "collect",
	}
	if _, err := service.Replan(context.Background(), "owner-a", accepted.ID, ReplanRequest{
		ExpectedRevision: accepted.Revision, ExpectedDigest: accepted.Digest,
		IdempotencyKey: "historical-recovery-replan", Title: "Changed conditions",
		Nodes: []Node{validNode("collect")}, Reason: "world state changed", Trigger: "monitor", CreatedBy: "robert",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveAccepted(context.Background(), "owner-a", reference); !errors.Is(err, ErrReferenceStale) {
		t.Fatalf("new work must reject the stale accepted revision, got %v", err)
	}
	binding, err := service.ResolveAcceptedRevision(context.Background(), "owner-a", reference)
	if err != nil {
		t.Fatalf("historical accepted provenance: %v", err)
	}
	if binding.CanExecute || binding.PlanID != reference.PlanID || binding.Revision != reference.Revision || binding.NodeID != reference.NodeID {
		t.Fatalf("historical binding granted authority or changed identity: %#v", binding)
	}
}

func TestResolveAcceptedReturnsDefensiveAdvisoryGraph(t *testing.T) {
	service, _ := newTestService()
	request := previewRequest("resolve-full-graph")
	earliest := fixedNow.Add(time.Hour)
	deadline := fixedNow.Add(3 * time.Hour)
	request.Nodes[0].EarliestStart = &earliest
	request.Nodes[0].Deadline = &deadline
	draft, err := service.Preview(context.Background(), "owner-a", request)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	accepted, err := service.Accept(context.Background(), "owner-a", draft.ID, AcceptRequest{
		ExpectedRevision: draft.Revision, ExpectedDigest: draft.Digest, AcceptedBy: "robert",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	reference := AcceptedRevisionReference{
		PlanID: accepted.ID, Revision: accepted.Revision, Digest: accepted.Digest, NodeID: "collect",
	}
	binding, err := service.ResolveAccepted(context.Background(), "owner-a", reference)
	if err != nil {
		t.Fatalf("resolve accepted: %v", err)
	}
	if len(binding.Nodes) != len(accepted.Nodes) || len(binding.Edges) != len(accepted.Edges) {
		t.Fatalf("expected complete accepted graph, got %d nodes and %d edges", len(binding.Nodes), len(binding.Edges))
	}
	if binding.Nodes[0].ID != "collect" || binding.Nodes[1].ID != "review" || binding.Edges[0].ID != "collect-review" {
		t.Fatalf("unexpected accepted graph: nodes=%+v edges=%+v", binding.Nodes, binding.Edges)
	}
	if binding.CanExecute {
		t.Fatal("accepted graph binding must never grant execution authority")
	}
	originalEarliest := *binding.Nodes[0].EarliestStart
	originalDeadline := *binding.Nodes[0].Deadline

	changedTime := fixedNow.Add(24 * time.Hour)
	binding.Node.Title = "mutated selected node"
	binding.Nodes[0].Title = "mutated graph node"
	binding.Nodes[0].EarliestStart = &changedTime
	*binding.Nodes[0].Deadline = changedTime
	binding.Edges[0].To = "mutated-target"
	binding.Nodes = append(binding.Nodes, validNode("injected"))
	binding.Edges = nil

	resolvedAgain, err := service.ResolveAccepted(context.Background(), "owner-a", reference)
	if err != nil {
		t.Fatalf("resolve accepted again: %v", err)
	}
	if resolvedAgain.CanExecute {
		t.Fatal("re-resolved binding must remain non-authoritative")
	}
	if resolvedAgain.Node.Title != "Complete collect" || len(resolvedAgain.Nodes) != 2 || len(resolvedAgain.Edges) != 1 {
		t.Fatalf("binding mutation leaked into accepted graph: %+v", resolvedAgain)
	}
	if got := resolvedAgain.Nodes[0].EarliestStart; got == nil || !got.Equal(originalEarliest) {
		t.Fatalf("earliest start mutation leaked: %v", got)
	}
	if got := resolvedAgain.Nodes[0].Deadline; got == nil || !got.Equal(originalDeadline) {
		t.Fatalf("deadline mutation leaked: %v", got)
	}
	if resolvedAgain.Edges[0].To != "review" {
		t.Fatalf("edge mutation leaked: %+v", resolvedAgain.Edges[0])
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
