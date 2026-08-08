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

func TestBuildRemovesDiagnosticDetailsAndUnsafeCountLabels(t *testing.T) {
	report := doctor.Report{Checks: []doctor.Check{{
		Name:     "database.host",
		Severity: doctor.SeverityOK,
		Detail:   "db.internal.example password=do-not-export",
	}}}
	bundle := Build(report, buildinfo.Snapshot(), map[string]int{
		"memories":          12,
		"api_token_secret":  1,
		"Authorization":     2,
		"invalid count key": 3,
		"negative":          -1,
	})
	if got := bundle.Readiness.Check[0].Detail; got != "" {
		t.Fatalf("diagnostic detail leaked: %q", got)
	}
	if len(bundle.Counts) != 1 || bundle.Counts["memories"] != 12 {
		t.Fatalf("unsafe count labels were retained: %+v", bundle.Counts)
	}
}

func TestBuildDoesNotRetainCallerCheckSlice(t *testing.T) {
	checks := []doctor.Check{{Name: "database.host", Severity: doctor.SeverityOK}}
	bundle := Build(doctor.Report{Checks: checks}, buildinfo.Snapshot(), nil)
	checks[0].Name = "changed"
	if bundle.Readiness.Check[0].Name != "database.host" {
		t.Fatalf("bundle retained caller check slice")
	}
}
