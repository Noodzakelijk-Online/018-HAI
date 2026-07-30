package guardrails

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Guardrails AI runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Guardrails AI runner could not be reached"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Validate(c *gin.Context) {
	var request Request
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Guardrails AI validation request"})
		return
	}
	result, err := h.service.Validate(c.Request.Context(), request)
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Guardrails AI runner is not configured"})
		return
	}
	if errors.Is(err, ErrInvalidRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Guardrails AI accepts only one bounded action_proposal JSON document"})
		return
	}
	if errors.Is(err, ErrUnsafeProposal) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Guardrails AI proposals must not include detected personal data or secrets; redact or replace them first"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Guardrails AI runner could not return validation metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
