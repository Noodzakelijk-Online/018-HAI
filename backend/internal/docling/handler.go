package docling

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Status(c *gin.Context)  { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Docling document extractor is not configured"})
	case err != nil:
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Docling document extractor could not be reached"})
	default:
		c.JSON(http.StatusOK, result)
	}
}
