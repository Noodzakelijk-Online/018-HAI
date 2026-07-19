package braincatalog

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes the transparent catalog. It deliberately has no enable or
// install endpoint: activation belongs to a reviewed runtime adapter.
type Handler struct{ reviewer UpstreamReviewer }

func NewHandler() *Handler { return NewHandlerWithReviewer(NewUpstreamReviewer(nil)) }

func NewHandlerWithReviewer(reviewer UpstreamReviewer) *Handler { return &Handler{reviewer: reviewer} }

func (h *Handler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sourceCatalog":       sourceCatalogURL,
		"discoverySources":    DiscoverySources(),
		"verifiedAt":          verifiedAt,
		"entries":             Entries(),
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
