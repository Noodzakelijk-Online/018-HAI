package deepeval

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local DeepEval runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local DeepEval runner could not be reached"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Run(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1))
	if err != nil || len(body) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local DeepEval evaluation runs the configured fixed synthetic suite and accepts no caller-provided answer, source, model, endpoint, prompt, metric, command, or data"})
		return
	}
	result, err := h.service.Run(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local DeepEval runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local DeepEval runner could not return evaluation metadata"})
		return
	}
	c.JSON(http.StatusOK, result)
}
