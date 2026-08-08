package executionapproval

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/task"

	"github.com/google/uuid"
)

func TestTaskReviewResolverReturnsServerDerivedApproval(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repository, decision := createDurableDecision(t, "alice", "approved", now.Add(-time.Minute))
	resolver := testResolver(t, repository, now)

	approval, err := resolver.Resolve(
		context.Background(),
		"alice",
		decision.ApprovalSourceID,
		decision.RequestDigest,
	)
	if err != nil {
		t.Fatalf("resolve durable approval: %v", err)
	}
	if approval.SourceID != decision.ApprovalSourceID ||
		approval.DecisionID != decision.ID ||
		approval.BindingDigest != decision.RequestDigest ||
		approval.ApprovedBy != "alice" ||
		!approval.ApprovedAt.Equal(decision.ResolvedAt) ||
		!approval.ExpiresAt.Equal(decision.ResolvedAt.Add(approvalFreshnessLimit)) {
		t.Fatalf("resolved approval does not match durable decision: %#v", approval)
	}
	if !validSHA256Digest(approval.DecisionDigest) {
		t.Fatalf("decision digest is not a lowercase SHA-256 digest: %q", approval.DecisionDigest)
	}
	if len(approval.ApproverRoles) != 1 || approval.ApproverRoles[0] != "owner" {
		t.Fatalf("approver roles = %#v, want owner", approval.ApproverRoles)
	}

	repeated, err := resolver.Resolve(
		context.Background(),
		"alice",
		decision.ApprovalSourceID,
		decision.RequestDigest,
	)
	if err != nil {
		t.Fatalf("repeat resolution: %v", err)
	}
	if !reflect.DeepEqual(repeated, approval) {
		t.Fatalf("repeat resolution changed immutable evidence:\nfirst  %#v\nsecond %#v", approval, repeated)
	}
}

func TestTaskReviewResolverRejectsInvalidAndInventedReferences(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repository, decision := createDurableDecision(t, "alice", "approved", now.Add(-time.Minute))
	resolver := testResolver(t, repository, now)
	id := strings.TrimPrefix(decision.ApprovalSourceID, taskReviewPrefix)

	for _, sourceID := range []string{
		"",
		id,
		"workflow-review:" + id,
		taskReviewPrefix + "not-a-uuid",
		taskReviewPrefix + strings.ToUpper(id),
		taskReviewPrefix + "{" + id + "}",
		taskReviewPrefix + uuid.Nil.String(),
		taskReviewPrefix + id + " ",
	} {
		t.Run(sourceID, func(t *testing.T) {
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				sourceID,
				decision.RequestDigest,
			)
			if !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("Resolve(%q) error = %v, want invalid reference", sourceID, err)
			}
		})
	}

	invented := taskReviewPrefix + uuid.NewString()
	if _, err := resolver.Resolve(
		context.Background(),
		"alice",
		invented,
		decision.RequestDigest,
	); !errors.Is(err, ErrApprovalUnavailable) {
		t.Fatalf("invented approval error = %v, want unavailable", err)
	}
}

func TestTaskReviewResolverRejectsCrossOwnerAndRejectedDecision(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)

	repository, approved := createDurableDecision(t, "alice", "approved", now.Add(-time.Minute))
	resolver := testResolver(t, repository, now)
	if _, err := resolver.Resolve(
		context.Background(),
		"bob",
		approved.ApprovalSourceID,
		approved.RequestDigest,
	); !errors.Is(err, ErrApprovalUnavailable) {
		t.Fatalf("cross-owner approval error = %v, want unavailable", err)
	}

	rejectedRepository, rejected := createDurableDecision(
		t,
		"alice",
		"rejected",
		now.Add(-time.Minute),
	)
	rejectedResolver := testResolver(t, rejectedRepository, now)
	if _, err := rejectedResolver.Resolve(
		context.Background(),
		"alice",
		rejected.ApprovalSourceID,
		rejected.RequestDigest,
	); !errors.Is(err, ErrApprovalUnavailable) {
		t.Fatalf("rejected approval error = %v, want unavailable", err)
	}
}

func TestTaskReviewResolverRequiresExactBindingDigest(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repository, decision := createDurableDecision(t, "alice", "approved", now.Add(-time.Minute))
	resolver := testResolver(t, repository, now)

	for name, binding := range map[string]string{
		"empty":     "",
		"short":     "abc",
		"uppercase": strings.ToUpper(decision.RequestDigest),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				decision.ApprovalSourceID,
				binding,
			)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("invalid binding error = %v, want invalid request", err)
			}
		})
	}

	otherDigest := strings.Repeat("a", 64)
	if otherDigest == decision.RequestDigest {
		otherDigest = strings.Repeat("b", 64)
	}
	if _, err := resolver.Resolve(
		context.Background(),
		"alice",
		decision.ApprovalSourceID,
		otherDigest,
	); !errors.Is(err, ErrBindingMismatch) {
		t.Fatalf("mismatched binding error = %v, want binding mismatch", err)
	}
}

func TestTaskReviewResolverEnforcesBoundedFreshness(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		resolvedAt time.Time
		wantErr    error
	}{
		{
			name:       "inside freshness window",
			resolvedAt: now.Add(-approvalFreshnessLimit).Add(time.Millisecond),
		},
		{
			name:       "expires exactly at freshness limit",
			resolvedAt: now.Add(-approvalFreshnessLimit),
			wantErr:    ErrStaleApproval,
		},
		{
			name:       "older than freshness limit",
			resolvedAt: now.Add(-approvalFreshnessLimit - time.Second),
			wantErr:    ErrStaleApproval,
		},
		{
			name:       "future within clock skew",
			resolvedAt: now.Add(approvalFutureSkew),
		},
		{
			name:       "future beyond clock skew",
			resolvedAt: now.Add(approvalFutureSkew + time.Millisecond),
			wantErr:    ErrFutureApproval,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, decision := createDurableDecision(
				t,
				"alice",
				"approved",
				test.resolvedAt,
			)
			resolver := testResolver(t, repository, now)
			approval, err := resolver.Resolve(
				context.Background(),
				"alice",
				decision.ApprovalSourceID,
				decision.RequestDigest,
			)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Resolve error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve error = %v", err)
			}
			if !approval.ExpiresAt.Equal(test.resolvedAt.UTC().Add(approvalFreshnessLimit)) {
				t.Fatalf(
					"expiry = %s, want %s",
					approval.ExpiresAt,
					test.resolvedAt.UTC().Add(approvalFreshnessLimit),
				)
			}
		})
	}
}

func TestTaskReviewResolverRejectsMalformedDurableDecision(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	reviewID := uuid.New()
	sourceID := taskReviewPrefix + reviewID.String()
	base := validDecision(reviewID, "alice", now.Add(-time.Minute))

	tests := []struct {
		name    string
		mutate  func(*task.ReviewDecisionRecord)
		wantErr error
	}{
		{
			name:    "missing decision",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ID = "" },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "noncanonical decision",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ID = strings.ToUpper(value.ID) },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "wrong review item",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ReviewItemID = uuid.NewString() },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "zero revision",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ReviewRevision = 0 },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "blank task plan",
			mutate:  func(value *task.ReviewDecisionRecord) { value.TaskPlanID = "" },
			wantErr: ErrInvalidDecision,
		},
		{
			name: "oversized task plan",
			mutate: func(value *task.ReviewDecisionRecord) {
				value.TaskPlanID = strings.Repeat("p", maximumTaskPlanRunes+1)
			},
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "rejected projection",
			mutate:  func(value *task.ReviewDecisionRecord) { value.Decision = "rejected" },
			wantErr: ErrApprovalUnavailable,
		},
		{
			name:    "blank approver",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ResolvedBy = "" },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "different approver",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ResolvedBy = "bob" },
			wantErr: ErrInvalidDecision,
		},
		{
			name: "oversized resolution note",
			mutate: func(value *task.ReviewDecisionRecord) {
				value.ResolutionNote = strings.Repeat("n", maximumNoteRunes+1)
			},
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "wrong source",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ApprovalSource = "caller" },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "wrong source id",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ApprovalSourceID = taskReviewPrefix + uuid.NewString() },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "invalid request digest",
			mutate:  func(value *task.ReviewDecisionRecord) { value.RequestDigest = "invalid" },
			wantErr: ErrInvalidDecision,
		},
		{
			name:    "missing resolution time",
			mutate:  func(value *task.ReviewDecisionRecord) { value.ResolvedAt = time.Time{} },
			wantErr: ErrInvalidDecision,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			resolver := testResolver(t, &stubDecisionRepository{decision: &value}, now)
			_, err := resolver.Resolve(
				context.Background(),
				"alice",
				sourceID,
				base.RequestDigest,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Resolve error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestTaskReviewResolverDecisionDigestIsCanonicalAndSensitive(t *testing.T) {
	reviewID := uuid.MustParse("11111111-2222-4333-8444-555555555555")
	decision := validDecision(
		reviewID,
		"alice",
		time.Date(2026, 7, 31, 10, 0, 0, 123456000, time.UTC),
	)
	decision.ID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	first, err := digestReviewDecision(decision)
	if err != nil {
		t.Fatalf("digest decision: %v", err)
	}
	sameInstant := decision
	sameInstant.ResolvedAt = decision.ResolvedAt.In(time.FixedZone("CEST", 2*60*60))
	second, err := digestReviewDecision(sameInstant)
	if err != nil {
		t.Fatalf("digest same instant: %v", err)
	}
	if first != second {
		t.Fatalf("same immutable instant changed digest: %s != %s", first, second)
	}

	changed := decision
	changed.ResolutionNote = decision.ResolutionNote + " changed"
	third, err := digestReviewDecision(changed)
	if err != nil {
		t.Fatalf("digest changed decision: %v", err)
	}
	if third == first {
		t.Fatal("changing an immutable decision field did not change its digest")
	}
	if !validSHA256Digest(first) || !validSHA256Digest(third) {
		t.Fatalf("invalid decision digests: %q %q", first, third)
	}
}

func TestTaskReviewResolverHandlesContextAndRepositoryFailures(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	reviewID := uuid.New()
	sourceID := taskReviewPrefix + reviewID.String()
	binding := strings.Repeat("a", 64)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	notCalled := &stubDecisionRepository{}
	resolver := testResolver(t, notCalled, now)
	if _, err := resolver.Resolve(cancelled, "alice", sourceID, binding); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
	if notCalled.calls != 0 {
		t.Fatalf("repository called %d times for cancelled request", notCalled.calls)
	}

	backendFailure := errors.New("database unavailable")
	failing := &stubDecisionRepository{err: backendFailure}
	resolver = testResolver(t, failing, now)
	if _, err := resolver.Resolve(
		context.Background(),
		"alice",
		sourceID,
		binding,
	); !errors.Is(err, ErrApprovalUnavailable) ||
		!errors.Is(err, backendFailure) ||
		!strings.Contains(err.Error(), backendFailure.Error()) {
		t.Fatalf("repository failure error = %v", err)
	}
}

func TestTaskReviewResolverValidatesConstructionAndOwner(t *testing.T) {
	if _, err := NewTaskReviewResolver(nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil repository error = %v", err)
	}
	var typedNil *stubDecisionRepository
	if _, err := NewTaskReviewResolver(typedNil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("typed nil repository error = %v", err)
	}

	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repository, decision := createDurableDecision(t, "alice", "approved", now.Add(-time.Minute))
	resolver := testResolver(t, repository, now)
	for _, owner := range []string{
		"",
		" alice",
		"alice ",
		strings.Repeat("a", maximumOwnerBytes+1),
		string([]byte{0xff}),
	} {
		_, err := resolver.Resolve(
			context.Background(),
			owner,
			decision.ApprovalSourceID,
			decision.RequestDigest,
		)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("owner %q error = %v, want invalid request", owner, err)
		}
	}

	if _, err := resolver.Resolve(nil, "alice", decision.ApprovalSourceID, decision.RequestDigest); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil context error = %v", err)
	}
	var nilResolver *TaskReviewResolver
	if _, err := nilResolver.Resolve(
		context.Background(),
		"alice",
		decision.ApprovalSourceID,
		decision.RequestDigest,
	); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil resolver error = %v", err)
	}
}

type stubDecisionRepository struct {
	decision *task.ReviewDecisionRecord
	err      error
	calls    int
	owner    string
	reviewID string
}

func (r *stubDecisionRepository) FindApprovedReviewDecision(
	ownerIdentity string,
	reviewItemID string,
) (*task.ReviewDecisionRecord, error) {
	r.calls++
	r.owner = ownerIdentity
	r.reviewID = reviewItemID
	if r.err != nil {
		return nil, r.err
	}
	return r.decision, nil
}

func testResolver(
	t *testing.T,
	repository TaskReviewDecisionRepository,
	now time.Time,
) *TaskReviewResolver {
	t.Helper()
	resolver, err := NewTaskReviewResolver(repository)
	if err != nil {
		t.Fatalf("new task review resolver: %v", err)
	}
	resolver.now = func() time.Time { return now }
	return resolver
}

func createDurableDecision(
	t *testing.T,
	owner string,
	decision string,
	resolvedAt time.Time,
) (*task.MemoryTaskStateRepository, task.ReviewDecisionRecord) {
	t.Helper()
	repository := task.NewMemoryTaskStateRepository()
	item := task.ReviewQueueItem{
		ID:     uuid.NewString(),
		TaskID: "task-plan-1",
		Request: task.IntakeRequest{
			OwnerIdentity: owner,
			Request:       "Execute the reviewed deployment",
			ProjectKey:    "project",
			AutomationID:  "automation",
		},
		Reason:    "human approval is required",
		Priority:  "high",
		Status:    "open",
		CreatedAt: resolvedAt.Add(-time.Minute),
	}
	stored, err := repository.CreateReviewItem(owner, item)
	if err != nil {
		t.Fatalf("create durable review: %v", err)
	}
	resolution, err := repository.ResolveReviewItem(owner, stored.ID, task.ReviewResolution{
		Decision:   decision,
		Note:       "Reviewed by the owner",
		ResolvedAt: resolvedAt,
	})
	if err != nil {
		t.Fatalf("resolve durable review: %v", err)
	}
	return repository, resolution.Decision
}

func validDecision(
	reviewID uuid.UUID,
	owner string,
	resolvedAt time.Time,
) task.ReviewDecisionRecord {
	return task.ReviewDecisionRecord{
		ID:               uuid.NewString(),
		ReviewItemID:     reviewID.String(),
		ReviewRevision:   1,
		TaskPlanID:       "task-plan-1",
		Decision:         "approved",
		ResolutionNote:   "Reviewed by the owner",
		ResolvedBy:       owner,
		ApprovalSource:   taskReviewSource,
		ApprovalSourceID: taskReviewPrefix + reviewID.String(),
		RequestDigest:    strings.Repeat("a", 64),
		ResolvedAt:       resolvedAt,
	}
}
