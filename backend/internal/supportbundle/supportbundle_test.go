package supportbundle

import (
	"testing"

	"automation-hub-backend/internal/buildinfo"
	"automation-hub-backend/internal/doctor"
)

func TestBuildSummarizesReadinessAndCounts(t *testing.T) {
	report := doctor.Report{Checks: []doctor.Check{
		{Name: "database.host", Severity: doctor.SeverityOK},
		{Name: "security.backendApiKey", Severity: doctor.SeverityWarn},
	}}
	counts := map[string]int{"memories": 12}
	bundle := Build(report, buildinfo.Snapshot(), counts)

	if !bundle.Readiness.Ready || bundle.Readiness.OK != 1 || bundle.Readiness.Warn != 1 {
		t.Fatalf("readiness summary wrong: %+v", bundle.Readiness)
	}
	if bundle.Counts["memories"] != 12 {
		t.Fatalf("counts not carried: %+v", bundle.Counts)
	}
	// Mutating the caller's map must not affect the bundle.
	counts["memories"] = 999
	if bundle.Counts["memories"] != 12 {
		t.Fatalf("bundle counts should be an independent copy")
	}
}

func TestBuildMarksNotReadyOnFailure(t *testing.T) {
	report := doctor.Report{Checks: []doctor.Check{{Name: "database.host", Severity: doctor.SeverityFail}}}
	bundle := Build(report, buildinfo.Snapshot(), nil)
	if bundle.Readiness.Ready {
		t.Fatalf("a failing check must mark the bundle not ready")
	}
}
