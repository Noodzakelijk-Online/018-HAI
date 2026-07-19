package braincatalog

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes the transparent catalog. It deliberately has no enable or
// install endpoint: activation belongs to a reviewed runtime adapter.
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sourceCatalog":    sourceCatalogURL,
		"verifiedAt":       verifiedAt,
		"entries":          Entries(),
		"activationPolicy": "Catalog discovery is read-only. HAI never installs, enables, or executes a listed project without a reviewed adapter and the existing approval gates.",
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
