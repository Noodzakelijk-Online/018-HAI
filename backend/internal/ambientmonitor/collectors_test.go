package ambientmonitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type fakeCollectorSourceReader struct {
	openLoops           sourceSnapshot
	verifiedCompletions sourceSnapshot
	overdueSnapshot     sourceSnapshot
	err                 error
	called              SourceKind
	owner               string
	at                  time.Time
}

func (reader *fakeCollectorSourceReader) workflowOpenLoops(_ context.Context, owner string, at time.Time) (sourceSnapshot, error) {
	reader.called, reader.owner, reader.at = SourceWorkflowOpenLoopCount, owner, at
	return reader.openLoops, reader.err
}

func (reader *fakeCollectorSourceReader) workflowVerifiedCompletions(_ context.Context, owner string) (sourceSnapshot, error) {
	reader.called, reader.owner = SourceWorkflowVerifiedCompletionCount, owner
	return reader.verifiedCompletions, reader.err
}

func (reader *fakeCollectorSourceReader) overdueCommitments(_ context.Context, owner string, at time.Time) (sourceSnapshot, error) {
	reader.called, reader.owner, reader.at = SourceOverdueCommitmentCount, owner, at
	return reader.overdueSnapshot, reader.err
}

func TestGormCollectorDispatchesClosedSourceKinds(t *testing.T) {
	clock := time.Date(2026, time.August, 5, 12, 30, 0, 123456789, time.FixedZone("CEST", 2*60*60))
	observedAt := clock.UTC().Truncate(time.Microsecond)
	due := observedAt.Add(-time.Hour)
	tests := []struct {
		name     string
		kind     SourceKind
		snapshot sourceSnapshot
	}{
		{name: "open loops", kind: SourceWorkflowOpenLoopCount, snapshot: sourceSnapshot{Count: 75, Records: []sourceSnapshotRecord{{
			RecordID: "loop-1", ParentID: "workflow-1", State: "open", DueAt: &due, SourceAt: observedAt.Add(-time.Minute),
		}}}},
		{name: "verified completions", kind: SourceWorkflowVerifiedCompletionCount, snapshot: sourceSnapshot{Count: 4, Records: []sourceSnapshotRecord{{
			RecordID: "attestation-1", ParentID: "workflow-1", State: "verified", RecordDigest: strings.Repeat("a", 64), SourceAt: observedAt.Add(-time.Minute),
		}}}},
		{name: "overdue commitments", kind: SourceOverdueCommitmentCount, snapshot: sourceSnapshot{Count: 2, Records: []sourceSnapshotRecord{{
			RecordID: "commitment-1", State: "active", RecordDigest: strings.Repeat("b", 64), Revision: 2, DueAt: &due, SourceAt: observedAt.Add(-time.Minute),
		}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeCollectorSourceReader{}
			switch test.kind {
			case SourceWorkflowOpenLoopCount:
				reader.openLoops = test.snapshot
			case SourceWorkflowVerifiedCompletionCount:
				reader.verifiedCompletions = test.snapshot
			case SourceOverdueCommitmentCount:
				reader.overdueSnapshot = test.snapshot
			}
			collector := newCollectorWithReader(reader, func() time.Time { return clock })
			result, err := collector.Collect(context.Background(), collectorTarget(test.kind))
			if err != nil {
				t.Fatalf("Collect() error = %v", err)
			}
			if reader.called != test.kind || reader.owner != "owner-a" {
				t.Fatalf("reader call = kind %q owner %q", reader.called, reader.owner)
			}
			if test.kind != SourceWorkflowVerifiedCompletionCount && !reader.at.Equal(observedAt) {
				t.Fatalf("reader time = %s, want %s", reader.at, observedAt)
			}
			if result.Value != float64(test.snapshot.Count) || !result.ObservedAt.Equal(observedAt) {
				t.Fatalf("result = %#v", result)
			}
			if len(result.SourceDigest) != 64 {
				t.Fatalf("digest = %q", result.SourceDigest)
			}
		})
	}
}

func TestGormCollectorDigestIsDeterministicAndSnapshotSensitive(t *testing.T) {
	now := time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
	base := sourceSnapshot{Count: 1, Records: []sourceSnapshotRecord{{
		RecordID: "attestation-1", ParentID: "workflow-1", State: "verified",
		RecordDigest: strings.Repeat("a", 64), SourceAt: now.Add(-time.Minute),
	}}}
	reader := &fakeCollectorSourceReader{verifiedCompletions: base}
	collector := newCollectorWithReader(reader, func() time.Time { return now })
	target := collectorTarget(SourceWorkflowVerifiedCompletionCount)
	first, err := collector.Collect(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	second, err := collector.Collect(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceDigest != second.SourceDigest {
		t.Fatalf("same snapshot produced %q and %q", first.SourceDigest, second.SourceDigest)
	}
	reader.verifiedCompletions.Records[0].RecordDigest = strings.Repeat("b", 64)
	changed, err := collector.Collect(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SourceDigest == first.SourceDigest {
		t.Fatal("changed source record did not change digest")
	}
}

func TestGormCollectorRejectsUnsupportedOrUnsafeInputs(t *testing.T) {
	reader := &fakeCollectorSourceReader{}
	collector := newCollectorWithReader(reader, func() time.Time {
		return time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
	})
	target := collectorTarget(SourceKind("SELECT * FROM secrets"))
	if _, err := collector.Collect(context.Background(), target); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsupported source error = %v", err)
	}
	if reader.called != "" {
		t.Fatalf("reader called for unsupported source: %q", reader.called)
	}
	target = collectorTarget(SourceWorkflowOpenLoopCount)
	target.Scope.OwnerID = "owner-a' OR TRUE --"
	if _, err := collector.Collect(context.Background(), target); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe owner error = %v", err)
	}
}

func TestGormCollectorFailsClosedForInvalidSnapshotsAndReadErrors(t *testing.T) {
	now := time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
	reader := &fakeCollectorSourceReader{openLoops: sourceSnapshot{Count: 2, Records: []sourceSnapshotRecord{{
		RecordID: "loop-1", State: "open", SourceAt: now,
	}}}}
	collector := newCollectorWithReader(reader, func() time.Time { return now })
	if _, err := collector.Collect(context.Background(), collectorTarget(SourceWorkflowOpenLoopCount)); !errors.Is(err, ErrCollectorFailed) {
		t.Fatalf("invalid snapshot error = %v", err)
	}
	reader.err = errors.New("database unavailable")
	if _, err := collector.Collect(context.Background(), collectorTarget(SourceWorkflowOpenLoopCount)); !errors.Is(err, ErrCollectorFailed) {
		t.Fatalf("read error = %v", err)
	}
}

func TestGormCollectorHonorsCancellationAndConstructorGuards(t *testing.T) {
	if _, err := NewGormCollector(nil); !errors.Is(err, ErrCollectorUnavailable) {
		t.Fatalf("nil database error = %v", err)
	}
	if _, err := NewGormCollector(&gorm.DB{}, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil clock error = %v", err)
	}
	collector := newCollectorWithReader(&fakeCollectorSourceReader{}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collector.Collect(ctx, collectorTarget(SourceWorkflowOpenLoopCount)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}

func TestCollectorSQLIsFixedOwnerScopedAndReadOnly(t *testing.T) {
	assertSQL := func(name, query string, required ...string) {
		t.Helper()
		upper := strings.ToUpper(query)
		for _, forbidden := range []string{"INSERT ", "UPDATE ", "DELETE ", "TRUNCATE ", "EXECUTE ", "%S", "%Q"} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("%s contains forbidden SQL fragment %q", name, forbidden)
			}
		}
		for _, fragment := range required {
			if !strings.Contains(query, fragment) {
				t.Fatalf("%s missing %q", name, fragment)
			}
		}
	}
	assertSQL("open-loop count", workflowOpenLoopCountSQL,
		"workflow_items.owner_identity = ?", "workflow_open_loops.status = 'open'",
		"workflow_open_loops.follow_up_at <= ?", "workflow_items.archived = FALSE",
		"workflow_items.current_state NOT IN ('archived', 'completed')", "COUNT(*)::bigint")
	assertSQL("open-loop snapshot", workflowOpenLoopSnapshotSQL,
		"workflow_items.owner_identity = ?", "workflow_open_loops.status = 'open'",
		"workflow_open_loops.follow_up_at <= ?", "workflow_items.archived = FALSE",
		"workflow_items.current_state NOT IN ('archived', 'completed')", "LIMIT 256")
	assertSQL("completion count", workflowVerifiedCompletionCountSQL,
		"owner_identity = ?", "completion_status = 'completed'", "('verified', 'test_passed')")
	assertSQL("completion snapshot", workflowVerifiedCompletionSnapshotSQL,
		"owner_identity = ?", "record_digest", "LIMIT 256")
	assertSQL("commitment count", overdueCommitmentCountSQL,
		"owner_identity = ?", "DISTINCT ON (commitment_key)", "revision DESC",
		"(payload ->> 'dueAt')::timestamptz < ?", "('proposed', 'active', 'waiting', 'breached', 'disputed')")
	assertSQL("commitment snapshot", overdueCommitmentSnapshotSQL,
		"owner_identity = ?", "DISTINCT ON (commitment_key)", "record_digest", "LIMIT 256")
	if strings.Contains(overdueCommitmentCountSQL, "fulfilled") || strings.Contains(overdueCommitmentCountSQL, "cancelled") {
		t.Fatal("terminal commitment status entered overdue query")
	}
}

func collectorTarget(kind SourceKind) MonitorTarget {
	return MonitorTarget{
		ContractVersion: ContractVersion,
		Scope:           Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"},
		SourceKind:      kind,
	}
}
