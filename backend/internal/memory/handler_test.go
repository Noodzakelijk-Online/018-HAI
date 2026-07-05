package memory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// buildQueryHandler wires the real handler over the in-memory fake repository
// and seeds a few memories, so the HTTP layer is exercised end-to-end.
func buildQueryHandler(t *testing.T) *Handler {
	t.Helper()
	repo := newFakeRepository()
	service := NewService(repo)
	seed := []CreateRequest{
		{ProjectKey: "018-hai", Kind: "preference", Content: "Prefer local Ollama models before cloud models.", Tags: []string{"llm", "routing"}, Confidence: 0.9},
		{ProjectKey: "018-hai", Kind: "project", Content: "The dashboard is built with Angular and the backend in Go.", Tags: []string{"frontend"}, Confidence: 0.6},
		{ProjectKey: "018-hai", Kind: "preference", Content: "Always require approval before running automations.", Tags: []string{"safety"}, Confidence: 0.75},
	}
	for _, req := range seed {
		if _, err := service.Create(req); err != nil {
			t.Fatalf("seed memory: %v", err)
		}
	}
	return NewHandler(service)
}

func doQuery(t *testing.T, h *Handler, rawQuery string) PageResult {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/memory/query?"+rawQuery, nil)
	h.Query(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("Query status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var result PageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode PageResult: %v (body: %s)", err, rec.Body.String())
	}
	return result
}

func TestHandlerQueryPaginatesAndFilters(t *testing.T) {
	h := buildQueryHandler(t)

	// Search narrows to the single Angular/Go memory.
	search := doQuery(t, h, "q=angular+backend")
	if search.Total != 1 || search.Items[0].Kind != "project" {
		t.Fatalf("search total=%d, want 1 project memory", search.Total)
	}

	// Kind filter + pagination: two preferences, one per page.
	page1 := doQuery(t, h, "kind=preference&pageSize=1&page=1")
	if page1.Total != 2 || page1.TotalPages != 2 || len(page1.Items) != 1 {
		t.Fatalf("kind page1 total=%d totalPages=%d items=%d, want 2/2/1", page1.Total, page1.TotalPages, len(page1.Items))
	}
	page2 := doQuery(t, h, "kind=preference&pageSize=1&page=2")
	if len(page2.Items) != 1 || page2.Items[0].ID == page1.Items[0].ID {
		t.Fatalf("kind page2 should return the other preference, got %d items", len(page2.Items))
	}

	// Echoed normalized params let a client render controls truthfully.
	if page1.PageSize != 1 || page1.Sort != "updatedAt" || page1.Order != "desc" {
		t.Fatalf("echoed params wrong: size=%d sort=%s order=%s", page1.PageSize, page1.Sort, page1.Order)
	}
}
