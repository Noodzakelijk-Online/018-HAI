package knowledgegraph

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)

func newTestService() *Service {
	return NewService(NewMemoryRepository(), func() time.Time { return testNow })
}

func createTestNode(t *testing.T, service *Service, owner string, kind NodeKind, label string, options ...func(*CreateNodeRequest)) Node {
	t.Helper()
	request := CreateNodeRequest{
		OwnerIdentity:      owner,
		Kind:               kind,
		Label:              label,
		Confidence:         0.8,
		VerificationStatus: VerificationSourceSupported,
		Sensitivity:        SensitivityInternal,
	}
	for _, option := range options {
		option(&request)
	}
	result, err := service.CreateNode(context.Background(), request)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	return result.Node
}

func TestNodeOntologyAcceptsEveryRequiredKind(t *testing.T) {
	service := newTestService()
	kinds := []NodeKind{
		NodePerson, NodeOrganization, NodeProject, NodeGoal, NodeTask, NodeEvent,
		NodeDocument, NodeSource, NodeClaim, NodePreference, NodeDecision,
		NodeObligation, NodeDeadline, NodePlace, NodeAccount, NodeCapability,
	}
	for _, kind := range kinds {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			createTestNode(t, service, "owner", kind, "node "+string(kind))
		})
	}
}

func TestOwnerIsolationCoversReadsRetrievalAndEdges(t *testing.T) {
	service := newTestService()
	aliceNode := createTestNode(t, service, "alice", NodeProject, "Private project")
	bobNode := createTestNode(t, service, "bob", NodeTask, "Bob task")

	if _, err := service.GetNode(context.Background(), "bob", aliceNode.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get must be not found, got %v", err)
	}
	if _, err := service.CreateEdge(context.Background(), CreateEdgeRequest{
		OwnerIdentity:      "alice",
		FromNodeID:         aliceNode.ID,
		ToNodeID:           bobNode.ID,
		Relationship:       RelationDependsOn,
		Confidence:         0.8,
		VerificationStatus: VerificationSourceSupported,
		Sensitivity:        SensitivityInternal,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner edge must be rejected as not found, got %v", err)
	}

	result, err := service.Retrieve(context.Background(), RetrieveRequest{
		OwnerIdentity:  "bob",
		Query:          "private project",
		AllowLocalOnly: true,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(result.Nodes) != 0 {
		t.Fatalf("cross-owner node leaked into retrieval: %#v", result.Nodes)
	}
}

func TestDuplicateNodesMergeEvidenceWithoutLosingProvenance(t *testing.T) {
	service := newTestService()
	source := createTestNode(t, service, "owner", NodeSource, "Calendar export")

	first, err := service.CreateNode(context.Background(), CreateNodeRequest{
		OwnerIdentity:      "owner",
		Kind:               NodePreference,
		DeduplicationKey:   "meeting-time",
		Label:              "Preferred meeting time",
		Content:            "Morning",
		ProjectKeys:        []string{"work"},
		Tags:               []string{"calendar"},
		Confidence:         0.6,
		VerificationStatus: VerificationUnverified,
		Sensitivity:        SensitivityInternal,
		Sources:            []SourceReference{{ID: "source-a", URI: "file:///calendar.ics"}},
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := service.CreateNode(context.Background(), CreateNodeRequest{
		OwnerIdentity:      "owner",
		Kind:               NodePreference,
		DeduplicationKey:   "meeting-time",
		Label:              "Preferred meeting time",
		Content:            "Morning",
		ProjectKeys:        []string{"personal"},
		Tags:               []string{"preference"},
		Confidence:         0.9,
		VerificationStatus: VerificationHumanApproved,
		Sensitivity:        SensitivitySensitive,
		Sources:            []SourceReference{{ID: "source-b", SourceNodeID: source.ID}},
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if second.Action != WriteMerged {
		t.Fatalf("expected merge, got %s", second.Action)
	}
	if second.Node.ID != first.Node.ID {
		t.Fatalf("merge created a second identity: %s != %s", second.Node.ID, first.Node.ID)
	}
	if len(second.Node.Sources) != 2 {
		t.Fatalf("expected both provenance records, got %#v", second.Node.Sources)
	}
	if second.Node.Confidence != 0.9 || second.Node.VerificationStatus != VerificationHumanApproved {
		t.Fatalf("stronger evidence was not retained: %#v", second.Node)
	}
	if second.Node.Sensitivity != SensitivitySensitive {
		t.Fatalf("stronger sensitivity was not retained: %s", second.Node.Sensitivity)
	}
	if len(second.Node.ProjectKeys) != 2 || len(second.Node.Tags) != 2 {
		t.Fatalf("merge lost project or tag context: %#v", second.Node)
	}
}

func TestConflictingMemoriesArePreservedAndLinked(t *testing.T) {
	service := newTestService()
	first, err := service.CreateNode(context.Background(), CreateNodeRequest{
		OwnerIdentity:      "owner",
		Kind:               NodePreference,
		DeduplicationKey:   "contact-tone",
		Label:              "Contact tone",
		Content:            "Formal",
		Confidence:         0.8,
		VerificationStatus: VerificationHumanApproved,
		Sensitivity:        SensitivityInternal,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := service.CreateNode(context.Background(), CreateNodeRequest{
		OwnerIdentity:      "owner",
		Kind:               NodePreference,
		DeduplicationKey:   "contact-tone",
		Label:              "Contact tone",
		Content:            "Informal",
		Confidence:         0.7,
		VerificationStatus: VerificationSourceSupported,
		Sensitivity:        SensitivityInternal,
	})
	if err != nil {
		t.Fatalf("conflicting create: %v", err)
	}
	if second.Action != WriteConflict || len(second.ConflictingNodeIDs) != 1 || second.ConflictingNodeIDs[0] != first.Node.ID {
		t.Fatalf("conflict was not reported: %#v", second)
	}
	refreshedFirst, err := service.GetNode(context.Background(), "owner", first.Node.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if refreshedFirst.VerificationStatus != VerificationConflicting ||
		second.Node.VerificationStatus != VerificationConflicting ||
		refreshedFirst.ConflictGroupID == "" ||
		refreshedFirst.ConflictGroupID != second.Node.ConflictGroupID {
		t.Fatalf("conflict group/status not preserved: first=%#v second=%#v", refreshedFirst, second.Node)
	}
	nodes, err := service.ListNodes(context.Background(), "owner", ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("conflicting facts must both remain, got %d", len(nodes))
	}
}

func TestSourceNodeProvenanceMustExistForOwnerAndHaveSourceKind(t *testing.T) {
	service := newTestService()
	otherSource := createTestNode(t, service, "other", NodeSource, "Other source")
	ordinaryNode := createTestNode(t, service, "owner", NodeTask, "Not a source")

	cases := []struct {
		name         string
		sourceNodeID string
		want         string
	}{
		{name: "cross owner", sourceNodeID: otherSource.ID, want: "not available to owner"},
		{name: "wrong kind", sourceNodeID: ordinaryNode.ID, want: "must be a source or document"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := service.CreateNode(context.Background(), CreateNodeRequest{
				OwnerIdentity:      "owner",
				Kind:               NodeClaim,
				Label:              "Claim",
				Confidence:         0.5,
				VerificationStatus: VerificationSourceSupported,
				Sensitivity:        SensitivityInternal,
				Sources:            []SourceReference{{SourceNodeID: test.sourceNodeID}},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestRetrievalIsBoundedAndExplainsTraversal(t *testing.T) {
	service := newTestService()
	nodes := []Node{
		createTestNode(t, service, "owner", NodeProject, "Alpha project", func(r *CreateNodeRequest) {
			r.ProjectKeys = []string{"alpha"}
			r.Confidence = 1
			r.VerificationStatus = VerificationVerified
		}),
		createTestNode(t, service, "owner", NodeGoal, "Release goal"),
		createTestNode(t, service, "owner", NodeTask, "Build task"),
		createTestNode(t, service, "owner", NodeDocument, "Remote document"),
	}
	for i := 0; i < len(nodes)-1; i++ {
		_, err := service.CreateEdge(context.Background(), CreateEdgeRequest{
			OwnerIdentity:      "owner",
			FromNodeID:         nodes[i].ID,
			ToNodeID:           nodes[i+1].ID,
			Relationship:       RelationDependsOn,
			Confidence:         1,
			VerificationStatus: VerificationVerified,
			Sensitivity:        SensitivityInternal,
		})
		if err != nil {
			t.Fatalf("create edge %d: %v", i, err)
		}
	}

	result, err := service.Retrieve(context.Background(), RetrieveRequest{
		OwnerIdentity:  "owner",
		Query:          "alpha",
		MaxDepth:       2,
		Limit:          10,
		AllowLocalOnly: true,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(result.Nodes) != 3 {
		t.Fatalf("depth two should return root plus two neighbors, got %d: %#v", len(result.Nodes), result.Nodes)
	}
	if result.Nodes[0].Node.ID != nodes[0].ID || !result.Nodes[0].Explanation.DirectMatch {
		t.Fatalf("root explanation is wrong: %#v", result.Nodes[0])
	}
	deepest := result.Nodes[2].Explanation
	if deepest.Depth != 2 || deepest.DirectMatch || len(deepest.Path) != 5 ||
		!strings.Contains(deepest.Summary, "bounded graph traversal") {
		t.Fatalf("traversal explanation is incomplete: %#v", deepest)
	}
	for _, retrieved := range result.Nodes {
		if retrieved.Node.ID == nodes[3].ID {
			t.Fatal("depth-three node escaped the bound")
		}
	}

	limited, err := service.Retrieve(context.Background(), RetrieveRequest{
		OwnerIdentity:  "owner",
		Query:          "alpha",
		MaxDepth:       5,
		Limit:          2,
		AllowLocalOnly: true,
	})
	if err != nil {
		t.Fatalf("limited retrieve: %v", err)
	}
	if len(limited.Nodes) != 2 || !limited.Truncated {
		t.Fatalf("limit was not enforced: %#v", limited)
	}
}

func TestRetrievalFiltersLocalOnlyAndExpiredKnowledge(t *testing.T) {
	service := newTestService()
	expiredAt := testNow.Add(-time.Hour)
	createTestNode(t, service, "owner", NodeDocument, "Alpha local notes", func(r *CreateNodeRequest) {
		r.LocalOnly = true
	})
	createTestNode(t, service, "owner", NodeDocument, "Alpha expired notes", func(r *CreateNodeRequest) {
		r.ValidUntil = &expiredAt
	})
	visible := createTestNode(t, service, "owner", NodeDocument, "Alpha public notes")
	sourceLocal := createTestNode(t, service, "owner", NodeClaim, "Alpha source-local claim", func(r *CreateNodeRequest) {
		r.Sources = []SourceReference{{URI: "file:///private.txt", LocalOnly: true}}
	})
	if !sourceLocal.LocalOnly {
		t.Fatal("local-only provenance must make the derived node local-only")
	}

	external, err := service.Retrieve(context.Background(), RetrieveRequest{
		OwnerIdentity:  "owner",
		Query:          "alpha",
		AllowLocalOnly: false,
	})
	if err != nil {
		t.Fatalf("external retrieval: %v", err)
	}
	if len(external.Nodes) != 1 || external.Nodes[0].Node.ID != visible.ID {
		t.Fatalf("local or expired knowledge leaked: %#v", external.Nodes)
	}

	local, err := service.Retrieve(context.Background(), RetrieveRequest{
		OwnerIdentity:  "owner",
		Query:          "alpha",
		AllowLocalOnly: true,
	})
	if err != nil {
		t.Fatalf("local retrieval: %v", err)
	}
	if len(local.Nodes) != 3 {
		t.Fatalf("local retrieval should include both local records but not expired record, got %d", len(local.Nodes))
	}
}

func TestCorrectionPreservesHistoryAndArchivesOldNode(t *testing.T) {
	service := newTestService()
	old := createTestNode(t, service, "owner", NodeDecision, "Deployment target", func(r *CreateNodeRequest) {
		r.Content = "Cloud"
		r.DeduplicationKey = "deployment-target"
		r.Sources = []SourceReference{{URI: "file:///decision-1.txt"}}
	})
	corrected, err := service.CorrectNode(context.Background(), "owner", old.ID, CreateNodeRequest{
		Content:            "Local Windows device",
		Confidence:         1,
		VerificationStatus: VerificationHumanApproved,
		Sources:            []SourceReference{{URI: "file:///decision-2.txt"}},
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Action != WriteCorrected || corrected.Node.SupersedesID != old.ID {
		t.Fatalf("correction relationship missing: %#v", corrected)
	}
	refreshedOld, err := service.GetNode(context.Background(), "owner", old.ID)
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	if !refreshedOld.Archived || refreshedOld.CorrectedByID != corrected.Node.ID {
		t.Fatalf("old record did not preserve correction link: %#v", refreshedOld)
	}
	if len(corrected.Node.Sources) != 2 {
		t.Fatalf("correction lost provenance history: %#v", corrected.Node.Sources)
	}
	active, err := service.ListNodes(context.Background(), "owner", ListOptions{})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 || active[0].ID != corrected.Node.ID {
		t.Fatalf("archived original should not be active: %#v", active)
	}
}

func TestDeleteNodeEmitsPropagationSignalAndTombstonesEdges(t *testing.T) {
	service := newTestService()
	center := createTestNode(t, service, "owner", NodePerson, "Robert")
	left := createTestNode(t, service, "owner", NodeProject, "Left")
	right := createTestNode(t, service, "owner", NodeGoal, "Right")
	var edgeIDs []string
	for _, target := range []Node{left, right} {
		result, err := service.CreateEdge(context.Background(), CreateEdgeRequest{
			OwnerIdentity:      "owner",
			FromNodeID:         center.ID,
			ToNodeID:           target.ID,
			Relationship:       RelationRelatedTo,
			Confidence:         0.8,
			VerificationStatus: VerificationSourceSupported,
			Sensitivity:        SensitivityInternal,
		})
		if err != nil {
			t.Fatalf("create edge: %v", err)
		}
		edgeIDs = append(edgeIDs, result.Edge.ID)
	}

	signal, err := service.DeleteNode(context.Background(), "owner", center.ID, "user requested erasure")
	if err != nil {
		t.Fatalf("delete node: %v", err)
	}
	if signal.EntityType != EntityNode || signal.EntityID != center.ID ||
		len(signal.PropagatedEdgeIDs) != 2 ||
		signal.PropagatedEdgeIDs[0] != edgeIDs[0] ||
		signal.PropagatedEdgeIDs[1] != edgeIDs[1] {
		t.Fatalf("propagation signal is incomplete: %#v", signal)
	}
	activeNodes, _ := service.ListNodes(context.Background(), "owner", ListOptions{})
	for _, node := range activeNodes {
		if node.ID == center.ID {
			t.Fatal("deleted node remains active")
		}
	}
	activeEdges, _ := service.ListEdges(context.Background(), "owner", ListOptions{})
	if len(activeEdges) != 0 {
		t.Fatalf("incident edges remain active: %#v", activeEdges)
	}
	tombstones, err := service.ListEdges(context.Background(), "owner", ListOptions{IncludeArchived: true, IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tombstones: %v", err)
	}
	if len(tombstones) != 2 || tombstones[0].DeletedAt == nil || tombstones[1].DeletedAt == nil {
		t.Fatalf("edges were not tombstoned: %#v", tombstones)
	}
	signals, err := service.ListDeletionSignals(context.Background(), "owner")
	if err != nil || len(signals) != 1 {
		t.Fatalf("deletion signal not retained: %#v err=%v", signals, err)
	}
	otherSignals, err := service.ListDeletionSignals(context.Background(), "other")
	if err != nil || len(otherSignals) != 0 {
		t.Fatalf("deletion signal leaked across owners: %#v err=%v", otherSignals, err)
	}
}

func TestRepositoryIDsAndCopiesAreDeterministic(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo, func() time.Time { return testNow })
	first := createTestNode(t, service, "owner", NodeTask, "First")
	second := createTestNode(t, service, "owner", NodeTask, "Second")
	if first.ID != "node-000001" || second.ID != "node-000002" {
		t.Fatalf("unexpected deterministic ids: %s %s", first.ID, second.ID)
	}
	first.Properties = map[string]string{"mutated": "outside"}
	stored, err := service.GetNode(context.Background(), "owner", first.ID)
	if err != nil {
		t.Fatalf("get stored: %v", err)
	}
	if _, leaked := stored.Properties["mutated"]; leaked {
		t.Fatal("repository returned aliased mutable state")
	}
}
