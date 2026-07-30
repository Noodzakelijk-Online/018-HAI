package research

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if err == ErrNotConfigured {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local research is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local research endpoint could not be reached"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Search(c *gin.Context) {
	var request Request
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid research request"})
		return
	}
	result, err := h.service.Search(c.Request.Context(), request)
	if err == ErrNotConfigured {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local research is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local research could not return source candidates"})
		return
	}
	c.JSON(http.StatusOK, result)
}
