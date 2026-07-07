// Package supportbundle assembles a redaction-safe diagnostic bundle for
// support/debugging: build metadata, readiness diagnosis, and operational
// counts. It deliberately includes only check names/severities and caller-
// supplied counts — never secret values.
package supportbundle

import (
	"automation-hub-backend/internal/buildinfo"
	"automation-hub-backend/internal/doctor"
)

// ReadinessSummary is the roll-up of the configuration self-diagnostic.
type ReadinessSummary struct {
	Ready bool            `json:"ready"`
	OK    int             `json:"ok"`
	Warn  int             `json:"warn"`
	Fail  int             `json:"fail"`
	Check []doctor.Check  `json:"checks"`
}

// Bundle is a portable support snapshot.
type Bundle struct {
	Build     buildinfo.Info   `json:"build"`
	Readiness ReadinessSummary `json:"readiness"`
	Counts    map[string]int   `json:"counts"`
	Note      string           `json:"note"`
}

// Build assembles a bundle from a readiness report, build info, and operational
// counts (e.g. number of memories, workflows). Counts are copied so the input
// map is never retained.
func Build(report doctor.Report, build buildinfo.Info, counts map[string]int) Bundle {
	ok, warn, fail := report.Counts()
	copied := make(map[string]int, len(counts))
	for k, v := range counts {
		copied[k] = v
	}
	return Bundle{
		Build: build,
		Readiness: ReadinessSummary{
			Ready: fail == 0,
			OK:    ok,
			Warn:  warn,
			Fail:  fail,
			Check: report.Checks,
		},
		Counts: copied,
		Note:   "Contains no secret values: only check names/severities, build metadata, and counts.",
	}
}
