package frameworkregistry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestFamilyTaxonomyHandlerRequiresOwnerAndReturnsImmutableVersionedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if owner := c.GetHeader("X-Test-Owner"); owner != "" {
			c.Set(identity.ContextSubjectKey, owner)
		}
		c.Next()
	})
	router.GET("/family-taxonomy", handler.FamilyTaxonomy)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(
		unauthenticated,
		httptest.NewRequest(http.MethodGet, "/family-taxonomy", nil),
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated status = %d, want %d: %s",
			unauthenticated.Code,
			http.StatusUnauthorized,
			unauthenticated.Body.String(),
		)
	}

	firstResponse := requestFamilyTaxonomy(t, router, "")
	var first FrameworkFamilyTaxonomy
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode taxonomy: %v", err)
	}
	if first.Version != FrameworkFamilyTaxonomyVersion ||
		first.Digest == "" ||
		len(first.Families) != 55 {
		t.Fatalf("taxonomy metadata = %#v", first)
	}
	if err := ValidateFamilyTaxonomy(first); err != nil {
		t.Fatalf("ValidateFamilyTaxonomy: %v", err)
	}
	if got := firstResponse.Header().Get("ETag"); got != `"`+first.Digest+`"` {
		t.Fatalf("ETag = %q, want digest ETag", got)
	}
	if got := firstResponse.Header().Get("Cache-Control"); got != "private, max-age=86400, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}

	first.Families[0].Purpose = "caller mutation"
	secondResponse := requestFamilyTaxonomy(t, router, "")
	var second FrameworkFamilyTaxonomy
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second taxonomy: %v", err)
	}
	if second.Families[0].Purpose == "caller mutation" ||
		second.Digest != first.Digest {
		t.Fatalf("taxonomy leaked caller mutation: %#v", second.Families[0])
	}

	notModified := requestFamilyTaxonomy(t, router, `"`+first.Digest+`"`)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf(
			"conditional response = %d %q",
			notModified.Code,
			notModified.Body.String(),
		)
	}
}

func TestFrameworkHandlerRejectsMalformedAndDuplicateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.GET("/selections", handler.Selections)
	router.GET("/constitution/history", handler.ConstitutionHistory)

	for _, test := range []struct {
		path string
	}{
		{path: "/selections?limit=invalid"},
		{path: "/selections?limit=0"},
		{path: "/selections?limit=201"},
		{path: "/selections?limit=1&limit=2"},
		{path: "/constitution/history?limit=invalid"},
		{path: "/constitution/history?limit=101"},
		{path: "/constitution/history?limit=1&limit=2"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, test.path, nil),
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf(
				"%s status = %d, want %d: %s",
				test.path,
				response.Code,
				http.StatusBadRequest,
				response.Body.String(),
			)
		}
	}
}

func requestFamilyTaxonomy(
	t *testing.T,
	router http.Handler,
	ifNoneMatch string,
) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/family-taxonomy", nil)
	request.Header.Set("X-Test-Owner", "alice")
	if ifNoneMatch != "" {
		request.Header.Set("If-None-Match", ifNoneMatch)
	}
	router.ServeHTTP(response, request)
	if ifNoneMatch == "" && response.Code != http.StatusOK {
		t.Fatalf(
			"taxonomy status = %d, want %d: %s",
			response.Code,
			http.StatusOK,
			response.Body.String(),
		)
	}
	return response
}
