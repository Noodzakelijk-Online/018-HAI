package braincatalog

import "strings"

const (
	readinessReviewNow     = "review_now"
	readinessLicenseReview = "license_review"
	readinessReferenceOnly = "reference_only"
	readinessNotAdopted    = "not_adopted"
	readinessArchived      = "archived"
	readinessUnavailable   = "upstream_unavailable"
	readinessProfileReview = "profile_review"
)

// applyReadinessAssessment turns source metadata into a transparent review
// priority. It never changes Entry.Status or grants execution authority.
func applyReadinessAssessment(entry Entry, review *UpstreamReview) {
	if review == nil {
		return
	}
	review.RequiredGates = requiredAdapterGates(entry)
	switch {
	case entry.Status == StatusExcluded:
		review.Readiness = readinessNotAdopted
		review.ReadinessReason = "HAI has already excluded this project. Metadata does not reopen the adoption decision."
	case entry.Status == StatusReferenceOnly:
		review.Readiness = readinessReferenceOnly
		review.ReadinessReason = "This project is retained as a reference pattern, not an active HAI control-plane candidate."
	case entry.Status == StatusLicenseReview:
		review.Readiness = readinessLicenseReview
		review.ReadinessReason = "The catalog requires an explicit licence and architecture decision before any adapter review."
	case !review.Available:
		review.Readiness = readinessUnavailable
		review.ReadinessReason = "The upstream could not be confirmed. HAI cannot prioritize an adapter review from unavailable metadata."
	case review.Archived:
		review.Readiness = readinessArchived
		review.ReadinessReason = "GitHub reports this upstream as archived. Keep it out of active integration work unless a human records an exception."
	case needsLicenseReview(review.License):
		review.Readiness = readinessLicenseReview
		review.ReadinessReason = "The reported licence needs explicit review before HAI can prioritize an adapter design."
	case entry.Status == StatusIntegrated:
		review.Readiness = readinessProfileReview
		review.ReadinessReason = "HAI has a profile boundary, but local configuration and a live health check still determine operational readiness."
	default:
		review.Readiness = readinessReviewNow
		review.ReadinessReason = "Public metadata is available and does not trigger the current archive or licence hold rules. This is a review priority, not an approval to install or run code."
	}
}

func needsLicenseReview(license string) bool {
	license = strings.ToUpper(strings.TrimSpace(license))
	if license == "" || license == "NOT REPORTED" || license == "NOASSERTION" {
		return true
	}
	return strings.Contains(license, "AGPL") || strings.Contains(license, "GPL") || strings.Contains(license, "SSPL") || strings.Contains(license, "BSL") || strings.Contains(license, "SUSTAINABLE")
}

func requiredAdapterGates(entry Entry) []string {
	gates := []string{
		"Owner approval of a narrow adapter design",
		"Local deployment, credential, data-egress, retention, and source-scope review",
		"Health check, audit event, rollback path, and no-op validation",
	}
	if entry.RequiresApproval {
		gates = append(gates, "Existing HAI approval policy before any consequential action")
	}
	return gates
}
