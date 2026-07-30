package serena

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Serena semantic context is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Serena endpoint could not pass its read-only tool check"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) FindSymbols(c *gin.Context) {
	var request SymbolRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Serena symbol request"})
		return
	}
	result, err := h.service.FindSymbols(c.Request.Context(), request)
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Serena semantic context is not configured"})
		return
	}
	if errors.Is(err, ErrInvalidRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Serena pattern and optional relative path must be bounded, single-line, and project-relative"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Serena symbol lookup could not return bounded metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
