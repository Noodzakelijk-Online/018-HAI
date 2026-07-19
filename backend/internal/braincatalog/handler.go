package braincatalog

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type discoveryReviewRequest struct {
	Repository string `json:"repository"`
}

type capabilityRecommendationRequest struct {
	Need string `json:"need"`
}

// Handler exposes the transparent catalog. It deliberately has no enable or
// install endpoint: activation belongs to a reviewed runtime adapter.
type Handler struct {
	reviewer           UpstreamReviewer
	collectionReviewer OSSInsightCollectionReviewer
	repositoryScout    OSSInsightRepositoryScout
}

func NewHandler() *Handler {
	return NewHandlerWithReviewersAndScout(NewUpstreamReviewer(nil), NewOSSInsightCollectionReviewer(nil), NewOSSInsightRepositoryScout(nil))
}

func NewHandlerWithReviewer(reviewer UpstreamReviewer) *Handler {
	return NewHandlerWithReviewersAndScout(reviewer, NewOSSInsightCollectionReviewer(nil), NewOSSInsightRepositoryScout(nil))
}

func NewHandlerWithReviewers(reviewer UpstreamReviewer, collectionReviewer OSSInsightCollectionReviewer) *Handler {
	return NewHandlerWithReviewersAndScout(reviewer, collectionReviewer, NewOSSInsightRepositoryScout(nil))
}

func NewHandlerWithReviewersAndScout(reviewer UpstreamReviewer, collectionReviewer OSSInsightCollectionReviewer, repositoryScout OSSInsightRepositoryScout) *Handler {
	return &Handler{reviewer: reviewer, collectionReviewer: collectionReviewer, repositoryScout: repositoryScout}
}

func (h *Handler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sourceCatalog":       sourceCatalogURL,
		"discoverySources":    DiscoverySources(),
		"verifiedAt":          verifiedAt,
		"entries":             Entries(),
		"planeCoverage":       CapabilityPlaneCoverageReport(),
		"collectionScreening": OSSInsightScreening(),
		"activationPolicy":    "Catalog discovery is read-only. HAI never installs, enables, or executes a listed project without a reviewed adapter and the existing approval gates.",
	})
}

func (h *Handler) Get(c *gin.Context) {
	entry, ok := EntryByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "catalog entry not found"})
		return
	}
	c.JSON(http.StatusOK, entry)
}

// Revalidate performs a bounded public metadata check for a fixed catalog
// entry. It does not mutate catalog status, install software, or enable a
// runtime; the admin route protects the external request from bulk use.
func (h *Handler) Revalidate(c *gin.Context) {
	entry, ok := EntryByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "catalog entry not found"})
		return
	}
	if h.reviewer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "upstream revalidation is unavailable"})
		return
	}
	review, err := h.reviewer.Review(entry)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not revalidate the configured upstream"})
		return
	}
	c.JSON(http.StatusOK, review)
}

// RevalidateCollections compares HAI's fixed 138-category source snapshot to
// the public OSS Insight list. It is an admin-only, read-only drift check; it
// cannot add entries, change their status, or activate third-party code.
func (h *Handler) RevalidateCollections(c *gin.Context) {
	if h.collectionReviewer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OSS Insight collection revalidation is unavailable"})
		return
	}
	review, err := h.collectionReviewer.ReviewCollections()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not revalidate the OSS Insight collection list"})
		return
	}
	c.JSON(http.StatusOK, review)
}

// DiscoverRepositories reads repository names only from pre-screened candidate
// collections. It cannot add a catalog entry, create credentials, or execute
// a discovered project; the owner must initiate a separate review.
func (h *Handler) DiscoverRepositories(c *gin.Context) {
	if h.repositoryScout == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OSS Insight repository discovery is unavailable"})
		return
	}
	report, err := h.repositoryScout.DiscoverRepositories()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not discover OSS Insight repositories"})
		return
	}
	c.JSON(http.StatusOK, report)
}

// RevalidateDiscovery verifies GitHub metadata for one repository that was
// returned by the source-controlled discovery report. It rejects arbitrary
// repository names, does not alter the catalog, and cannot activate code.
func (h *Handler) RevalidateDiscovery(c *gin.Context) {
	if h.repositoryScout == nil || h.reviewer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "discovery metadata revalidation is unavailable"})
		return
	}
	var request discoveryReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Repository) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repository is required"})
		return
	}
	report, err := h.repositoryScout.DiscoverRepositories()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not verify the OSS Insight discovery report"})
		return
	}
	discovery, ok := discoveryByRepository(report, request.Repository)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "repository is not in the current OSS Insight discovery report"})
		return
	}
	entry, err := entryForDiscovery(discovery)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "discovered repository is not a valid GitHub repository path"})
		return
	}
	review, err := h.reviewer.Review(entry)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not retrieve discovered repository metadata"})
		return
	}
	c.JSON(http.StatusOK, review)
}

// RecommendCapabilities maps a stated need to the reviewed catalog only. It
// is a planner aid and cannot revalidate, install, configure, or run anything.
func (h *Handler) RecommendCapabilities(c *gin.Context) {
	var request capabilityRecommendationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "capability need is required"})
		return
	}
	response, err := RecommendForNeed(request.Need)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}
