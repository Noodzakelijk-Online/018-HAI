package executionapproval

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/executionauth"

	"github.com/google/uuid"
)

func TestWorkflowApprovalResolverReturnsServerDerivedApproval(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 123, time.UTC)
	bindingDigest := strings.Repeat("a", 64)
	decision := validWorkflowDecision("alice", bindingDigest, now.Add(-time.Minute))
	repository := &stubWorkflowApprovalRepository{decision: &decision}
	resolver := testWorkflowResolver(t, repository, now)

	approval, err := resolver.Resolve(
		context.Background(),
		"alice",
		workflowDecisionPrefix+decision.DecisionID,
		bindingDigest,
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if approval.SourceID != workflowDecisionPrefix+decision.DecisionID ||
		approval.DecisionID != decision.DecisionID ||
		approval.BindingDigest != bindingDigest ||
		approval.ApprovedBy != "alice" ||
		len(approval.ApproverRoles) != 1 ||
		approval.ApproverRoles[0] != "owner" ||
		!approval.ApprovedAt.Equal(decision.CreatedAt) ||
		!approval.ExpiresAt.Equal(decision.CreatedAt.Add(approvalFreshnessLimit)) ||
		!validSHA256Digest(approval.DecisionDigest) {
		t.Fatalf("resolved approval = %#v", approval)
	}
	if repository.calls != 1 ||
		repository.owner != "alice" ||
		repository.decisionID != decision.DecisionID {
		t.Fatalf("repository lookup = %#v", repository)
	}
}

func TestWorkflowApprovalResolverAcceptsExactReadOnlyAPIApproval(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 123, time.UTC)
	bindingDigest := strings.Repeat("7", 64)
	decision := validWorkflowDecision("alice", bindingDigest, now.Add(-time.Minute))
	decision.ActionBinding = "automation-action:automation.api.read:" + bindingDigest
	resolver := testWorkflowResolver(
		t,
		&stubWorkflowApprovalRepository{decision: &decision, ownerScoped: true},
		now,
	)

	approval, err := resolver.Resolve(
		context.Background(),
		"alice",
		workflowDecisionPrefix+decision.DecisionID,
		bindingDigest,
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if approval.SourceID != workflowDecisionPrefix+decision.DecisionID ||
		approval.BindingDigest != bindingDigest ||
		approval.ApprovedBy != "alice" ||
		!approval.ApprovedAt.Equal(decision.CreatedAt) {
		t.Fatalf("resolved approval = %#v", approval)
	}
}

func TestWorkflowApprovalResolverRejectsInvalidAndInventedReferences(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("b", 64)
	decision := validWorkflowDecision("alice", digest, now)
	resolver := testWorkflowResolver(t, &stubWorkflowApprovalRepository{}, now)
	id := decision.DecisionID
	invalid := []string{
		"",
		"task-review:" + id,
		workflowDecisionPrefix + "not-a-uuid",
		workflowDecisionPrefix + strings.ToUpper(id),
		workflowDecisionPrefix + "{" + id + "}",
		workflowDecisionPrefix + uuid.Nil.String(),
		workflowDecisionPrefix + id + " ",
	}
	for _, sourceID := range invalid {
		t.Run(sourceID, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), "alice", sourceID, digest)
			if !errors.Is(err, ErrInvalidWorkflowReference) {
				t.Fatalf("Resolve(%q) error = %v", sourceID, err)
			}
		})
	}
	_, err := resolver.Resolve(
		context.Background(),
		"alice",
		workflowDecisionPrefix+uuid.NewString(),
		digest,
	)
	if !errors.Is(err, ErrWorkflowApprovalUnavailable) {
		t.Fatalf("invented decision error = %v", err)
	}
}

func TestWorkflowApprovalResolverOwnerScopesLookupAndRecord(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("c", 64)
	decision := validWorkflowDecision("alice", digest, now)
	repository := &stubWorkflowApprovalRepository{
		decision:    &decision,
		ownerScoped: true,
	}
	resolver := testWorkflowResolver(t, repository, now)

	_, err := resolver.Resolve(
		context.Background(),
		"bob",
		workflowDecisionPrefix+decision.DecisionID,
		digest,
	)
	if !errors.Is(err, ErrWorkflowApprovalUnavailable) {
		t.Fatalf("cross-owner lookup error = %v", err)
	}

	repository.ownerScoped = false
	_, err = resolver.Resolve(
		context.Background(),
		"bob",
		workflowDecisionPrefix+decision.DecisionID,
		digest,
	)
	if !errors.Is(err, ErrInvalidWorkflowDecision) {
		t.Fatalf("malicious cross-owner record error = %v", err)
	}
}

func TestWorkflowApprovalResolverRequiresApprovedDecision(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("d", 64)
	tests := []struct {
		name   string
		mutate func(*WorkflowApprovalDecisionRecord)
	}{
		{"rejected value", func(value *WorkflowApprovalDecisionRecord) { value.Decision = "rejected" }},
		{"approval flag false", func(value *WorkflowApprovalDecisionRecord) { value.Approved = false }},
		{"wrong decision type", func(value *WorkflowApprovalDecisionRecord) { value.DecisionType = "proposal" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := validWorkflowDecision("alice", digest, now)
			test.mutate(&decision)
			resolver := testWorkflowResolver(
				t,
				&stubWorkflowApprovalRepository{decision: &decision},
				now,
			)
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				workflowDecisionPrefix+decision.DecisionID,
				digest,
			)
			if !errors.Is(err, ErrWorkflowApprovalUnavailable) {
				t.Fatalf("Resolve error = %v", err)
			}
		})
	}
}

func TestWorkflowApprovalResolverRequiresExactImmutableActionDigest(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("e", 64)
	decision := validWorkflowDecision("alice", digest, now)
	resolver := testWorkflowResolver(
		t,
		&stubWorkflowApprovalRepository{decision: &decision},
		now,
	)

	_, err := resolver.Resolve(
		context.Background(),
		"alice",
		workflowDecisionPrefix+decision.DecisionID,
		strings.Repeat("f", 64),
	)
	if !errors.Is(err, ErrWorkflowBindingMismatch) {
		t.Fatalf("changed action digest error = %v", err)
	}

	invalidBindings := []string{
		"",
		"manual approval gate",
		"automation-action:unknown:" + digest,
		"automation-action:automation.script.execute",
		"automation-action:automation.script.execute:" + strings.ToUpper(digest),
		"automation-action:automation.script.execute:" + digest + ":extra",
		" automation-action:automation.script.execute:" + digest,
		"automation-action:automation.script.execute:" + digest + " ",
	}
	for _, binding := range invalidBindings {
		t.Run(binding, func(t *testing.T) {
			value := decision
			value.ActionBinding = binding
			resolver := testWorkflowResolver(
				t,
				&stubWorkflowApprovalRepository{decision: &value},
				now,
			)
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				workflowDecisionPrefix+value.DecisionID,
				digest,
			)
			if !errors.Is(err, ErrInvalidWorkflowDecision) {
				t.Fatalf("binding %q error = %v", binding, err)
			}
		})
	}
}

func TestWorkflowApprovalResolverEnforcesBoundedFreshness(t *testing.T) {
	now := time.Date(2026, 7, 31, 15, 0, 0, 0, time.UTC)
	digest := strings.Repeat("1", 64)
	tests := []struct {
		name      string
		createdAt time.Time
		wantErr   error
	}{
		{"fresh", now.Add(-approvalFreshnessLimit + time.Nanosecond), nil},
		{"boundary expired", now.Add(-approvalFreshnessLimit), ErrStaleWorkflowApproval},
		{"stale", now.Add(-approvalFreshnessLimit - time.Second), ErrStaleWorkflowApproval},
		{"future within skew", now.Add(approvalFutureSkew), nil},
		{"future beyond skew", now.Add(approvalFutureSkew + time.Nanosecond), ErrFutureWorkflowApproval},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := validWorkflowDecision("alice", digest, test.createdAt)
			resolver := testWorkflowResolver(
				t,
				&stubWorkflowApprovalRepository{decision: &decision},
				now,
			)
			approval, err := resolver.Resolve(
				context.Background(),
				"alice",
				workflowDecisionPrefix+decision.DecisionID,
				digest,
			)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Resolve error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if !approval.ApprovedAt.Equal(test.createdAt) {
				t.Fatalf("ApprovedAt = %v, want %v", approval.ApprovedAt, test.createdAt)
			}
		})
	}
}

func TestWorkflowApprovalResolverRejectsMalformedDurableRecords(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("2", 64)
	base := validWorkflowDecision("alice", digest, now)
	tests := []struct {
		name   string
		mutate func(*WorkflowApprovalDecisionRecord)
	}{
		{"empty decision id", func(value *WorkflowApprovalDecisionRecord) { value.DecisionID = "" }},
		{"noncanonical decision id", func(value *WorkflowApprovalDecisionRecord) { value.DecisionID = strings.ToUpper(value.DecisionID) }},
		{"different decision id", func(value *WorkflowApprovalDecisionRecord) { value.DecisionID = uuid.NewString() }},
		{"empty workflow id", func(value *WorkflowApprovalDecisionRecord) { value.WorkflowID = "" }},
		{"noncanonical workflow id", func(value *WorkflowApprovalDecisionRecord) { value.WorkflowID = strings.ToUpper(value.WorkflowID) }},
		{"empty owner", func(value *WorkflowApprovalDecisionRecord) { value.OwnerIdentity = "" }},
		{"owner whitespace", func(value *WorkflowApprovalDecisionRecord) { value.OwnerIdentity = " alice" }},
		{"empty actor", func(value *WorkflowApprovalDecisionRecord) { value.Actor = "" }},
		{"actor mismatch", func(value *WorkflowApprovalDecisionRecord) { value.Actor = "bob" }},
		{"actor whitespace", func(value *WorkflowApprovalDecisionRecord) { value.Actor = "alice " }},
		{"reason too long", func(value *WorkflowApprovalDecisionRecord) {
			value.Reason = strings.Repeat("x", maximumWorkflowReasonRunes+1)
		}},
		{"reason invalid utf8", func(value *WorkflowApprovalDecisionRecord) { value.Reason = string([]byte{0xff}) }},
		{"missing timestamp", func(value *WorkflowApprovalDecisionRecord) { value.CreatedAt = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			resolver := testWorkflowResolver(
				t,
				&stubWorkflowApprovalRepository{decision: &value},
				now,
			)
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				workflowDecisionPrefix+base.DecisionID,
				digest,
			)
			if !errors.Is(err, ErrInvalidWorkflowDecision) {
				t.Fatalf("Resolve error = %v", err)
			}
		})
	}
}

func TestWorkflowApprovalResolverDecisionDigestIsCanonicalAndSensitive(t *testing.T) {
	instant := time.Date(2026, 7, 31, 10, 0, 0, 99, time.UTC)
	decision := validWorkflowDecision("alice", strings.Repeat("3", 64), instant)
	first, err := digestWorkflowDecision(decision)
	if err != nil {
		t.Fatalf("digestWorkflowDecision: %v", err)
	}
	sameInstant := decision
	sameInstant.CreatedAt = instant.In(time.FixedZone("CEST", 2*60*60))
	second, err := digestWorkflowDecision(sameInstant)
	if err != nil || second != first {
		t.Fatalf("timezone-normalized digest = %q, %v; want %q", second, err, first)
	}
	changed := decision
	changed.Reason = "A different immutable decision"
	third, err := digestWorkflowDecision(changed)
	if err != nil {
		t.Fatalf("changed digest: %v", err)
	}
	if third == first {
		t.Fatal("decision digest did not change with immutable evidence")
	}
}

func TestWorkflowApprovalResolverHandlesConstructionContextAndRepositoryFailure(t *testing.T) {
	var typedNil *stubWorkflowApprovalRepository
	if _, err := NewWorkflowApprovalResolver(nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil repository error = %v", err)
	}
	if _, err := NewWorkflowApprovalResolver(typedNil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("typed nil repository error = %v", err)
	}

	now := time.Now().UTC()
	digest := strings.Repeat("4", 64)
	decision := validWorkflowDecision("alice", digest, now)
	sourceID := workflowDecisionPrefix + decision.DecisionID
	notCalled := &stubWorkflowApprovalRepository{decision: &decision}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := testWorkflowResolver(t, notCalled, now)
	if _, err := resolver.Resolve(cancelled, "alice", sourceID, digest); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
	if notCalled.calls != 0 {
		t.Fatalf("repository called %d times for cancelled context", notCalled.calls)
	}

	failing := &stubWorkflowApprovalRepository{err: errors.New("database unavailable")}
	resolver = testWorkflowResolver(t, failing, now)
	if _, err := resolver.Resolve(context.Background(), "alice", sourceID, digest); !errors.Is(err, ErrWorkflowApprovalUnavailable) {
		t.Fatalf("repository failure error = %v", err)
	}
	if _, err := resolver.Resolve(nil, "alice", sourceID, digest); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), " alice", sourceID, digest); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid owner error = %v", err)
	}
	if _, err := resolver.Resolve(
		context.Background(),
		"alice",
		sourceID,
		strings.ToUpper(strings.Repeat("a", 64)),
	); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid digest error = %v", err)
	}
	var nilResolver *WorkflowApprovalResolver
	if _, err := nilResolver.Resolve(context.Background(), "alice", sourceID, digest); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil resolver error = %v", err)
	}
}

type stubWorkflowApprovalRepository struct {
	decision    *WorkflowApprovalDecisionRecord
	err         error
	ownerScoped bool
	calls       int
	owner       string
	decisionID  string
}

func (r *stubWorkflowApprovalRepository) FindWorkflowApprovalDecision(
	ctx context.Context,
	ownerIdentity string,
	decisionID string,
) (*WorkflowApprovalDecisionRecord, error) {
	r.calls++
	r.owner = ownerIdentity
	r.decisionID = decisionID
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.ownerScoped && r.decision != nil && r.decision.OwnerIdentity != ownerIdentity {
		return nil, nil
	}
	return r.decision, nil
}

func testWorkflowResolver(
	t *testing.T,
	repository WorkflowApprovalDecisionRepository,
	now time.Time,
) *WorkflowApprovalResolver {
	t.Helper()
	resolver, err := NewWorkflowApprovalResolver(repository)
	if err != nil {
		t.Fatalf("NewWorkflowApprovalResolver: %v", err)
	}
	resolver.now = func() time.Time { return now }
	return resolver
}

func validWorkflowDecision(
	owner string,
	bindingDigest string,
	createdAt time.Time,
) WorkflowApprovalDecisionRecord {
	return WorkflowApprovalDecisionRecord{
		DecisionID:    uuid.NewString(),
		WorkflowID:    uuid.NewString(),
		OwnerIdentity: owner,
		DecisionType:  workflowDecisionType,
		Decision:      workflowDecisionValue,
		Reason:        "Owner approved the exact immutable automation action",
		ActionBinding: "automation-action:automation.script.execute:" + bindingDigest,
		Approved:      true,
		Actor:         owner,
		CreatedAt:     createdAt,
	}
}

var _ executionauth.ApprovalResolver = (*WorkflowApprovalResolver)(nil)
