package standingmandate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxStandingMandateRequestBytes = 128 * 1024

type Handler struct {
	service *Service
}

type revisionRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
}

type revokeMandateRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	Reason           string `json:"reason"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	owner, ok := mandateOwner(c)
	if !ok {
		return
	}
	var input struct {
		Name             string          `json:"name"`
		Purpose          string          `json:"purpose"`
		Version          string          `json:"version,omitempty"`
		Scopes           []Scope         `json:"scopes"`
		AutonomyCeiling  int             `json:"autonomyCeiling"`
		ApprovalPolicy   ApprovalPolicy  `json:"approvalPolicy"`
		StopConditions   []StopCondition `json:"stopConditions,omitempty"`
		SourceReferences []string        `json:"sourceReferences,omitempty"`
		ExpiresAt        *time.Time      `json:"expiresAt,omitempty"`
	}
	if !decodeStandingMandateJSON(c, &input) {
		return
	}
	result, err := h.service.Create(c.Request.Context(), CreateRequest{
		OwnerIdentity: owner, Name: input.Name, Purpose: input.Purpose,
		Version: input.Version, Scopes: input.Scopes, AutonomyCeiling: input.AutonomyCeiling,
		ApprovalPolicy: input.ApprovalPolicy, StopConditions: input.StopConditions,
		SourceReferences: input.SourceReferences, CreatedBy: owner, ExpiresAt: input.ExpiresAt,
	})
	respondStandingMandate(c, result, err, http.StatusCreated)
}

func (h *Handler) List(c *gin.Context) {
	owner, ok := mandateOwner(c)
	if !ok {
		return
	}
	result, err := h.service.List(c.Request.Context(), owner)
	respondStandingMandate(c, gin.H{"mandates": result}, err, http.StatusOK)
}

func (h *Handler) Get(c *gin.Context) {
	owner, id, ok := mandateOwnerAndID(c)
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), owner, id)
	respondStandingMandate(c, result, err, http.StatusOK)
}

func (h *Handler) Activate(c *gin.Context) {
	owner, id, ok := mandateOwnerAndID(c)
	if !ok {
		return
	}
	var request revisionRequest
	if !decodeStandingMandateJSON(c, &request) {
		return
	}
	result, err := h.service.Activate(c.Request.Context(), owner, id, request.ExpectedRevision)
	respondStandingMandate(c, result, err, http.StatusOK)
}

func (h *Handler) Revoke(c *gin.Context) {
	owner, id, ok := mandateOwnerAndID(c)
	if !ok {
		return
	}
	var request revokeMandateRequest
	if !decodeStandingMandateJSON(c, &request) {
		return
	}
	result, err := h.service.Revoke(
		c.Request.Context(), owner, id, request.ExpectedRevision, owner, request.Reason,
	)
	respondStandingMandate(c, result, err, http.StatusOK)
}

func (h *Handler) Authorize(c *gin.Context) {
	owner, id, ok := mandateOwnerAndID(c)
	if !ok {
		return
	}
	var request ActionRequest
	if !decodeStandingMandateJSON(c, &request) {
		return
	}
	// HTTP callers cannot assert another owner or runtime actor. Cryptographic
	// runtime identity binding is a separate execution-layer responsibility.
	request.OwnerIdentity = owner
	request.ActorIdentity = owner
	result, err := h.service.Authorize(c.Request.Context(), id, request)
	respondStandingMandate(c, result, err, http.StatusOK)
}

func (h *Handler) Decisions(c *gin.Context) {
	owner, ok := mandateOwner(c)
	if !ok {
		return
	}
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 1000"})
			return
		}
		limit = parsed
	}
	var mandateID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("mandateId")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a valid mandate id is required"})
			return
		}
		mandateID = &parsed
	}
	result, err := h.service.ListDecisions(c.Request.Context(), owner, mandateID, limit)
	respondStandingMandate(c, gin.H{"decisions": result}, err, http.StatusOK)
}

func mandateOwner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for standing mandates"})
		return "", false
	}
	return owner, true
}

func mandateOwnerAndID(c *gin.Context) (string, uuid.UUID, bool) {
	owner, ok := mandateOwner(c)
	if !ok {
		return "", uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a valid mandate id is required"})
		return "", uuid.Nil, false
	}
	return owner, id, true
}

func decodeStandingMandateJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStandingMandateRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "standing mandate request is too large"})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid standing mandate request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "standing mandate request must contain one JSON object"})
		return false
	}
	return true
}

func respondStandingMandate(c *gin.Context, value any, err error, status int) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "standing mandate record not found"})
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrDecisionConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "standing mandate state conflict"})
	case isStandingMandateValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": compactStandingMandateError(err.Error())})
	default:
		errorID := uuid.NewString()
		_ = c.Error(fmt.Errorf("standing mandate error %s: %w", errorID, err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "standing mandate operation failed", "errorId": errorID,
		})
	}
}

func isStandingMandateValidationError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"is required", "must ", "cannot ", "invalid ", "outside", "exceeds",
		"expired", "not active", "only draft", "already revoked",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func compactStandingMandateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 300 {
		return "standing mandate request failed validation"
	}
	return message
}
