package anythingllm

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local AnythingLLM evidence retrieval is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local AnythingLLM endpoint could not be verified"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Retrieve(c *gin.Context) {
	var request Request
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid AnythingLLM evidence retrieval request"})
		return
	}
	result, err := h.service.Retrieve(c.Request.Context(), request)
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local AnythingLLM evidence retrieval is not configured"})
		return
	}
	if errors.Is(err, ErrInvalidRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AnythingLLM query must be single-line, at most 512 characters, and use an approved workspace"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local AnythingLLM could not return candidate evidence"})
		return
	}
	c.JSON(http.StatusOK, result)
}
