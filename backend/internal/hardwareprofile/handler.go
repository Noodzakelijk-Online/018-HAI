package hardwareprofile

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the Hardware + Power API (§10.19).
type Handler struct {
	svc *Service
}

// NewHandler builds a handler over a service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// DefaultHandler builds a handler over the default service.
func DefaultHandler() *Handler { return NewHandler(DefaultService()) }

func (h *Handler) Profile(c *gin.Context) {
	profile := h.svc.Get()
	c.JSON(http.StatusOK, gin.H{"profile": profile, "selectedServingStack": profile.SelectServingStack()})
}

func (h *Handler) Detect(c *gin.Context) {
	profile := h.svc.Detect()
	c.JSON(http.StatusOK, gin.H{"profile": profile, "selectedServingStack": profile.SelectServingStack()})
}

func (h *Handler) Patch(c *gin.Context) {
	var patch HardwareProfilePatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate any provided execution providers.
	if patch.ExecutionProviders != nil {
		for _, ep := range *patch.ExecutionProviders {
			if !ep.IsValid() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution provider: " + ep.String()})
				return
			}
		}
	}
	profile := h.svc.Patch(patch)
	c.JSON(http.StatusOK, gin.H{"profile": profile, "selectedServingStack": profile.SelectServingStack()})
}

func (h *Handler) PowerPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Power())
}

func (h *Handler) UpdatePowerPolicy(c *gin.Context) {
	var p PowerPolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.svc.SetPower(p))
}
