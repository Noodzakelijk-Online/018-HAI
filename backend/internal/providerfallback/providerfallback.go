// Package providerfallback implements a pure, deterministic provider-selection
// policy. Given an ordered list of candidate providers, it picks the first
// available one, always preferring free/local providers over paid ones and
// never selecting a paid provider unless paid usage is explicitly allowed.
package providerfallback

// Provider is a candidate generation backend.
type Provider struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Paid      bool   `json:"paid"`
}

// Select returns the chosen provider following the fallback policy:
//   1. free/local available providers, in order;
//   2. only if allowPaid, paid available providers, in order.
// It returns ok=false when nothing is selectable. Selection is deterministic:
// the same input always yields the same result.
func Select(providers []Provider, allowPaid bool) (Provider, bool) {
	// First pass: free/local providers.
	for _, p := range providers {
		if p.Available && !p.Paid {
			return p, true
		}
	}
	// Second pass: paid providers, only when explicitly allowed.
	if allowPaid {
		for _, p := range providers {
			if p.Available && p.Paid {
				return p, true
			}
		}
	}
	return Provider{}, false
}
