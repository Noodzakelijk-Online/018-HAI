package ambientmonitor

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidationAcceptsOnlyClosedSourceKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []SourceKind{SourceWorkflowOpenLoopCount, SourceWorkflowVerifiedCompletionCount, SourceOverdueCommitmentCount} {
		if err := validateSourceKind(kind); err != nil {
			t.Errorf("validateSourceKind(%q) = %v", kind, err)
		}
	}
	for _, kind := range []SourceKind{"sql", "workflow_open_loop_count; DROP TABLE workflows", "script", "https://example.test/count"} {
		if err := validateSourceKind(kind); err == nil {
			t.Errorf("validateSourceKind(%q) succeeded", kind)
		}
	}
}

func TestValidationBoundsIdentifiersDurationsTimesAndValues(t *testing.T) {
	t.Parallel()
	for _, identifier := range []string{"", " space", "has space", "target;drop", strings.Repeat("a", maxIdentifierLength+1), "target\nnext"} {
		if err := validateIdentifier("test", identifier); err == nil {
			t.Errorf("identifier %q accepted", identifier)
		}
	}
	for _, cadence := range []time.Duration{0, time.Second, minCadence - time.Second, maxCadence + time.Second, minCadence + time.Millisecond} {
		if err := validateCadence(cadence); err == nil {
			t.Errorf("cadence %v accepted", cadence)
		}
	}
	for _, lease := range []time.Duration{0, time.Second, minLeaseDuration - time.Second, maxLeaseDuration + time.Second, minLeaseDuration + time.Millisecond} {
		if err := validateLeaseDuration(lease); err == nil {
			t.Errorf("lease %v accepted", lease)
		}
	}
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	for _, value := range []float64{-1, 0.5, math.NaN(), math.Inf(1), maxCountValue + 1} {
		if _, err := validateCollected(CollectedObservation{Value: value, ObservedAt: now, SourceDigest: strings.Repeat("a", 64)}, now); err == nil {
			t.Errorf("value %v accepted", value)
		}
	}
	if _, err := validateCollected(CollectedObservation{Value: 1, ObservedAt: now.Add(6 * time.Minute), SourceDigest: strings.Repeat("a", 64)}, now); err == nil {
		t.Fatal("future observation accepted")
	}
	if _, err := validateTime("test", time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("out-of-range time accepted")
	}
}

func TestRecordValidationRejectsAnyAuthorityCapability(t *testing.T) {
	t.Parallel()
	base := advisoryAuthority()
	mutations := []func(*AuthorityControl){func(v *AuthorityControl) { v.CanExecute = true }, func(v *AuthorityControl) { v.CanDeliver = true }, func(v *AuthorityControl) { v.CanNotify = true }, func(v *AuthorityControl) { v.CanWriteCalendar = true }, func(v *AuthorityControl) { v.CanMutateWorkflow = true }, func(v *AuthorityControl) { v.CanAuthorizeMandate = true }, func(v *AuthorityControl) { v.CanMutateLearning = true }}
	for index, mutate := range mutations {
		value := base
		mutate(&value)
		if err := validateAuthority(value); err == nil {
			t.Errorf("authority mutation %d accepted: %+v", index, value)
		}
	}
	wrong := base
	wrong.Label = "autonomous"
	if err := validateAuthority(wrong); err == nil {
		t.Fatal("wrong authority label accepted")
	}
}

func TestExactDigestIsStableAndSensitiveToSemanticInput(t *testing.T) {
	t.Parallel()
	left, err := exactDigest("operation", struct {
		ID    string
		Value int
	}{"target", 1})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := exactDigest("operation", struct {
		ID    string
		Value int
	}{"target", 1})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := exactDigest("operation", struct {
		ID    string
		Value int
	}{"target", 2})
	if err != nil {
		t.Fatal(err)
	}
	if left != replay || left == changed || len(left) != 64 {
		t.Fatalf("unexpected digests: %q %q %q", left, replay, changed)
	}
}
