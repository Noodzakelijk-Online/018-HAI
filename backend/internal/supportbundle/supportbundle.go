// Package supportbundle assembles a redaction-safe diagnostic bundle for
// support/debugging: build metadata, readiness diagnosis, and operational
// counts. It deliberately includes only check names/severities and caller-
// supplied counts — never secret values.
package supportbundle

import (
	"regexp"
	"sort"
	"strings"

	"automation-hub-backend/internal/buildinfo"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/safety"
)

var safeCountName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// ReadinessSummary is the roll-up of the configuration self-diagnostic.
type ReadinessSummary struct {
	Ready bool           `json:"ready"`
	OK    int            `json:"ok"`
	Warn  int            `json:"warn"`
	Fail  int            `json:"fail"`
	Check []doctor.Check `json:"checks"`
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
	safeChecks := make([]doctor.Check, 0, len(report.Checks))
	for _, check := range report.Checks {
		safeChecks = append(safeChecks, doctor.Check{
			Name:     safeLabel(check.Name),
			Severity: check.Severity,
			// Diagnostic details can contain hostnames, usernames, paths, and
			// configuration values. The support summary needs only status.
			Detail: "",
		})
	}
	copied := make(map[string]int, len(counts))
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := counts[key]
		key = strings.ToLower(strings.TrimSpace(key))
		if value < 0 || !safeCountName.MatchString(key) || sensitiveLabel(key) {
			continue
		}
		copied[key] = value
		if len(copied) == 128 {
			break
		}
	}
	return Bundle{
		Build: build,
		Readiness: ReadinessSummary{
			Ready: fail == 0,
			OK:    ok,
			Warn:  warn,
			Fail:  fail,
			Check: safeChecks,
		},
		Counts: copied,
		Note:   "Contains no secret values: only check names/severities, build metadata, and counts.",
	}
}

func safeLabel(value string) string {
	value = strings.TrimSpace(safety.RedactSecrets(value))
	if value == "" || len(value) > 128 {
		return "redacted"
	}
	return value
}

func sensitiveLabel(value string) bool {
	for _, part := range strings.FieldsFunc(
		strings.ToLower(value),
		func(r rune) bool { return r == '.' || r == '_' || r == '-' },
	) {
		switch part {
		case "authorization", "cookie", "credential", "key", "password",
			"passwd", "secret", "token":
			return true
		}
	}
	return false
}
