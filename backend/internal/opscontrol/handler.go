package opscontrol

import (
	"automation-hub-backend/internal/apierror"
	"errors"
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
	_ = c.ShouldBindJSON(&req)
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

// PrepareResume obtains an explicit, short-lived owner approval bound to the
// exact persisted emergency-stop revision. The approval cannot itself change
// state; Resume consumes it immediately.
func (h *Handler) PrepareResume(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated actor is required"})
		return
	}
	authorization, err := h.svc.PrepareEmergencyStopResume(actor)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{"error": publicControlError(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"idempotencyKey":        authorization.IdempotencyKey,
		"taskId":                authorization.TaskID,
		"approvalSourceId":      authorization.ApprovalSourceID,
		"approvalBindingDigest": authorization.ApprovalBindingDigest,
	})
}

type prepareModeRequest struct {
	Mode string `json:"mode"`
}

// PrepareModeChange returns an approval only for an autonomy escalation.
// Restrictive transitions deliberately remain usable without a fresh approval.
func (h *Handler) PrepareModeChange(c *gin.Context) {
	var request prepareModeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode is required"})
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated actor is required"})
		return
	}
	authorization, err := h.svc.PrepareAutonomyModeChange(actor, request.Mode)
	if err != nil {
		c.JSON(controlErrorStatus(err), gin.H{"error": publicControlError(err)})
		return
	}
	if authorization.IdempotencyKey == "" {
		c.JSON(http.StatusOK, gin.H{"authorizationRequired": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"authorizationRequired": true,
		"authorization": gin.H{
			"idempotencyKey":        authorization.IdempotencyKey,
			"taskId":                authorization.TaskID,
			"approvalSourceId":      authorization.ApprovalSourceID,
			"approvalBindingDigest": authorization.ApprovalBindingDigest,
		},
	})
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
			"error":         publicControlError(err),
			"emergencyStop": h.svc.Control().EmergencyState(),
		})
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
		c.JSON(controlErrorStatus(err), gin.H{"error": publicControlError(err)})
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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": publicControlError(err)})
		return
	}
	code := http.StatusOK
	if !v.Halted {
		code = http.StatusInternalServerError // a non-halting emergency stop is a hard failure
	}
	c.JSON(code, v)
}

func publicControlError(err error) string {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return "an authenticated actor is required"
	case errors.Is(err, ErrAuthorizationDenied):
		return "safety-control authorization was denied"
	case errors.Is(err, ErrAuthorizationMismatch):
		return "safety-control authorization does not match this change"
	case errors.Is(err, ErrAuthorizationUnavailable):
		return "safety-control authorization is unavailable"
	case errors.Is(err, ErrControlPersistence):
		return "safety-control state could not be persisted"
	case errors.Is(err, ErrEmergencyStopStateChanged), errors.Is(err, ErrAutonomyModeStateChanged):
		return "safety-control state changed; refresh and retry"
	default:
		return apierror.PublicMessage(err, "safety-control request could not be completed")
	}
}
