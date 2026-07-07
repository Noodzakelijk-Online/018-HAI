// Package entitlements defines which product features are available, encoding
// the "no forced billing" stance: every core capability is usable without
// payment. Pure and dependency-free.
package entitlements

import "sort"

// Feature identifies a product capability.
type Feature string

const (
	Memory     Feature = "memory"
	Search     Feature = "search"
	Workflows  Feature = "workflows"
	Approvals  Feature = "approvals"
	Export     Feature = "export"
	Automation Feature = "automation"
	Verification Feature = "verification"
)

// coreFeatures are always available without payment.
var coreFeatures = map[Feature]bool{
	Memory: true, Search: true, Workflows: true, Approvals: true,
	Export: true, Automation: true, Verification: true,
}

// Available reports whether a feature is usable. Core features are always
// available — the product never gates core functionality behind billing.
func Available(f Feature) bool {
	return coreFeatures[f]
}

// RequiresPayment reports whether a feature requires payment. No core feature
// does, so this is always false for known features (kept as an explicit,
// auditable statement of the no-forced-billing policy).
func RequiresPayment(f Feature) bool {
	return false
}

// FreeTier returns all features available for free, sorted.
func FreeTier() []Feature {
	out := make([]Feature, 0, len(coreFeatures))
	for f := range coreFeatures {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
