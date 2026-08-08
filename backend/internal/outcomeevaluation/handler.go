package outcomeevaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxOutcomeEvaluationRequestBytes = 4 * 1024 * 1024

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RouteGuards leaves authentication and authorization policy in the owning
// router while refusing to register any route when a guard is omitted.
type RouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
	Govern             gin.HandlerFunc
}

// RegisterRoutes mounts the advisory API below the supplied API group.
func RegisterRoutes(parent *gin.RouterGroup, handler *Handler, guards RouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil || handler.service.repository == nil {
		return errors.New("outcome evaluation route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil || guards.Govern == nil {
		return errors.New("outcome evaluation routes require authentication, recognized-role, read, write, and govern guards")
	}
	routes := parent.Group("/outcome-evaluations/workspaces/:workspaceId/outcomes")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.PUT("/:outcomeId", guards.Govern, handler.StoreOutcome)
		routes.GET("/:outcomeId", guards.Read, handler.GetOutcome)
		routes.GET("/:outcomeId/history", guards.Read, handler.OutcomeHistory)
		routes.POST("/:outcomeId/evaluations", guards.Write, handler.CreateEvaluation)
		routes.GET("/:outcomeId/evaluations", guards.Read, handler.Evaluations)
		routes.GET("/:outcomeId/evaluations/:evaluationId", guards.Read, handler.GetEvaluation)
		routes.POST("/:outcomeId/corrections", guards.Write, handler.StoreCorrection)
		routes.GET("/:outcomeId/corrections", guards.Read, handler.Corrections)
	}
	return nil
}

func (h *Handler) StoreOutcome(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	var request StoreOutcomeRequest
	if !decodeStrictJSON(c, &request) {
		return
	}
	// Identity and resource scope are authority-bearing server inputs. Ignore
	// browser-supplied values so a client cannot assert another owner or bind an
	// outcome to a different URL than the resource being written.
	request.Outcome.ID = outcomeID
	request.Outcome.Scope = Scope{OwnerID: ownerID, WorkspaceID: workspaceID}
	for index := range request.Outcome.Indicators {
		request.Outcome.Indicators[index].Baseline.Scope = request.Outcome.Scope
	}
	result, created, err := h.service.StoreOutcome(c.Request.Context(), ownerID, workspaceID, outcomeID, request)
	status := http.StatusOK
	if created && result.Revision == 1 {
		status = http.StatusCreated
	}
	respondOutcomeEvaluation(c, result, err, status)
}

func (h *Handler) GetOutcome(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetOutcome(c.Request.Context(), ownerID, workspaceID, outcomeID)
	respondOutcomeEvaluation(c, result, err, http.StatusOK)
}

func (h *Handler) OutcomeHistory(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	result, err := h.service.OutcomeHistory(c.Request.Context(), ownerID, workspaceID, outcomeID)
	respondOutcomeEvaluation(c, gin.H{"revisions": result}, err, http.StatusOK)
}

func (h *Handler) CreateEvaluation(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	var request CreateEvaluationRequest
	if !decodeStrictJSON(c, &request) {
		return
	}
	bindEvaluationRequestScope(&request, ownerID, workspaceID)
	result, created, err := h.service.CreateEvaluation(c.Request.Context(), ownerID, workspaceID, outcomeID, request)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respondOutcomeEvaluation(c, result, err, status)
}

func (h *Handler) GetEvaluation(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	evaluationID := strings.TrimSpace(c.Param("evaluationId"))
	if err := validateText("evaluation id", evaluationID, maxIDRunes+64, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outcome evaluation resource id"})
		return
	}
	result, err := h.service.GetEvaluation(c.Request.Context(), ownerID, workspaceID, outcomeID, evaluationID)
	respondOutcomeEvaluation(c, result, err, http.StatusOK)
}

func (h *Handler) Evaluations(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	result, err := h.service.Evaluations(c.Request.Context(), ownerID, workspaceID, outcomeID)
	respondOutcomeEvaluation(c, gin.H{"evaluations": result}, err, http.StatusOK)
}

func (h *Handler) StoreCorrection(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	var request StoreCorrectionRequest
	if !decodeStrictJSON(c, &request) {
		return
	}
	bindObservationScope(&request.Observation, ownerID, workspaceID)
	request.Correction.Scope = Scope{OwnerID: ownerID, WorkspaceID: workspaceID}
	request.Correction.ActorID = ownerID
	result, created, err := h.service.StoreCorrection(c.Request.Context(), ownerID, workspaceID, outcomeID, request)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respondOutcomeEvaluation(c, result, err, status)
}

func bindEvaluationRequestScope(request *CreateEvaluationRequest, ownerID, workspaceID string) {
	if request == nil {
		return
	}
	for index := range request.Observations {
		bindObservationScope(&request.Observations[index], ownerID, workspaceID)
	}
	for index := range request.Corrections {
		request.Corrections[index].Scope = Scope{OwnerID: ownerID, WorkspaceID: workspaceID}
		request.Corrections[index].ActorID = ownerID
	}
}

func bindObservationScope(observation *Observation, ownerID, workspaceID string) {
	if observation == nil {
		return
	}
	observation.Scope = Scope{OwnerID: ownerID, WorkspaceID: workspaceID}
}

func (h *Handler) Corrections(c *gin.Context) {
	ownerID, workspaceID, outcomeID, ok := h.requestScope(c)
	if !ok {
		return
	}
	result, err := h.service.Corrections(c.Request.Context(), ownerID, workspaceID, outcomeID)
	respondOutcomeEvaluation(c, gin.H{"corrections": result}, err, http.StatusOK)
}

func (h *Handler) requestScope(c *gin.Context) (string, string, string, bool) {
	if h == nil || h.service == nil || h.service.repository == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "outcome evaluation is unavailable"})
		return "", "", "", false
	}
	value, exists := c.Get(identity.ContextSubjectKey)
	ownerID, ownerOK := value.(string)
	ownerID = strings.TrimSpace(ownerID)
	if !exists || !ownerOK || ownerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required"})
		return "", "", "", false
	}
	workspaceID := strings.TrimSpace(c.Param("workspaceId"))
	outcomeID := strings.TrimSpace(c.Param("outcomeId"))
	if _, _, _, err := validateServiceScope(ownerID, workspaceID, outcomeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outcome evaluation resource id"})
		return "", "", "", false
	}
	return ownerID, workspaceID, outcomeID, true
}

func decodeStrictJSON(c *gin.Context, target any) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "outcome evaluation requests require application/json"})
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOutcomeEvaluationRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "outcome evaluation request is too large"})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outcome evaluation request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outcome evaluation request must contain one JSON object"})
		return false
	}
	return true
}

func respondOutcomeEvaluation(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}
	status, message := outcomeEvaluationErrorResponse(err)
	if status == http.StatusInternalServerError {
		errorID := uuid.NewString()
		_ = c.Error(fmt.Errorf("outcome evaluation operation failed (%s)", errorID))
		c.JSON(status, gin.H{"error": message, "errorId": errorID})
		return
	}
	c.JSON(status, gin.H{"error": message})
}

func outcomeEvaluationErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrScopeViolation):
		return http.StatusNotFound, "outcome evaluation record not found"
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, "outcome evaluation state conflict"
	case errors.Is(err, ErrIntegrityViolation):
		return http.StatusConflict, "outcome evaluation record failed integrity verification"
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrSecretMaterial),
		errors.Is(err, ErrInvalidTimeWindow), errors.Is(err, ErrMissingProvenance):
		return http.StatusBadRequest, "outcome evaluation request failed validation"
	default:
		return http.StatusInternalServerError, "outcome evaluation operation failed"
	}
}
