package agentruntime

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	registry *Registry
}

func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) Registry(c *gin.Context) {
	c.JSON(http.StatusOK, h.registry.List())
}

func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.registry.Health(ctx))
}
