package agentcycle

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Run(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent cycle service is not configured"})
		return
	}
	var request RunRequest
	_ = c.ShouldBindJSON(&request)
	result := h.service.Run(request)
	status := http.StatusOK
	if result.Status == "failed" {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, result)
}
