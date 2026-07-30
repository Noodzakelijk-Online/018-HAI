package langfuse

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Langfuse observability bridge is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Langfuse health or readiness check failed"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) ExportOperationalSnapshot(c *gin.Context) {
	// The bridge exports one fixed aggregate schema. It deliberately accepts no
	// caller-selected trace content, source data, models, or provider settings.
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1))
	if err != nil || len(body) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Langfuse export accepts no caller-provided trace content, source data, model data, or settings"})
		return
	}
	result, err := h.service.ExportOperationalSnapshot(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Langfuse observability bridge is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Langfuse did not accept the aggregate observability trace"})
		return
	}
	c.JSON(http.StatusOK, result)
}
