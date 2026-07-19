package braincatalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeUpstreamReviewer struct {
	review UpstreamReview
	err    error
}

type fakeCollectionReviewer struct {
	review OSSInsightCollectionReview
	err    error
}

func (f fakeCollectionReviewer) ReviewCollections() (OSSInsightCollectionReview, error) {
	return f.review, f.err
}

type fakeRepositoryScout struct {
	report OSSInsightRepositoryDiscoveryReport
	err    error
}

func (f fakeRepositoryScout) DiscoverRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	return f.report, f.err
}

func (f fakeUpstreamReviewer) Review(entry Entry) (UpstreamReview, error) {
	if f.review.ID == "" {
		f.review.ID = entry.ID
	}
	return f.review, f.err
}

func TestListHandlerPublishesReadOnlyCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler()
	router.GET("/", handler.List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Catalog discovery is read-only") || !strings.Contains(response.Body.String(), "openhands") || !strings.Contains(response.Body.String(), "OSS Insight") || !strings.Contains(response.Body.String(), "litellm") || !strings.Contains(response.Body.String(), "langfuse") || !strings.Contains(response.Body.String(), "collectionScreening") || !strings.Contains(response.Body.String(), `"total":138`) || !strings.Contains(response.Body.String(), "license_review") {
		t.Fatalf("catalog response lacks policy or entry: %s", response.Body.String())
	}
}

func TestGetHandlerReturnsNotFoundForUnknownEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler()
	router.GET("/:id", handler.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRevalidateHandlerReturnsBoundedReview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandlerWithReviewer(fakeUpstreamReviewer{review: UpstreamReview{Available: true, License: "MIT", Message: "metadata only"}})
	router.POST("/:id/revalidate", handler.Revalidate)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/opencode/revalidate", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"opencode"`) || !strings.Contains(response.Body.String(), `"license":"MIT"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestRevalidateCollectionsHandlerReportsDriftWithoutMutatingCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandlerWithReviewers(
		fakeUpstreamReviewer{},
		fakeCollectionReviewer{review: OSSInsightCollectionReview{Available: true, ExpectedTotal: 138, CurrentTotal: 139, NewCollections: []string{"Future capability"}, Message: "drift only"}},
	)
	router.POST("/ossinsight/revalidate", handler.RevalidateCollections)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ossinsight/revalidate", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"newCollections":["Future capability"]`) || !strings.Contains(response.Body.String(), "drift only") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestDiscoverRepositoriesHandlerReportsUnreviewedCandidates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandlerWithReviewersAndScout(
		fakeUpstreamReviewer{},
		fakeCollectionReviewer{},
		fakeRepositoryScout{report: OSSInsightRepositoryDiscoveryReport{Available: true, CollectionsScreened: 138, Discoveries: []OSSInsightRepositoryDiscovery{{Repository: "owner/new-agent"}}, Message: "did not add catalog entries"}},
	)
	router.POST("/ossinsight/discover", handler.DiscoverRepositories)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ossinsight/discover", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "owner/new-agent") || !strings.Contains(response.Body.String(), "did not add catalog entries") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
