package executionauth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"automation-hub-backend/internal/sourceevidence"
)

type fakeSourceEvidenceRepository struct {
	mu       sync.Mutex
	snapshot sourceevidence.Snapshot
	err      error
	calls    int
	onCall   func(int, sourceevidence.Snapshot) (sourceevidence.Snapshot, error)
}

func (f *fakeSourceEvidenceRepository) Resolve(
	_ context.Context,
	ownerIdentity string,
	extractionID string,
) (sourceevidence.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.onCall != nil {
		return f.onCall(f.calls, f.snapshot)
	}
	if f.snapshot.OwnerIdentity != ownerIdentity || f.snapshot.ExtractionID != extractionID {
		return sourceevidence.Snapshot{}, sourceevidence.ErrNotFound
	}
	return f.snapshot, f.err
}

func exactSourceEvidence(now time.Time) (sourceevidence.Snapshot, sourceevidence.Claim) {
	snapshot := sourceevidence.Snapshot{
		OwnerIdentity:           "alice",
		ExtractionID:            "11111111-1111-4111-8111-111111111111",
		SourceID:                "22222222-2222-4222-8222-222222222222",
		RawItemID:               "33333333-3333-4333-8333-333333333333",
		ProjectKey:              "018-hai",
		RawProjectKey:           "018-hai",
		ExtractionURI:           "local://evidence/item-1",
		RawItemURI:              "local://evidence/item-1",
		ExtractionHash:          strings.Repeat("a", 64),
		RawItemHash:             strings.Repeat("a", 64),
		ExtractionPayloadDigest: strings.Repeat("b", 64),
		FetchedAt:               now.Add(-time.Hour),
		ExtractionAt:            now.Add(-30 * time.Minute),
		LocalOnly:               true,
		ConnectorKey:            "local-folder",
	}
	snapshot.SnapshotDigest = sourceevidence.SnapshotDigest(snapshot)
	claim := sourceevidence.Claim{
		RequirementID:  "fer-source-1",
		Validator:      sourceevidence.ValidatorFreshSource,
		ExtractionID:   snapshot.ExtractionID,
		SourceID:       snapshot.SourceID,
		RawItemID:      snapshot.RawItemID,
		SnapshotDigest: snapshot.SnapshotDigest,
		MaxAgeSeconds:  7200,
	}
	return snapshot, claim
}

func sourceAssertionsJSON(t *testing.T, claim sourceevidence.Claim) json.RawMessage {
	t.Helper()
	value, err := json.Marshal([]sourceEvidenceAssertion{{
		RequirementID: claim.RequirementID,
		Validator:     claim.Validator,
		Status:        "verified",
		SourceClaims:  []sourceevidence.Claim{claim},
	}})
	if err != nil {
		t.Fatalf("marshal source assertions: %v", err)
	}
	return value
}

type frameworkEvidencePreflightCall struct {
	ownerIdentity        string
	taskPlanID           string
	frameworkSelectionID string
	preflightDigest      string
}

type fakeFrameworkEvidencePreflightResolver struct {
	mu       sync.Mutex
	snapshot FrameworkEvidencePreflightSnapshot
	err      error
	calls    []frameworkEvidencePreflightCall
	onCall   func(int, FrameworkEvidencePreflightSnapshot) (FrameworkEvidencePreflightSnapshot, error)
}

func (f *fakeFrameworkEvidencePreflightResolver) ResolveFrameworkEvidencePreflight(
	_ context.Context,
	ownerIdentity string,
	taskPlanID string,
	frameworkSelectionID string,
	preflightDigest string,
) (FrameworkEvidencePreflightSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, frameworkEvidencePreflightCall{
		ownerIdentity:        ownerIdentity,
		taskPlanID:           taskPlanID,
		frameworkSelectionID: frameworkSelectionID,
		preflightDigest:      preflightDigest,
	})
	if f.onCall != nil {
		return f.onCall(len(f.calls), f.snapshot)
	}
	return f.snapshot, f.err
}

func matchingFrameworkEvidencePreflight(
	owner string,
	governance GovernanceEvidence,
) FrameworkEvidencePreflightSnapshot {
	return FrameworkEvidencePreflightSnapshot{
		OwnerIdentity:        owner,
		TaskPlanID:           governance.TaskPlanID,
		FrameworkSelectionID: governance.FrameworkSelectionID,
		PreflightDigest:      governance.FrameworkEvidencePreflightDigest,
		Status:               frameworkEvidencePreflightPassed,
		AssertionsJSON:       []byte("[]"),
	}
}

func withMatchingFrameworkEvidencePreflight(
	t *testing.T,
	service *Service,
	governance GovernanceEvidence,
) *fakeFrameworkEvidencePreflightResolver {
	t.Helper()
	resolver := &fakeFrameworkEvidencePreflightResolver{
		snapshot: matchingFrameworkEvidencePreflight("alice", governance),
	}
	if _, err := service.WithFrameworkEvidencePreflightResolver(resolver); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}
	return resolver
}

func withMatchingFrameworkSelectionOnly(
	t *testing.T,
	service *Service,
	governance GovernanceEvidence,
) *fakeFrameworkSelectionResolver {
	t.Helper()
	resolver := &fakeFrameworkSelectionResolver{values: map[string]FrameworkSelectionSnapshot{
		"alice\x00" + governance.FrameworkSelectionID: snapshotFromGovernance(governance),
	}}
	if _, err := service.WithFrameworkSelectionResolver(resolver); err != nil {
		t.Fatalf("WithFrameworkSelectionResolver: %v", err)
	}
	return resolver
}

func TestWithFrameworkEvidencePreflightResolverRejectsNil(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	if _, err := service.WithFrameworkEvidencePreflightResolver(nil); err == nil {
		t.Fatal("WithFrameworkEvidencePreflightResolver accepted nil")
	}
}

func TestSelectorV5PreflightFailsClosedWhenUnavailableOrMissing(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Service, GovernanceEvidence)
	}{
		{name: "resolver unavailable"},
		{
			name: "record missing",
			configure: func(service *Service, governance GovernanceEvidence) {
				resolver := &fakeFrameworkEvidencePreflightResolver{err: ErrNotFound}
				if _, err := service.WithFrameworkEvidencePreflightResolver(resolver); err != nil {
					t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constitution := permissiveConstitution()
			service := newTestService(t, NewMemoryRepository(), constitution, nil, nil)
			request := selectorV5Request("preflight-" + strings.ReplaceAll(test.name, " ", "-"))
			withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
			if test.configure != nil {
				test.configure(service, *request.Governance)
			}

			receipt, err := service.Authorize(context.Background(), request)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			assertUnverifiedFrameworkEvidencePreflight(t, receipt)
			if constitution.calls != 0 {
				t.Fatalf("Constitution calls = %d, want preflight denial first", constitution.calls)
			}
		})
	}
}

func TestSelectorV5PreflightRequiresCanonicalDigest(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	request := selectorV5Request("preflight-digest-missing")
	request.Governance.FrameworkEvidencePreflightDigest = ""
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	resolver := withMatchingFrameworkEvidencePreflight(t, service, *request.Governance)

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	assertUnverifiedFrameworkEvidencePreflight(t, receipt)
	if len(resolver.calls) != 0 {
		t.Fatalf("preflight resolver calls = %d, want zero for missing digest", len(resolver.calls))
	}
}

func TestSelectorV5PreflightRejectsUntrustedResolvedRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FrameworkEvidencePreflightSnapshot)
	}{
		{
			name: "foreign owner",
			mutate: func(snapshot *FrameworkEvidencePreflightSnapshot) {
				snapshot.OwnerIdentity = "bob"
			},
		},
		{
			name: "mismatched task plan",
			mutate: func(snapshot *FrameworkEvidencePreflightSnapshot) {
				snapshot.TaskPlanID = "plan-other"
			},
		},
		{
			name: "mismatched framework selection",
			mutate: func(snapshot *FrameworkEvidencePreflightSnapshot) {
				snapshot.FrameworkSelectionID = "selection-other"
			},
		},
		{
			name: "failed status",
			mutate: func(snapshot *FrameworkEvidencePreflightSnapshot) {
				snapshot.Status = "failed"
			},
		},
		{
			name: "forged digest",
			mutate: func(snapshot *FrameworkEvidencePreflightSnapshot) {
				snapshot.PreflightDigest = strings.Repeat("0", 64)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constitution := permissiveConstitution()
			service := newTestService(t, NewMemoryRepository(), constitution, nil, nil)
			request := selectorV5Request("preflight-reject-" + strings.ReplaceAll(test.name, " ", "-"))
			withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
			snapshot := matchingFrameworkEvidencePreflight("alice", *request.Governance)
			test.mutate(&snapshot)
			resolver := &fakeFrameworkEvidencePreflightResolver{snapshot: snapshot}
			if _, err := service.WithFrameworkEvidencePreflightResolver(resolver); err != nil {
				t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
			}

			receipt, err := service.Authorize(context.Background(), request)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			assertUnverifiedFrameworkEvidencePreflight(t, receipt)
			if constitution.calls != 0 {
				t.Fatalf("Constitution calls = %d, want preflight denial first", constitution.calls)
			}
		})
	}
}

func TestSelectorV5PreflightUsesOwnerScopedPassedRecord(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	request := selectorV5Request("preflight-verified")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	resolver := withMatchingFrameworkEvidencePreflight(t, service, *request.Governance)

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if receipt.Outcome != OutcomeAuthorized ||
		!receipt.Evidence.FrameworkEvidencePreflight.OwnerScoped ||
		!receipt.Evidence.FrameworkEvidencePreflight.Verified ||
		receipt.Evidence.FrameworkEvidencePreflight.Digest !=
			request.Governance.FrameworkEvidencePreflightDigest {
		t.Fatalf("verified preflight receipt = %#v", receipt)
	}
	if len(resolver.calls) != 1 {
		t.Fatalf("preflight resolver calls = %d, want 1", len(resolver.calls))
	}
	call := resolver.calls[0]
	if call.ownerIdentity != request.OwnerIdentity ||
		call.taskPlanID != request.Governance.TaskPlanID ||
		call.frameworkSelectionID != request.Governance.FrameworkSelectionID ||
		call.preflightDigest != request.Governance.FrameworkEvidencePreflightDigest {
		t.Fatalf("preflight resolver call = %#v", call)
	}
}

func TestSelectorV5PreflightIndependentlyVerifiesSourceClaims(t *testing.T) {
	service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
	request := selectorV5Request("preflight-source-verified")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	snapshot, claim := exactSourceEvidence(fixedNow())
	preflight := matchingFrameworkEvidencePreflight("alice", *request.Governance)
	preflight.AssertionsJSON = sourceAssertionsJSON(t, claim)
	if _, err := service.WithFrameworkEvidencePreflightResolver(
		&fakeFrameworkEvidencePreflightResolver{snapshot: preflight},
	); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}
	sources := &fakeSourceEvidenceRepository{snapshot: snapshot}
	if _, err := service.WithSourceEvidenceRepository(sources); err != nil {
		t.Fatalf("WithSourceEvidenceRepository: %v", err)
	}

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if receipt.Outcome != OutcomeAuthorized ||
		receipt.Evidence.FrameworkEvidencePreflight.SourceClaimsVerified != 1 ||
		!validDigest(receipt.Evidence.FrameworkEvidencePreflight.SourceClaimsDigest) ||
		sources.calls != 1 {
		t.Fatalf("source-verified receipt = %#v; calls=%d", receipt, sources.calls)
	}
}

func TestSelectorV5VerifiedSourceAssertionWithoutClaimsFailsClosed(t *testing.T) {
	constitution := permissiveConstitution()
	service := newTestService(t, NewMemoryRepository(), constitution, nil, nil)
	request := selectorV5Request("preflight-source-missing-claims")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	preflight := matchingFrameworkEvidencePreflight("alice", *request.Governance)
	preflight.AssertionsJSON = json.RawMessage(`[{"requirementId":"fer-source-1","validator":"fresh_source","status":"verified"}]`)
	if _, err := service.WithFrameworkEvidencePreflightResolver(
		&fakeFrameworkEvidencePreflightResolver{snapshot: preflight},
	); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if receipt.Outcome != OutcomeDenied ||
		!containsFold(receipt.Evidence.ReasonCodes, "source.evidence_unverified") {
		t.Fatalf("missing source claims denial = %#v", receipt)
	}
	if constitution.calls != 0 {
		t.Fatalf("Constitution calls = %d, want source evidence denial first", constitution.calls)
	}
}

func TestSelectorV5MalformedAssertionsJSONFailsClosed(t *testing.T) {
	constitution := permissiveConstitution()
	service := newTestService(t, NewMemoryRepository(), constitution, nil, nil)
	request := selectorV5Request("preflight-source-malformed-assertions")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	preflight := matchingFrameworkEvidencePreflight("alice", *request.Governance)
	preflight.AssertionsJSON = json.RawMessage(`[{"requirementId":"fer-source-1"`)
	if _, err := service.WithFrameworkEvidencePreflightResolver(
		&fakeFrameworkEvidencePreflightResolver{snapshot: preflight},
	); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if receipt.Outcome != OutcomeDenied ||
		!containsFold(receipt.Evidence.ReasonCodes, "source.evidence_unverified") {
		t.Fatalf("malformed assertions denial = %#v", receipt)
	}
	if constitution.calls != 0 {
		t.Fatalf("Constitution calls = %d, want assertions denial first", constitution.calls)
	}
}

func TestSelectorV5SourceEvidenceFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Service, *sourceevidence.Snapshot, *sourceevidence.Claim)
	}{
		{name: "resolver unavailable"},
		{
			name: "foreign owner snapshot",
			configure: func(service *Service, snapshot *sourceevidence.Snapshot, _ *sourceevidence.Claim) {
				snapshot.OwnerIdentity = "bob"
				_, _ = service.WithSourceEvidenceRepository(&fakeSourceEvidenceRepository{snapshot: *snapshot})
			},
		},
		{
			name: "stale raw item",
			configure: func(service *Service, snapshot *sourceevidence.Snapshot, _ *sourceevidence.Claim) {
				snapshot.FetchedAt = fixedNow().Add(-3 * time.Hour)
				_, _ = service.WithSourceEvidenceRepository(&fakeSourceEvidenceRepository{snapshot: *snapshot})
			},
		},
		{
			name: "changed payload digest",
			configure: func(service *Service, snapshot *sourceevidence.Snapshot, _ *sourceevidence.Claim) {
				snapshot.ExtractionPayloadDigest = strings.Repeat("c", 64)
				snapshot.SnapshotDigest = sourceevidence.SnapshotDigest(*snapshot)
				_, _ = service.WithSourceEvidenceRepository(&fakeSourceEvidenceRepository{snapshot: *snapshot})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTestService(t, NewMemoryRepository(), permissiveConstitution(), nil, nil)
			request := selectorV5Request("source-denied-" + strings.ReplaceAll(test.name, " ", "-"))
			withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
			snapshot, claim := exactSourceEvidence(fixedNow())
			preflight := matchingFrameworkEvidencePreflight("alice", *request.Governance)
			preflight.AssertionsJSON = sourceAssertionsJSON(t, claim)
			if _, err := service.WithFrameworkEvidencePreflightResolver(
				&fakeFrameworkEvidencePreflightResolver{snapshot: preflight},
			); err != nil {
				t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
			}
			if test.configure != nil {
				test.configure(service, &snapshot, &claim)
			}

			receipt, err := service.Authorize(context.Background(), request)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if receipt.Outcome != OutcomeDenied ||
				!containsFold(receipt.Evidence.ReasonCodes, "source.evidence_unverified") {
				t.Fatalf("source evidence denial = %#v", receipt)
			}
		})
	}
}

func TestSelectorV5SourceEvidenceIsRecheckedBeforeConsumption(t *testing.T) {
	repository := NewMemoryRepository()
	service := newTestService(t, repository, permissiveConstitution(), nil, nil)
	request := selectorV5Request("source-changed-before-consumption")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	snapshot, claim := exactSourceEvidence(fixedNow())
	preflight := matchingFrameworkEvidencePreflight("alice", *request.Governance)
	preflight.AssertionsJSON = sourceAssertionsJSON(t, claim)
	if _, err := service.WithFrameworkEvidencePreflightResolver(
		&fakeFrameworkEvidencePreflightResolver{snapshot: preflight},
	); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}
	sources := &fakeSourceEvidenceRepository{snapshot: snapshot}
	sources.onCall = func(call int, value sourceevidence.Snapshot) (sourceevidence.Snapshot, error) {
		if call > 1 {
			return sourceevidence.Snapshot{}, sourceevidence.ErrNotFound
		}
		return value, nil
	}
	if _, err := service.WithSourceEvidenceRepository(sources); err != nil {
		t.Fatalf("WithSourceEvidenceRepository: %v", err)
	}

	receipt, err := service.AuthorizeAndConsume(
		context.Background(),
		request,
		"test-worker",
		"workspace-file:brief.txt",
	)
	if !errors.Is(err, ErrAuthorizationChanged) ||
		!strings.Contains(err.Error(), "source.evidence_unverified") {
		t.Fatalf("AuthorizeAndConsume error = %v, want source evidence authorization change", err)
	}
	if receipt.Outcome != OutcomeAuthorized || sources.calls != 2 {
		t.Fatalf("receipt = %#v; source resolver calls = %d", receipt, sources.calls)
	}
	if _, err := repository.GetConsumption(context.Background(), "alice", receipt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetConsumption error = %v, want ErrNotFound", err)
	}
}

func TestSelectorV5PreflightIsRecheckedBeforeConsumption(t *testing.T) {
	repository := NewMemoryRepository()
	service := newTestService(t, repository, permissiveConstitution(), nil, nil)
	request := selectorV5Request("preflight-changed-before-consumption")
	withMatchingFrameworkSelectionOnly(t, service, *request.Governance)
	resolver := &fakeFrameworkEvidencePreflightResolver{
		snapshot: matchingFrameworkEvidencePreflight("alice", *request.Governance),
	}
	resolver.onCall = func(
		call int,
		snapshot FrameworkEvidencePreflightSnapshot,
	) (FrameworkEvidencePreflightSnapshot, error) {
		if call > 1 {
			return FrameworkEvidencePreflightSnapshot{}, ErrNotFound
		}
		return snapshot, nil
	}
	if _, err := service.WithFrameworkEvidencePreflightResolver(resolver); err != nil {
		t.Fatalf("WithFrameworkEvidencePreflightResolver: %v", err)
	}

	receipt, err := service.AuthorizeAndConsume(
		context.Background(),
		request,
		"test-worker",
		"workspace-file:brief.txt",
	)
	if !errors.Is(err, ErrAuthorizationChanged) ||
		!strings.Contains(err.Error(), "framework.evidence_preflight_unverified") {
		t.Fatalf("AuthorizeAndConsume error = %v, want preflight authorization change", err)
	}
	if receipt.Outcome != OutcomeAuthorized || len(resolver.calls) != 2 {
		t.Fatalf("receipt = %#v; preflight resolver calls = %d", receipt, len(resolver.calls))
	}
	if _, err := repository.GetConsumption(context.Background(), "alice", receipt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetConsumption error = %v, want ErrNotFound", err)
	}
}

func TestFrameworkEvidencePreflightInspectionIsBounded(t *testing.T) {
	digest := strings.Repeat("a", 64)
	view := publicEvidence(DecisionEvidence{
		FrameworkEvidencePreflight: FrameworkEvidencePreflightVerificationEvidence{
			Digest:      digest,
			OwnerScoped: true,
			Verified:    true,
		},
	})
	if view.FrameworkEvidencePreflight.Digest != digest[:inspectionFingerprintN] ||
		!view.FrameworkEvidencePreflight.OwnerScoped ||
		!view.FrameworkEvidencePreflight.Verified {
		t.Fatalf("inspection preflight evidence = %#v", view.FrameworkEvidencePreflight)
	}
}

func assertUnverifiedFrameworkEvidencePreflight(t *testing.T, receipt Receipt) {
	t.Helper()
	if receipt.Outcome != OutcomeDenied ||
		!containsFold(receipt.Evidence.ReasonCodes, "framework.evidence_preflight_unverified") ||
		receipt.Evidence.FrameworkEvidencePreflight.Verified {
		t.Fatalf("preflight receipt = %#v", receipt)
	}
}
