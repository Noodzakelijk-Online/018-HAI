# Performance Baseline & Indexing

A baseline so regressions are visible. Numbers are from the in-memory query
benchmarks; they measure the filtering/sorting/pagination logic, not the DB.

## Query benchmark (10,000 memories, Apple silicon, Go 1.25)

| Benchmark | Operation | Time/op | Allocs/op |
| --- | --- | --- | --- |
| `BenchmarkQueryFilterSortPaginate` | kind filter + sort + paginate | ~0.88 ms | 6 |
| `BenchmarkQuerySearch` | free-text relevance search + sort | ~5.97 ms | ~77.5k |

Reproduce:

```bash
go test ./internal/memory -bench BenchmarkQuery -benchmem -run '^$'
```

**Reading it:** filter/sort/paginate is allocation-light and sub-millisecond at
10k rows. Free-text search is ~7× costlier and allocation-heavy because it
tokenizes every candidate; if search latency ever matters at scale, that is the
place to add a precomputed token index or push search into Postgres.

## Database indexing

`context_memories` already carries indexes aligned with its hot query paths
(see `internal/models/context_memory.go`):

| Column | Purpose |
| --- | --- |
| `project_key` | project-scoped listing/isolation |
| `kind` | kind filters |
| `content_hash` | dedup lookups on write |
| `archived` | excluding archived rows in the default list |

When list/search moves from in-memory filtering to SQL (recommended past tens of
thousands of rows per project), add a composite index on
`(project_key, archived, updated_at)` to serve the default sorted listing, and
consider a trigram/full-text index for `content` to back search.

## Guardrails already in place

- Pagination is bounded (`pageSize` max 100) so a single request can never load
  an unbounded result set.
- Large-dataset correctness is covered by `largedataset_test.go` (50k rows).
