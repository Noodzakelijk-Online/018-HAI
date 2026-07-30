package autogencompat

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxPreviewBody int64 = 128 << 10

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

// Preview is intentionally transient. It normalizes a bounded migration sample
// but does not write it to an HAI source, memory, workflow, task, or audit log.
func (h *Handler) Preview(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPreviewBody)
	var request PreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a bounded AutoGen compatibility preview request is required"})
		return
	}
	preview, err := h.service.Preview(request)
	if errors.Is(err, ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workloadId and 1-100 supported, bounded events are required"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not prepare AutoGen compatibility preview"})
		return
	}
	c.JSON(http.StatusOK, preview)
}

// MigrationPlan prepares a fixed-framework migration plan without starting or
// configuring a Microsoft Agent Framework process.
func (h *Handler) MigrationPlan(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPreviewBody)
	var request MigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a bounded Microsoft Agent Framework migration request is required"})
		return
	}
	plan, err := h.service.MigrationPlan(request)
	if errors.Is(err, ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target microsoft-agent-framework plus workloadId and 1-100 supported, bounded events are required"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not prepare Microsoft Agent Framework migration plan"})
		return
	}
	c.JSON(http.StatusOK, plan)
}
