package mcppreflight

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes the review-only MCP preflight API.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Overview(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Overview())
}

func (h *Handler) Run(c *gin.Context) {
	result, found := h.svc.Preflight(c.Request.Context(), c.Param("serverId"))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "configured MCP preflight server not found"})
		return
	}
	status := http.StatusOK
	if result.Status == "disabled" || result.Status == "blocked" {
		status = http.StatusConflict
	}
	c.JSON(status, result)
}
