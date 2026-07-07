package memory

import (
	"fmt"
	"testing"
	"time"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

func largeMemorySet(n int) []models.ContextMemory {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	kinds := []string{"preference", "project", "decision", "contact"}
	items := make([]models.ContextMemory, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, models.ContextMemory{
			ID:        uuid.New(),
			Kind:      kinds[i%len(kinds)],
			Content:   fmt.Sprintf("memory number %d about topic %d", i, i%50),
			Tags:      fmt.Sprintf("tag%d", i%len(kinds)),
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return items
}

func TestQueryPaginatesLargeDatasetCorrectly(t *testing.T) {
	const total = 50000
	items := largeMemorySet(total)

	first := Query(items, QueryParams{Sort: "updatedAt", Order: "asc", Page: 1, PageSize: 50})
	if first.Total != total || first.TotalPages != total/50 || len(first.Items) != 50 {
		t.Fatalf("page 1: total=%d totalPages=%d items=%d", first.Total, first.TotalPages, len(first.Items))
	}

	last := Query(items, QueryParams{Sort: "updatedAt", Order: "asc", Page: total / 50, PageSize: 50})
	if len(last.Items) != 50 {
		t.Fatalf("last page items=%d, want 50", len(last.Items))
	}
	// Page boundaries must not overlap: last item of page 1 precedes first of page 2.
	page2 := Query(items, QueryParams{Sort: "updatedAt", Order: "asc", Page: 2, PageSize: 50})
	if !first.Items[49].UpdatedAt.Before(page2.Items[0].UpdatedAt) {
		t.Fatalf("page boundary overlap between page 1 and 2")
	}
}

func TestQueryFilterOnLargeDatasetReducesTotal(t *testing.T) {
	items := largeMemorySet(50000)
	byKind := Query(items, QueryParams{Kind: "preference", PageSize: 10})
	// 4 kinds evenly distributed → ~1/4 of the set.
	if byKind.Total != 12500 {
		t.Fatalf("kind filter total = %d, want 12500", byKind.Total)
	}
	if len(byKind.Items) != 10 {
		t.Fatalf("page size not honored on filtered set: %d", len(byKind.Items))
	}
}
