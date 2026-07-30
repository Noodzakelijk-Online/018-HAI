package evidently

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Evidently runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Evidently runner could not be reached"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Evaluate(c *gin.Context) {
	var request Request
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Evidently evaluation request"})
		return
	}
	result, err := h.service.Evaluate(c.Request.Context(), request)
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Evidently runner is not configured"})
		return
	}
	if errors.Is(err, ErrInvalidRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Evidently requires 1 to 25 synthetic or redacted cases with opaque IDs and bounded text"})
		return
	}
	if errors.Is(err, ErrUnsafeFixture) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Evidently fixtures must not include detected personal data or secrets; redact or replace them first"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Evidently runner could not return report metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
