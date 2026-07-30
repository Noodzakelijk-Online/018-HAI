package braincatalog

import (
	"strings"
	"testing"
)

func TestCollectionScreeningsCoverTheUniqueOSSInsightSnapshot(t *testing.T) {
	screenings := CollectionScreenings()
	if len(screenings) != 138 {
		t.Fatalf("screening count = %d, want complete 138-collection snapshot", len(screenings))
	}

	seen := make(map[string]CollectionScreening, len(screenings))
	for _, screening := range screenings {
		name := strings.TrimSpace(screening.Collection)
		if name == "" || screening.Page < 1 || strings.TrimSpace(screening.Rationale) == "" || strings.TrimSpace(screening.SourceURL) == "" {
			t.Fatalf("invalid collection screening: %#v", screening)
		}
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("collection %q appears more than once in the screening snapshot", name)
		}
		seen[name] = screening
	}

	for name, id := range ossInsightCollectionIDs {
		screening, found := seen[name]
		if !found {
			t.Fatalf("collection API ID %q is not represented by a screening", name)
		}
		if !strings.Contains(screening.SourceURL, "/collections/"+id+"/repos/") {
			t.Fatalf("collection %q source URL = %q, want API repository provenance for ID %q", name, screening.SourceURL, id)
		}
	}
}
