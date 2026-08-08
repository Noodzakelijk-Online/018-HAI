package knowledgegraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var claimTestNow = time.Date(2026, time.July, 31, 18, 0, 0, 0, time.UTC)

func claimTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func claimTestRequest(object string) RecordClaimRequest {
	return RecordClaimRequest{
		OwnerIdentity:      "robert",
		WorkspaceID:        "hai",
		Subject:            "018-HAI",
		Predicate:          "release status",
		Object:             object,
		EffectiveFrom:      claimTestNow.Add(-time.Hour),
		ObservedAt:         claimTestNow.Add(-30 * time.Minute),
		VerificationStatus: VerificationSourceSupported,
		Provenance: []ClaimProvenance{{
			ReferenceID: "build-42", ContentDigest: claimTestDigest("build output"),
			Authority: "ci", CapturedAt: claimTestNow.Add(-45 * time.Minute),
		}},
		Sensitivity: SensitivityInternal,
	}
}

func claimTestService(repo Repository) *Service {
	return NewService(repo, func() time.Time { return claimTestNow })
}

func recordClaim(t *testing.T, service *Service, request RecordClaimRequest) Claim {
	t.Helper()
	claim, err := service.RecordClaim(context.Background(), request)
	if err != nil {
		t.Fatalf("RecordClaim: %v", err)
	}
	return claim
}

func TestRecordClaimIsDeterministicSourceBackedAndIdempotent(t *testing.T) {
	repo := NewMemoryRepository()
	service := claimTestService(repo)
	request := claimTestRequest("verified locally")
	request.Provenance = append(request.Provenance, ClaimProvenance{
		URI: "file:///report.json", ContentDigest: claimTestDigest("report"),
		CapturedAt: claimTestNow.Add(-40 * time.Minute), LocalOnly: true,
	})

	first := recordClaim(t, service, request)
	reversed := request
	reversed.Provenance = []ClaimProvenance{request.Provenance[1], request.Provenance[0]}
	second := recordClaim(t, service, reversed)

	if first.ID != second.ID || first.ClaimDigest != second.ClaimDigest {
		t.Fatalf("canonical source order changed identity: first=%#v second=%#v", first, second)
	}
	if !bareSHA256Pattern.MatchString(first.ClaimDigest) || !bareSHA256Pattern.MatchString(first.ProvenanceDigest) {
		t.Fatalf("claim digests are not canonical: %#v", first)
	}
	if first.ID != claimID(first.ClaimDigest) || !first.LocalOnly {
		t.Fatalf("claim identity/locality not derived from signed envelope: %#v", first)
	}
	stored, err := service.GetClaim(context.Background(), "robert", "hai", first.ID)
	if err != nil || stored.ClaimDigest != first.ClaimDigest || len(stored.Provenance) != 2 {
		t.Fatalf("stored claim mismatch: %#v err=%v", stored, err)
	}
	all, err := service.ListClaims(context.Background(), "robert", "hai", ClaimQuery{})
	if err != nil || len(all) != 1 {
		t.Fatalf("idempotent append created duplicates: %#v err=%v", all, err)
	}
}

func TestRecordClaimRejectsSecretFactsAndRedactsProvenanceURLs(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	secret := claimTestRequest("api_key=top-secret")
	if _, err := service.RecordClaim(context.Background(), secret); err == nil {
		t.Fatal("secret-bearing claim object was accepted")
	}

	request := claimTestRequest("verified locally")
	request.Provenance[0].URI =
		"https://operator:password@example.test/evidence?token=secret-value&record=42"
	claim := recordClaim(t, service, request)
	uri := claim.Provenance[0].URI
	if strings.Contains(uri, "operator:") || strings.Contains(uri, "secret-value") {
		t.Fatalf("provenance URI retained credentials: %q", uri)
	}
	if !strings.Contains(uri, "record=42") {
		t.Fatalf("provenance URI lost non-sensitive identity: %q", uri)
	}
}

func TestClaimReadsAreOwnerAndWorkspaceScoped(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	claim := recordClaim(t, service, claimTestRequest("scoped"))

	for _, scope := range []struct{ owner, workspace string }{
		{owner: "other", workspace: "hai"},
		{owner: "robert", workspace: "other"},
	} {
		if _, err := service.GetClaim(context.Background(), scope.owner, scope.workspace, claim.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("scope %q/%q leaked claim: %v", scope.owner, scope.workspace, err)
		}
		claims, err := service.ListClaims(context.Background(), scope.owner, scope.workspace, ClaimQuery{})
		if err != nil || len(claims) != 0 {
			t.Fatalf("scope %q/%q leaked list: %#v err=%v", scope.owner, scope.workspace, claims, err)
		}
	}
}

func TestClaimQueryAppliesEffectiveObservedStatusAndLimitBounds(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	oldRequest := claimTestRequest("old")
	oldRequest.EffectiveFrom = claimTestNow.Add(-4 * time.Hour)
	oldUntil := claimTestNow.Add(-2 * time.Hour)
	oldRequest.EffectiveUntil = &oldUntil
	oldRequest.ObservedAt = claimTestNow.Add(-3 * time.Hour)
	oldRequest.Provenance[0].CapturedAt = claimTestNow.Add(-3*time.Hour - time.Minute)
	recordClaim(t, service, oldRequest)

	currentRequest := claimTestRequest("current")
	currentRequest.VerificationStatus = VerificationVerified
	current := recordClaim(t, service, currentRequest)

	effectiveAt := claimTestNow.Add(-30 * time.Minute)
	observedBy := claimTestNow.Add(-20 * time.Minute)
	claims, err := service.ListClaims(context.Background(), "robert", "hai", ClaimQuery{
		EffectiveAt: &effectiveAt, ObservedBy: &observedBy,
		VerificationStatuses: []VerificationStatus{VerificationVerified}, Limit: 1,
	})
	if err != nil || len(claims) != 1 || claims[0].ID != current.ID {
		t.Fatalf("temporal query mismatch: %#v err=%v", claims, err)
	}
	for _, invalidLimit := range []int{-1, maximumClaimLimit + 1} {
		if _, err := service.ListClaims(context.Background(), "robert", "hai", ClaimQuery{Limit: invalidLimit}); err == nil {
			t.Fatalf("limit %d did not fail closed", invalidLimit)
		}
	}
	if _, err := service.ListClaims(context.Background(), "robert", "hai", ClaimQuery{
		VerificationStatuses: []VerificationStatus{"invented"},
	}); err == nil {
		t.Fatal("unknown verification status did not fail closed")
	}
}

func TestClaimLifecyclePreservesSupersessionAndBidirectionalConflictViews(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	firstRequest := claimTestRequest("pending")
	firstRequest.ObservedAt = claimTestNow.Add(-2 * time.Hour)
	firstRequest.Provenance[0].CapturedAt = claimTestNow.Add(-3 * time.Hour)
	first := recordClaim(t, service, firstRequest)

	replacementRequest := claimTestRequest("passed")
	replacementRequest.SupersedesClaimIDs = []string{first.ID}
	replacement := recordClaim(t, service, replacementRequest)

	conflictRequest := claimTestRequest("failed")
	conflictRequest.ConflictsWithIDs = []string{replacement.ID}
	conflictRequest.ObservedAt = claimTestNow.Add(-10 * time.Minute)
	conflict := recordClaim(t, service, conflictRequest)

	replacementLifecycle, err := service.GetClaimLifecycle(context.Background(), "robert", "hai", replacement.ID)
	if err != nil || len(replacementLifecycle.Supersedes) != 1 || replacementLifecycle.Supersedes[0].ID != first.ID {
		t.Fatalf("supersession missing: %#v err=%v", replacementLifecycle, err)
	}
	if len(replacementLifecycle.Conflicts) != 1 || replacementLifecycle.Conflicts[0].ID != conflict.ID {
		t.Fatalf("reverse conflict missing: %#v", replacementLifecycle)
	}
	firstLifecycle, err := service.GetClaimLifecycle(context.Background(), "robert", "hai", first.ID)
	if err != nil || len(firstLifecycle.SupersededBy) != 1 || firstLifecycle.SupersededBy[0].ID != replacement.ID {
		t.Fatalf("reverse supersession missing: %#v err=%v", firstLifecycle, err)
	}
}

func TestClaimValidationFailsClosed(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	tests := []struct {
		name   string
		mutate func(*RecordClaimRequest)
	}{
		{name: "not atomic", mutate: func(r *RecordClaimRequest) { r.Predicate = "" }},
		{name: "no effective time", mutate: func(r *RecordClaimRequest) { r.EffectiveFrom = time.Time{} }},
		{name: "future observed", mutate: func(r *RecordClaimRequest) { r.ObservedAt = claimTestNow.Add(time.Second) }},
		{name: "no provenance", mutate: func(r *RecordClaimRequest) { r.Provenance = nil }},
		{name: "prefixed digest", mutate: func(r *RecordClaimRequest) { r.Provenance[0].ContentDigest = "sha256:" + r.Provenance[0].ContentDigest }},
		{name: "uppercase digest", mutate: func(r *RecordClaimRequest) {
			r.Provenance[0].ContentDigest = strings.ToUpper(r.Provenance[0].ContentDigest)
		}},
		{name: "unknown status", mutate: func(r *RecordClaimRequest) { r.VerificationStatus = "trusted_by_model" }},
		{name: "invalid interval", mutate: func(r *RecordClaimRequest) { until := r.EffectiveFrom; r.EffectiveUntil = &until }},
		{name: "ambiguous provenance identity", mutate: func(r *RecordClaimRequest) {
			r.Provenance = append(r.Provenance, r.Provenance[0])
			r.Provenance[1].ContentDigest = claimTestDigest("different content")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := claimTestRequest("invalid")
			test.mutate(&request)
			if _, err := service.RecordClaim(context.Background(), request); err == nil {
				t.Fatal("invalid claim was accepted")
			}
		})
	}
}

func TestClaimLinksAndSourceNodesCannotCrossScope(t *testing.T) {
	repo := NewMemoryRepository()
	service := claimTestService(repo)
	otherWorkspace := claimTestRequest("other workspace")
	otherWorkspace.WorkspaceID = "other"
	otherClaim := recordClaim(t, service, otherWorkspace)

	request := claimTestRequest("bad link")
	request.SupersedesClaimIDs = []string{otherClaim.ID}
	if _, err := service.RecordClaim(context.Background(), request); err == nil || !strings.Contains(err.Error(), "owner workspace") {
		t.Fatalf("cross-workspace link error = %v", err)
	}

	otherService := claimTestService(repo)
	source := createTestNode(t, otherService, "other-owner", NodeSource, "private source")
	request = claimTestRequest("bad source")
	request.Provenance[0].ReferenceID = ""
	request.Provenance[0].SourceNodeID = source.ID
	if _, err := service.RecordClaim(context.Background(), request); err == nil || !strings.Contains(err.Error(), "not available to owner") {
		t.Fatalf("cross-owner source error = %v", err)
	}
}

func TestClaimEnvelopeAndProvenanceAreImmutable(t *testing.T) {
	repo := NewMemoryRepository()
	service := claimTestService(repo)
	claim := recordClaim(t, service, claimTestRequest("immutable"))
	node, err := service.GetNode(context.Background(), "robert", claim.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	node.Sources[0].ContentHash = claimTestDigest("tampered")
	node.UpdatedAt = claimTestNow.Add(time.Minute)
	if _, err := repo.UpdateNode(context.Background(), node); !errors.Is(err, ErrImmutableClaim) {
		t.Fatalf("provenance mutation error = %v, want ErrImmutableClaim", err)
	}
	if _, err := service.CorrectNode(context.Background(), "robert", claim.ID, CreateNodeRequest{Content: "edited"}); !errors.Is(err, ErrImmutableClaim) {
		t.Fatalf("generic correction error = %v, want ErrImmutableClaim", err)
	}
	archived, err := service.ArchiveNode(context.Background(), "robert", claim.ID, true)
	if err != nil || !archived.Archived {
		t.Fatalf("archive metadata should remain available: %#v err=%v", archived, err)
	}
}

func TestClaimReadRejectsCorruptStoredEnvelope(t *testing.T) {
	repo := NewMemoryRepository()
	service := claimTestService(repo)
	claim := recordClaim(t, service, claimTestRequest("corrupt me"))

	repo.mu.Lock()
	node := repo.nodes[claim.ID]
	node.Properties[claimPropertyDigest] = claimTestDigest("forged")
	repo.nodes[claim.ID] = node
	repo.mu.Unlock()

	if _, err := service.GetClaim(context.Background(), "robert", "hai", claim.ID); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("corrupt claim error = %v, want ErrCorruptStorage", err)
	}
}

func TestClaimReadRejectsUnsignedStoredProperties(t *testing.T) {
	repo := NewMemoryRepository()
	service := claimTestService(repo)
	claim := recordClaim(t, service, claimTestRequest("no hidden controls"))

	repo.mu.Lock()
	node := repo.nodes[claim.ID]
	node.Properties["execute_allowed"] = "true"
	repo.nodes[claim.ID] = node
	repo.mu.Unlock()

	if _, err := service.GetClaim(context.Background(), "robert", "hai", claim.ID); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("unsigned property error = %v, want ErrCorruptStorage", err)
	}
}

func TestClaimContractContainsNoExecutionAuthority(t *testing.T) {
	claim := recordClaim(t, claimTestService(NewMemoryRepository()), claimTestRequest("advisory fact"))
	encoded, err := json.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"executionauthority", "approvalgranted", "executeallowed"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("claim contract exposed authority field %q: %s", forbidden, encoded)
		}
	}
}
