package memory

import (
	"math"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

// These tests throw hostile / degenerate inputs at the query surface and assert
// it degrades safely: no panic, bounded output, non-nil slices.

func TestQueryHandlesExtremePaginationParams(t *testing.T) {
	items := sampleMemories()

	huge := Query(items, QueryParams{PageSize: math.MaxInt32, Page: math.MaxInt32})
	if huge.PageSize != maxPageSize {
		t.Fatalf("MaxInt pageSize not clamped: %d", huge.PageSize)
	}
	if len(huge.Items) != 0 {
		t.Fatalf("page past the end should be empty, got %d", len(huge.Items))
	}

	neg := Query(items, QueryParams{PageSize: -100, Page: -100})
	if neg.Page != 1 || neg.PageSize != defaultPageSize {
		t.Fatalf("negative params not normalized: page=%d size=%d", neg.Page, neg.PageSize)
	}
}

func TestQueryHandlesGiantAndWeirdSearchInput(t *testing.T) {
	items := sampleMemories()

	// A 200k-character search string with tens of thousands of tokens.
	giant := strings.Repeat("x ", 100000)
	res := Query(items, QueryParams{Search: giant})
	if res.Items == nil {
		t.Fatalf("Items must be non-nil even for pathological search")
	}

	// Control characters, unicode, and punctuation must not panic.
	for _, s := range []string{"\x00\x01\x02", "日本語 test", "'; DROP TABLE--", strings.Repeat("😀", 5000)} {
		_ = Query(items, QueryParams{Search: s, Tag: s, Kind: s})
	}
}

func TestQueryHandlesMemoriesWithEmptyAndHugeFields(t *testing.T) {
	items := []models.ContextMemory{
		{ID: uuid.New()}, // all-empty memory
		{ID: uuid.New(), Content: strings.Repeat("a", 500000), Tags: strings.Repeat("t,", 10000)},
	}
	res := Query(items, QueryParams{Search: "a", Sort: "relevance"})
	if res.Total < 0 || res.Items == nil {
		t.Fatalf("degenerate memories produced invalid result: %+v", res)
	}
}
