package runtimelab

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the Runtime Lab API (§10.19).
type Handler struct {
	svc *Service
}

// NewHandler builds a handler over a service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Overview(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"runtimes": h.svc.Overview(c.Request.Context())})
}

func (h *Handler) Probe(c *gin.Context) {
	res, ok := h.svc.Probe(c.Request.Context(), c.Param("runtimeId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "runtime not found"})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) SelfTest(c *gin.Context) {
	attempt, ok := h.svc.SelfTest(c.Request.Context(), c.Param("runtimeId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "runtime not found"})
		return
	}
	c.JSON(http.StatusOK, attempt)
}

func (h *Handler) Attempts(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"attempts": h.svc.Attempts(c.Param("runtimeId"))})
}
