package opscontrol

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the always-on runtime control API (§30/§31): background status,
// pause/resume (emergency stop), mode, recovery, and readiness.
type Handler struct {
	svc *Service
}

// NewHandler builds a handler over a service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) actor(c *gin.Context) string {
	if sub, ok := c.Get("subject"); ok {
		if s, ok := sub.(string); ok && s != "" {
			return s
		}
	}
	return "operator"
}

// Status returns the background/runtime status.
func (h *Handler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Status(c.Request.Context()))
}

type pauseRequest struct {
	Reason string `json:"reason"`
}

// Pause engages the emergency stop (halts background processing).
func (h *Handler) Pause(c *gin.Context) {
	var req pauseRequest
	_ = c.ShouldBindJSON(&req)
	state := h.svc.EngageEmergencyStop(req.Reason, h.actor(c))
	c.JSON(http.StatusOK, gin.H{"emergencyStop": state})
}

// Resume disengages the emergency stop.
func (h *Handler) Resume(c *gin.Context) {
	state := h.svc.DisengageEmergencyStop(h.actor(c))
	c.JSON(http.StatusOK, gin.H{"emergencyStop": state})
}

type modeRequest struct {
	Mode string `json:"mode"`
}

// SetMode updates the background autonomy mode.
func (h *Handler) SetMode(c *gin.Context) {
	var req modeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode, err := h.svc.SetMode(req.Mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": mode})
}

// Readiness returns the Windows-runtime readiness checklist.
func (h *Handler) Readiness(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Readiness(c.Request.Context()))
}

// Recovery runs a crash/reboot recovery pass.
func (h *Handler) Recovery(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Recover(c.Request.Context()))
}

// VerifyEmergencyStop proves the emergency stop halts background processing.
func (h *Handler) VerifyEmergencyStop(c *gin.Context) {
	v, err := h.svc.VerifyEmergencyStop(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	code := http.StatusOK
	if !v.Halted {
		code = http.StatusInternalServerError // a non-halting emergency stop is a hard failure
	}
	c.JSON(code, v)
}
