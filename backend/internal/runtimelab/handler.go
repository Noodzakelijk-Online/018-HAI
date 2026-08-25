package runtimelab

import (
	"automation-hub-backend/internal/apierror"
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

func (h *Handler) FeatureParity(c *gin.Context) {
	overview, err := h.svc.FeatureParity()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "runtime feature parity is unavailable")})
		return
	}
	c.JSON(http.StatusOK, overview)
}

func (h *Handler) RuntimeFeatureParity(c *gin.Context) {
	inventory, ok, err := h.svc.RuntimeFeatureParity(c.Param("runtimeId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "runtime parity inventory is unavailable")})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "runtime parity inventory not found"})
		return
	}
	c.JSON(http.StatusOK, inventory)
}

func (h *Handler) Capabilities(c *gin.Context) {
	overview, err := h.svc.CapabilityCards(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "runtime capabilities are unavailable")})
		return
	}
	c.JSON(http.StatusOK, overview)
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
