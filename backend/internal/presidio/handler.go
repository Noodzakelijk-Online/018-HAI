package presidio

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Analyze(c *gin.Context) {
	var request Request
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Presidio analysis request"})
		return
	}
	result, err := h.service.Analyze(c.Request.Context(), request)
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Presidio analysis is not configured"})
		return
	}
	if errors.Is(err, ErrInvalidRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Presidio text must be non-empty and at most 8192 characters"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Presidio analyzer could not return privacy metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
