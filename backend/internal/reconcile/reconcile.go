// Package reconcile scans stored records for broken invariants and proposes
// safe repairs. The scan is pure (no I/O): callers load records, run Scan, and
// decide whether to apply the proposed repairs.
package reconcile

import (
	"fmt"

	"automation-hub-backend/internal/invariants"
	"automation-hub-backend/internal/models"
)

// Finding is one record that violates an invariant, with a proposed repair.
type Finding struct {
	MemoryID   string                  `json:"memoryId"`
	Violations []invariants.Violation  `json:"violations"`
	Repair     string                  `json:"repair"`
	Repairable bool                    `json:"repairable"`
}

// Report summarizes a reconciliation scan.
type Report struct {
	Scanned  int       `json:"scanned"`
	Findings []Finding `json:"findings"`
}

// Clean reports whether the scan found no violations.
func (r Report) Clean() bool { return len(r.Findings) == 0 }

// ScanMemories checks each memory against its invariants and proposes a repair
// where one is safe to derive automatically.
func ScanMemories(memories []models.ContextMemory) Report {
	report := Report{Scanned: len(memories)}
	for _, m := range memories {
		v := invariants.ValidateMemory(m)
		if invariants.Valid(v) {
			continue
		}
		report.Findings = append(report.Findings, Finding{
			MemoryID:   m.ID.String(),
			Violations: v,
			Repair:     proposeRepair(m, v),
			Repairable: repairable(v),
		})
	}
	return report
}

// repairable reports whether every violation has a safe automatic repair.
// Only out-of-range confidence and over-long tags can be safely auto-repaired;
// missing required content/kind need human input.
func repairable(violations []invariants.Violation) bool {
	for _, viol := range violations {
		switch viol.Field {
		case "confidence", "tags":
			continue
		default:
			return false
		}
	}
	return true
}

func proposeRepair(m models.ContextMemory, violations []invariants.Violation) string {
	if repairable(violations) {
		return fmt.Sprintf("clamp confidence to [0,1] and/or truncate tags to 512 chars for memory %s", m.ID)
	}
	return "requires manual input: fill missing required fields (content/kind)"
}
