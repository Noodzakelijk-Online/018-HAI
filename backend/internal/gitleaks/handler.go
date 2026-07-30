package gitleaks

import (
	"automation-hub-backend/internal/identity"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Status(c *gin.Context)  { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Gitleaks runner could not be verified"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Scan(c *gin.Context) {
	var request struct {
		WorkspaceID string `json:"workspaceId"`
		WorkflowID  string `json:"workflowId,omitempty"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceId is required; HAI never accepts a filesystem path, source content, secret, command, scanner configuration, or report destination"})
		return
	}
	var result *ScanResult
	var err error
	if workflowScanner, ok := h.service.(WorkflowScanService); ok {
		result, err = workflowScanner.ScanWithWorkflow(c.Request.Context(), owner(c), request.WorkspaceID, request.WorkflowID)
	} else {
		result, err = h.service.Scan(c.Request.Context(), request.WorkspaceID)
	}
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()})
	case errors.Is(err, ErrWorkspace):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Gitleaks runner could not return a redacted aggregate scan result"})
	default:
		c.JSON(http.StatusOK, result)
	}
}

func owner(c *gin.Context) string {
	value, _ := c.Get(identity.ContextSubjectKey)
	owner, _ := value.(string)
	return strings.TrimSpace(owner)
}
