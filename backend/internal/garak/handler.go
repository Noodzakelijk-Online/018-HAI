package garak

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Status(c *gin.Context)  { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Garak safety runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Garak safety runner could not be reached"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Run(c *gin.Context) {
	// This suite is fixed. Reject even chunked bodies so callers cannot smuggle
	// targets, models, prompts, probes, or arbitrary scanner configuration.
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1))
	if err != nil || len(body) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local Garak evaluation runs the configured fixed synthetic suite and accepts no caller-provided target, model, endpoint, prompt, probe, command, or data"})
		return
	}
	result, err := h.service.Run(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Garak safety runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Garak safety runner could not return scan metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
