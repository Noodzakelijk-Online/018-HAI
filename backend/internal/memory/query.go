package memory

import (
	"sort"
	"strings"

	"automation-hub-backend/internal/models"
)

// QueryParams describes search, filter, sort, and pagination options for
// listing context memories. It is consumed by the pure Query function so the
// same behaviour can be unit-tested without any database or HTTP layer.
type QueryParams struct {
	Search   string // free-text; matched (AND semantics) against content, summary, tags
	Kind     string // exact kind filter (case-insensitive)
	Tag      string // single tag filter (case-insensitive, exact tag match)
	Sort     string // updatedAt|createdAt|confidence|kind|relevance (default updatedAt)
	Order    string // asc|desc (default desc)
	Page     int    // 1-based page number (default 1)
	PageSize int    // items per page (default 20, max 100)
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// PageResult is the paginated envelope returned by memory queries. It echoes
// the effective (normalized) query parameters so a client can render controls
// truthfully and never guess what filtering/sorting actually happened.
type PageResult struct {
	Items      []models.ContextMemory `json:"items"`
	Total      int                    `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	TotalPages int                    `json:"totalPages"`
	Sort       string                 `json:"sort"`
	Order      string                 `json:"order"`
	Search     string                 `json:"search,omitempty"`
	Kind       string                 `json:"kind,omitempty"`
	Tag        string                 `json:"tag,omitempty"`
}

// normalized applies safe defaults and bounds so out-of-range or empty input
// can never produce a panic or an unbounded page.
func (p QueryParams) normalized() QueryParams {
	out := p
	out.Search = strings.TrimSpace(p.Search)
	out.Kind = strings.TrimSpace(p.Kind)
	out.Tag = strings.TrimSpace(p.Tag)

	switch strings.ToLower(strings.TrimSpace(p.Sort)) {
	case "createdat":
		out.Sort = "createdAt"
	case "confidence":
		out.Sort = "confidence"
	case "kind":
		out.Sort = "kind"
	case "relevance":
		out.Sort = "relevance"
	default:
		out.Sort = "updatedAt"
	}

	if strings.EqualFold(strings.TrimSpace(p.Order), "asc") {
		out.Order = "asc"
	} else {
		out.Order = "desc"
	}

	if out.Page < 1 {
		out.Page = 1
	}
	if out.PageSize <= 0 {
		out.PageSize = defaultPageSize
	}
	if out.PageSize > maxPageSize {
		out.PageSize = maxPageSize
	}
	return out
}

// Query filters, sorts, and paginates the supplied memories deterministically.
//
// It is a pure function: no I/O and no shared state, so it is safe to unit test
// in isolation and safe to call from any handler after loading rows. The input
// slice is never mutated.
func Query(items []models.ContextMemory, params QueryParams) PageResult {
	p := params.normalized()

	// Filter.
	searchTokens := searchTokensOf(p.Search)
	filtered := make([]models.ContextMemory, 0, len(items))
	for _, item := range items {
		if p.Kind != "" && !strings.EqualFold(item.Kind, p.Kind) {
			continue
		}
		if p.Tag != "" && !hasTag(item.Tags, p.Tag) {
			continue
		}
		if len(searchTokens) > 0 && !matchesAllTokens(item, searchTokens) {
			continue
		}
		filtered = append(filtered, item)
	}

	// Sort (stable, so equal keys keep input order).
	sortMemories(filtered, p, searchTokens)

	total := len(filtered)
	totalPages := 0
	if total > 0 {
		totalPages = (total + p.PageSize - 1) / p.PageSize
	}

	// Paginate with safe bounds; a page past the end yields an empty slice
	// rather than an error, and the requested page is echoed back honestly.
	start := (p.Page - 1) * p.PageSize
	pageItems := []models.ContextMemory{}
	if start < total {
		end := start + p.PageSize
		if end > total {
			end = total
		}
		pageItems = append(pageItems, filtered[start:end]...)
	}

	return PageResult{
		Items:      pageItems,
		Total:      total,
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalPages: totalPages,
		Sort:       p.Sort,
		Order:      p.Order,
		Search:     p.Search,
		Kind:       p.Kind,
		Tag:        p.Tag,
	}
}

func sortMemories(items []models.ContextMemory, p QueryParams, searchTokens []string) {
	asc := p.Order == "asc"
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		var less bool
		switch p.Sort {
		case "createdAt":
			less = left.CreatedAt.Before(right.CreatedAt)
		case "confidence":
			if left.Confidence == right.Confidence {
				less = left.UpdatedAt.Before(right.UpdatedAt)
			} else {
				less = left.Confidence < right.Confidence
			}
		case "kind":
			if strings.EqualFold(left.Kind, right.Kind) {
				less = left.UpdatedAt.Before(right.UpdatedAt)
			} else {
				less = strings.ToLower(left.Kind) < strings.ToLower(right.Kind)
			}
		case "relevance":
			ls, rs := relevanceScore(left, searchTokens), relevanceScore(right, searchTokens)
			if ls == rs {
				less = left.UpdatedAt.Before(right.UpdatedAt)
			} else {
				less = ls < rs
			}
		default: // updatedAt
			less = left.UpdatedAt.Before(right.UpdatedAt)
		}
		if asc {
			return less
		}
		return !less
	})
}

// relevanceScore counts how many distinct search tokens appear in the memory's
// searchable text. Higher means more relevant.
func relevanceScore(item models.ContextMemory, searchTokens []string) int {
	if len(searchTokens) == 0 {
		return 0
	}
	haystack := haystackOf(item)
	score := 0
	for _, token := range searchTokens {
		if strings.Contains(haystack, token) {
			score++
		}
	}
	return score
}

func matchesAllTokens(item models.ContextMemory, searchTokens []string) bool {
	haystack := haystackOf(item)
	for _, token := range searchTokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func haystackOf(item models.ContextMemory) string {
	return strings.ToLower(item.Content + " " + item.Summary + " " + item.Tags)
}

func searchTokensOf(search string) []string {
	fields := strings.Fields(strings.ToLower(search))
	tokens := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
	}
	return tokens
}

// hasTag reports whether the comma-joined tag string contains an exact,
// case-insensitive match for the requested tag.
func hasTag(joinedTags, tag string) bool {
	want := strings.TrimSpace(strings.ToLower(tag))
	if want == "" {
		return true
	}
	for _, candidate := range strings.Split(joinedTags, ",") {
		if strings.TrimSpace(strings.ToLower(candidate)) == want {
			return true
		}
	}
	return false
}
