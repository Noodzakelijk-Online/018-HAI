package opscontrol

import (
	"errors"
	"io"
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

func (h *Handler) actor(c *gin.Context) (string, bool) {
	if sub, ok := c.Get("subject"); ok {
		if s, ok := sub.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
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
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pause request must be valid JSON"})
		return
	}
	actor, _ := h.actor(c)
	state, err := h.svc.EngageEmergencyStop(req.Reason, actor)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":         "failed to persist emergency-stop state; execution remains blocked",
			"emergencyStop": h.svc.Control().EmergencyState(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"emergencyStop": state})
}

type controlAuthorizationRequest struct {
	IdempotencyKey        string `json:"idempotencyKey"`
	TaskID                string `json:"taskId"`
	ApprovalSourceID      string `json:"approvalSourceId"`
	ApprovalBindingDigest string `json:"approvalBindingDigest"`
}

func (h *Handler) controlAuthorization(
	c *gin.Context,
	req controlAuthorizationRequest,
) ControlAuthorization {
	actor, _ := h.actor(c)
	return ControlAuthorization{
		ActorIdentity:         actor,
		IdempotencyKey:        req.IdempotencyKey,
		TaskID:                req.TaskID,
		ApprovalSourceID:      req.ApprovalSourceID,
		ApprovalBindingDigest: req.ApprovalBindingDigest,
	}
}

// Resume disengages the emergency stop.
func (h *Handler) Resume(c *gin.Context) {
	var req controlAuthorizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authorization request is required"})
		return
	}
	state, err := h.svc.DisengageEmergencyStop(
		c.Request.Context(),
		h.controlAuthorization(c, req),
	)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{
			"error":         err.Error(),
			"emergencyStop": h.svc.Control().EmergencyState(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"emergencyStop": state})
}

// RequestResumeApproval creates the exact owner review required to clear the
// current emergency-stop revision. It does not resume anything by itself.
func (h *Handler) RequestResumeApproval(c *gin.Context) {
	actor, _ := h.actor(c)
	approval, err := h.svc.RequestResumeApproval(c.Request.Context(), actor)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, approval)
}

type approveResumeRequest struct {
	Confirmation string `json:"confirmation"`
	Note         string `json:"note"`
}

const resumeApprovalConfirmation = "RESUME BACKGROUND PROCESSING"

// ApproveAndResume records the owner's explicit confirmation and consumes the
// resulting fresh, exact-bound approval in one API action.
func (h *Handler) ApproveAndResume(c *gin.Context) {
	var req approveResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "approval confirmation is required"})
		return
	}
	if req.Confirmation != resumeApprovalConfirmation {
		c.JSON(http.StatusBadRequest, gin.H{"error": "exact confirmation is required", "confirmation": resumeApprovalConfirmation})
		return
	}
	actor, _ := h.actor(c)
	state, err := h.svc.ApproveAndResume(c.Request.Context(), actor, c.Param("id"), req.Note)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{"error": err.Error(), "emergencyStop": h.svc.Control().EmergencyState()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"emergencyStop": state})
}

type modeRequest struct {
	Mode string `json:"mode"`
	controlAuthorizationRequest
}

// SetMode updates the background autonomy mode.
func (h *Handler) SetMode(c *gin.Context) {
	var req modeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode, err := h.svc.SetMode(
		c.Request.Context(),
		req.Mode,
		h.controlAuthorization(c, req.controlAuthorizationRequest),
	)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": mode})
}

func controlErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized
	case errors.Is(err, ErrAuthorizationDenied),
		errors.Is(err, ErrAuthorizationMismatch):
		return http.StatusForbidden
	case errors.Is(err, ErrAuthorizationUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrControlPersistence):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrEmergencyStopStateChanged),
		errors.Is(err, ErrAutonomyModeStateChanged):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
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
