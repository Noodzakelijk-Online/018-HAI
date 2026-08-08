package plangraph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxRequestBytes int64 = 512 * 1024

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (handler *Handler) List(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	value, err := handler.service.List(c.Request.Context(), owner)
	respond(c, value, err, http.StatusOK)
}

func (handler *Handler) Get(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	var revision uint64
	if raw := strings.TrimSpace(c.Query("revision")); raw != "" {
		revision, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || revision == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan revision"})
			return
		}
	}
	value, err := handler.service.Get(c.Request.Context(), owner, id, revision)
	respond(c, value, err, http.StatusOK)
}

func (handler *Handler) Preview(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	var request PreviewRequest
	if !decodeJSON(c, &request) {
		return
	}
	request.CreatedBy = owner
	value, err := handler.service.Preview(c.Request.Context(), owner, request)
	respond(c, value, err, http.StatusCreated)
}

func (handler *Handler) Accept(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	var request AcceptRequest
	if !decodeJSON(c, &request) {
		return
	}
	request.AcceptedBy = owner
	value, err := handler.service.Accept(c.Request.Context(), owner, id, request)
	respond(c, value, err, http.StatusOK)
}

func (handler *Handler) Replan(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	var request ReplanRequest
	if !decodeJSON(c, &request) {
		return
	}
	request.CreatedBy = owner
	value, err := handler.service.Replan(c.Request.Context(), owner, id, request)
	respond(c, value, err, http.StatusCreated)
}

func (handler *Handler) owner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for plan access"})
		return "", false
	}
	if handler == nil || handler.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plan graph service is unavailable"})
		return "", false
	}
	return owner, true
}

func decodeJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "plan graph request is too large"})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan graph request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan graph request must contain one JSON object"})
		return false
	}
	return true
}

func respond(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "plan graph not found"})
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrIdempotencyConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "plan graph state conflict"})
	case isValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan graph request failed validation"})
	default:
		errorID := uuid.NewString()
		_ = c.Error(fmt.Errorf("plan graph operation failed (%s): %w", errorID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plan graph operation failed", "errorId": errorID})
	}
}

func isValidationError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, fragment := range []string{" is required", " must ", " cannot ", "invalid ", "duplicate ", "unknown ", "contains a cycle", "outside the allowed range", "exceed"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
