package assistant

import (
	"automation-hub-backend/internal/identity"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Command(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "assistant service is not configured"})
		return
	}
	var request CommandRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	if strings.TrimSpace(request.Message) == "" && !request.RunCycle {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}
	result, err := h.service.Command(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "result": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

func verifiedActor(c *gin.Context, fallback string) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok && strings.TrimSpace(subject) != "" {
			return subject
		}
	}
	return fallback
}

func (h *Handler) Logs(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "assistant service is not configured"})
		return
	}
	c.JSON(http.StatusOK, h.service.Logs())
}
