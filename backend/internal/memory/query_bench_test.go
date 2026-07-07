package memory

import "testing"

// BenchmarkQuerySearch establishes a performance baseline for filtered,
// sorted, paginated queries over a large in-memory set. Run with:
//
//	go test ./internal/memory -bench BenchmarkQuery -benchmem
func BenchmarkQuerySearch(b *testing.B) {
	items := largeMemorySet(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Query(items, QueryParams{Search: "topic 7", Sort: "relevance", Page: 1, PageSize: 20})
	}
}

func BenchmarkQueryFilterSortPaginate(b *testing.B) {
	items := largeMemorySet(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Query(items, QueryParams{Kind: "preference", Sort: "updatedAt", Order: "desc", Page: 3, PageSize: 25})
	}
}
