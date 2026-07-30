package syft

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
	if errors.Is(err, ErrNotConfigured) { c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()}); return }
	if err != nil { c.JSON(http.StatusBadGateway, gin.H{"error": "local Syft runner could not be verified"}); return }
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Inventory(c *gin.Context) {
	var request struct {
		WorkspaceID string `json:"workspaceId"`
		WorkflowID  string `json:"workflowId,omitempty"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" { c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceId is required; HAI never accepts a filesystem path, source content, SBOM export, command, scanner configuration, or report destination"}); return }
	var result *InventoryResult
	var err error
	if workflowInventory, ok := h.service.(WorkflowInventoryService); ok {
		ownerIdentity, _ := c.Get(identity.ContextSubjectKey)
		owner, _ := ownerIdentity.(string)
		result, err = workflowInventory.InventoryWithWorkflow(c.Request.Context(), strings.TrimSpace(owner), request.WorkspaceID, request.WorkflowID)
	} else {
		result, err = h.service.Inventory(c.Request.Context(), request.WorkspaceID)
	}
	switch {
	case errors.Is(err, ErrNotConfigured): c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "status": h.service.Status()})
	case errors.Is(err, ErrWorkspace): c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil: c.JSON(http.StatusBadGateway, gin.H{"error": "local Syft runner could not return a redacted aggregate inventory result"})
	default: c.JSON(http.StatusOK, result)
	}
}
