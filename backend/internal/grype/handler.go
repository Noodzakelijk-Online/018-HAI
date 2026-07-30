package grype

import (
	"automation-hub-backend/internal/identity"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()})
	case err != nil:
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Grype runner could not be verified"})
	default:
		c.JSON(http.StatusOK, result)
	}
}

func (h *Handler) Scan(c *gin.Context) {
	var request struct { WorkspaceID string `json:"workspaceId"` }
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceId is required; HAI never accepts a filesystem path, source content, package, CVE, advisory, report, command, scanner configuration, or remediation request"})
		return
	}
	// Read the owner context before execution so future audit linkage cannot
	// accidentally treat a request without an authenticated subject as valid.
	subject, found := c.Get(identity.ContextSubjectKey)
	owner, isString := subject.(string)
	if !found || !isString || strings.TrimSpace(owner) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated owner identity is required"})
		return
	}
	result, err := h.service.Scan(c.Request.Context(), request.WorkspaceID)
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()})
	case errors.Is(err, ErrWorkspace):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Grype runner could not return a redacted aggregate vulnerability result"})
	default:
		c.JSON(http.StatusOK, result)
	}
}
