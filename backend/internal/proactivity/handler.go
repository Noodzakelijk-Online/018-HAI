package proactivity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxAdvisoryRequestBytes = 256 * 1024
	defaultAdvisoryLimit    = 100
	maxAdvisoryLimit        = 500
)

type Handler struct {
	service *Service
}

type RouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
	Govern             gin.HandlerFunc
}

type policyHTTPReq struct {
	IdempotencyKey string      `json:"idempotencyKey"`
	Policy         Preferences `json:"policy"`
}

type signalsHTTPReq struct {
	IdempotencyKey string           `json:"idempotencyKey"`
	Signals        []OpenLoopSignal `json:"signals"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// NewAdvisoryAPI is the package-local integration constructor. The returned
// handler still requires RegisterRoutes with all five external security guards.
func NewAdvisoryAPI(repository Repository) (*Handler, error) {
	if repositoryIsNil(repository) {
		return nil, ErrRepositoryUnavailable
	}
	return NewHandler(NewService(repository)), nil
}

// RegisterRoutes mounts the advisory API below parent at /proactivity. It
// refuses partial security wiring and never registers an unguarded fallback.
func RegisterRoutes(parent *gin.RouterGroup, handler *Handler, guards RouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil || repositoryIsNil(handler.service.repository) {
		return errors.New("proactivity route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil || guards.Govern == nil {
		return errors.New("proactivity routes require authenticated-owner, recognized-role, read, write, and govern guards")
	}
	routes := parent.Group("/proactivity")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.GET("/policy", guards.Read, handler.GetPolicy)
		routes.PUT("/policy", guards.Govern, handler.PutPolicy)
		routes.GET("/signals", guards.Read, handler.ListSignals)
		routes.POST("/signals", guards.Write, handler.RecordSignals)
		routes.GET("/decisions", guards.Read, handler.ListDecisions)
		routes.POST("/decisions/evaluate", guards.Write, handler.Evaluate)
		routes.GET("/inbox", guards.Read, handler.Inbox)
		routes.GET("/feedback", guards.Read, handler.ListFeedback)
		routes.POST("/feedback", guards.Write, handler.RecordFeedback)
	}
	return nil
}

func (h *Handler) Inbox(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	limit, ok := advisoryLimit(c)
	if !ok {
		return
	}
	result, err := h.service.Inbox(c.Request.Context(), owner, limit)
	respondAdvisory(c, result, err, http.StatusOK)
}

func (h *Handler) GetPolicy(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.service.CurrentPolicy(c.Request.Context(), owner)
	respondAdvisory(c, result, err, http.StatusOK)
}

func (h *Handler) PutPolicy(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request policyHTTPReq
	if !decodeAdvisoryJSON(c, &request) {
		return
	}
	result, created, err := h.service.RecordPolicy(c.Request.Context(), owner, request.IdempotencyKey, request.Policy)
	respondCreatedOrReplay(c, result, created, err)
}

func (h *Handler) ListSignals(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	limit, ok := advisoryLimit(c)
	if !ok {
		return
	}
	result, err := h.service.Signals(c.Request.Context(), owner, limit)
	respondAdvisory(c, gin.H{"signals": result}, err, http.StatusOK)
}

func (h *Handler) RecordSignals(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request signalsHTTPReq
	if !decodeAdvisoryJSON(c, &request) {
		return
	}
	result, created, err := h.service.RecordSignals(c.Request.Context(), owner, request.IdempotencyKey, request.Signals)
	respondCreatedOrReplay(c, gin.H{"signals": result}, created, err)
}

func (h *Handler) ListDecisions(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	limit, ok := advisoryLimit(c)
	if !ok {
		return
	}
	result, err := h.service.Decisions(c.Request.Context(), owner, limit)
	respondAdvisory(c, gin.H{"decisions": result}, err, http.StatusOK)
}

func (h *Handler) Evaluate(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request EvaluateStoredRequest
	if !decodeAdvisoryJSON(c, &request) {
		return
	}
	result, created, err := h.service.EvaluateStored(c.Request.Context(), owner, request)
	respondCreatedOrReplay(c, result, created, err)
}

func (h *Handler) ListFeedback(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	limit, ok := advisoryLimit(c)
	if !ok {
		return
	}
	result, err := h.service.Feedback(c.Request.Context(), owner, limit)
	respondAdvisory(c, gin.H{"feedback": result}, err, http.StatusOK)
}

func (h *Handler) RecordFeedback(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request FeedbackRequest
	if !decodeAdvisoryJSON(c, &request) {
		return
	}
	result, created, err := h.service.RecordFeedback(c.Request.Context(), owner, request)
	respondCreatedOrReplay(c, result, created, err)
}

func (h *Handler) owner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for proactivity access"})
		return "", false
	}
	if h == nil || h.service == nil || repositoryIsNil(h.service.repository) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proactivity advisory service is unavailable"})
		return "", false
	}
	return owner, true
}

func decodeAdvisoryJSON(c *gin.Context, target any) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "proactivity requests require application/json"})
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAdvisoryRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "proactivity request is too large"})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proactivity request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proactivity request must contain one JSON object"})
		return false
	}
	return true
}

func advisoryLimit(c *gin.Context) (int, bool) {
	values, exists := c.Request.URL.Query()["limit"]
	if !exists {
		return defaultAdvisoryLimit, true
	}
	if len(values) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proactivity limit must be provided exactly once"})
		return 0, false
	}
	limit, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil || limit < 1 || limit > maxAdvisoryLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("proactivity limit must be between 1 and %d", maxAdvisoryLimit)})
		return 0, false
	}
	return limit, true
}

func respondCreatedOrReplay(c *gin.Context, value any, created bool, err error) {
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respondAdvisory(c, value, err, status)
}

func respondAdvisory(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}
	status, message := advisoryErrorResponse(err)
	if status == http.StatusInternalServerError {
		errorID := uuid.NewString()
		_ = c.Error(fmt.Errorf("proactivity advisory operation failed (%s)", errorID))
		c.JSON(status, gin.H{"error": message, "errorId": errorID})
		return
	}
	c.JSON(status, gin.H{"error": message})
}

func advisoryErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrOwnerScopeViolation):
		return http.StatusNotFound, "proactivity record not found"
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, "proactivity idempotency conflict"
	case errors.Is(err, ErrInvalidLimit):
		return http.StatusBadRequest, "proactivity limit is invalid"
	case errors.Is(err, ErrRepositoryUnavailable):
		return http.StatusServiceUnavailable, "proactivity advisory service is unavailable"
	case isAdvisoryValidationError(err):
		return http.StatusBadRequest, "proactivity request failed validation"
	default:
		return http.StatusInternalServerError, "proactivity advisory operation failed"
	}
}

func isAdvisoryValidationError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, fragment := range []string{
		"is required", "is invalid", "must be", "must contain", "unsupported ",
		"exceeds ", "at least ", "in the future", "must match", "does not match",
		"secret material",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
