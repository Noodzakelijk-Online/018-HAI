package agentcycle

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"automation-hub-backend/internal/identity"

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
	ownerIdentity := verifiedOwner(c)
	if ownerIdentity == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required to refresh an operating brief"})
		return
	}
	var request RunRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must be valid JSON"})
		return
	}
	request.OwnerIdentity = ownerIdentity
	result := h.service.Run(request)
	status := http.StatusOK
	if result.Status == "failed" {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, result)
}

func verifiedOwner(c *gin.Context) string {
	value, ok := c.Get(identity.ContextSubjectKey)
	if !ok {
		return ""
	}
	owner, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(owner)
}
