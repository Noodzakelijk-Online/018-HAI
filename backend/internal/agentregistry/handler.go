package agentregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxAgentRegistryRequestBytes = 64 * 1024

type Handler struct {
	service *Service
}

type registerAgentRequest struct {
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	Type             AgentType               `json:"type"`
	Runtime          RuntimeAdapter          `json:"runtime"`
	Capabilities     []CapabilityDeclaration `json:"capabilities"`
	AuthorityCeiling int                     `json:"authorityCeiling"`
	AutonomyCeiling  int                     `json:"autonomyCeiling"`
	ToolAllowlist    []string                `json:"toolAllowlist,omitempty"`
	DataAllowlist    []string                `json:"dataAllowlist,omitempty"`
	FolderAllowlist  []string                `json:"folderAllowlist,omitempty"`
	Health           HealthEvidence          `json:"health"`
	Availability     Availability            `json:"availability"`
	Performance      PerformanceProfile      `json:"performance"`
}

type updateAgentRequest struct {
	ExpectedRevision uint64                  `json:"expectedRevision"`
	Name             string                  `json:"name"`
	Type             AgentType               `json:"type"`
	Runtime          RuntimeAdapter          `json:"runtime"`
	Capabilities     []CapabilityDeclaration `json:"capabilities"`
	AuthorityCeiling int                     `json:"authorityCeiling"`
	AutonomyCeiling  int                     `json:"autonomyCeiling"`
	ToolAllowlist    []string                `json:"toolAllowlist,omitempty"`
	DataAllowlist    []string                `json:"dataAllowlist,omitempty"`
	FolderAllowlist  []string                `json:"folderAllowlist,omitempty"`
	Health           HealthEvidence          `json:"health"`
	Availability     Availability            `json:"availability"`
	Performance      PerformanceProfile      `json:"performance"`
}

type transitionAgentRequest struct {
	ExpectedRevision uint64         `json:"expectedRevision"`
	To               LifecycleState `json:"to"`
	Reason           string         `json:"reason"`
}

type assignAgentRequest struct {
	TaskID              string                   `json:"taskId"`
	Capabilities        []CapabilityRequirement  `json:"capabilities"`
	Compatibility       CompatibilityRequirement `json:"compatibility"`
	RequiredAuthority   int                      `json:"requiredAuthority"`
	RequiredAutonomy    int                      `json:"requiredAutonomy"`
	PolicyMaxAuthority  int                      `json:"policyMaxAuthority"`
	PolicyMaxAutonomy   int                      `json:"policyMaxAutonomy"`
	RequiredTools       []string                 `json:"requiredTools,omitempty"`
	RequiredData        []string                 `json:"requiredData,omitempty"`
	RequiredFolders     []string                 `json:"requiredFolders,omitempty"`
	AllowedAgentTypes   []AgentType              `json:"allowedAgentTypes,omitempty"`
	MaxEstimatedCostEUR *float64                 `json:"maxEstimatedCostEur,omitempty"`
	RequireLocal        bool                     `json:"requireLocal"`
	AllowDegraded       bool                     `json:"allowDegraded"`
}

type recordOutcomeRequest struct {
	ExpectedRevision uint64        `json:"expectedRevision"`
	Success          bool          `json:"success"`
	Latency          time.Duration `json:"latency"`
	RecordedAt       time.Time     `json:"recordedAt,omitempty"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request registerAgentRequest
	if !decodeAgentRegistryJSON(c, &request) {
		return
	}
	result, err := h.service.Register(c.Request.Context(), Agent{
		ID:               request.ID,
		OwnerIdentity:    owner,
		Name:             request.Name,
		Type:             request.Type,
		Runtime:          request.Runtime,
		Capabilities:     request.Capabilities,
		AuthorityCeiling: request.AuthorityCeiling,
		AutonomyCeiling:  request.AutonomyCeiling,
		ToolAllowlist:    request.ToolAllowlist,
		DataAllowlist:    request.DataAllowlist,
		FolderAllowlist:  request.FolderAllowlist,
		Health:           request.Health,
		Availability:     request.Availability,
		Performance:      request.Performance,
	})
	respondAgentRegistry(c, result, err, http.StatusCreated)
}

func (h *Handler) List(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.service.List(c.Request.Context(), owner)
	respondAgentRegistry(c, gin.H{"agents": result}, err, http.StatusOK)
}

func (h *Handler) Get(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), owner, c.Param("id"))
	respondAgentRegistry(c, result, err, http.StatusOK)
}

func (h *Handler) Update(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request updateAgentRequest
	if !decodeAgentRegistryJSON(c, &request) {
		return
	}
	if request.ExpectedRevision == 0 {
		respondAgentRegistry(c, nil, fmt.Errorf("expected revision must be positive"), http.StatusOK)
		return
	}
	result, err := h.service.Update(c.Request.Context(), owner, Agent{
		ID:               c.Param("id"),
		Name:             request.Name,
		Type:             request.Type,
		Runtime:          request.Runtime,
		Capabilities:     request.Capabilities,
		AuthorityCeiling: request.AuthorityCeiling,
		AutonomyCeiling:  request.AutonomyCeiling,
		ToolAllowlist:    request.ToolAllowlist,
		DataAllowlist:    request.DataAllowlist,
		FolderAllowlist:  request.FolderAllowlist,
		Health:           request.Health,
		Availability:     request.Availability,
		Performance:      request.Performance,
	}, request.ExpectedRevision)
	respondAgentRegistry(c, result, err, http.StatusOK)
}

func (h *Handler) Transition(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request transitionAgentRequest
	if !decodeAgentRegistryJSON(c, &request) {
		return
	}
	if request.ExpectedRevision == 0 {
		respondAgentRegistry(c, nil, fmt.Errorf("expected revision must be positive"), http.StatusOK)
		return
	}
	result, err := h.service.Transition(
		c.Request.Context(),
		owner,
		c.Param("id"),
		request.ExpectedRevision,
		request.To,
		request.Reason,
	)
	respondAgentRegistry(c, result, err, http.StatusOK)
}

func (h *Handler) ListTransitions(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.service.ListTransitions(c.Request.Context(), owner, c.Param("id"))
	respondAgentRegistry(c, gin.H{"transitions": result}, err, http.StatusOK)
}

func (h *Handler) Assign(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request assignAgentRequest
	if !decodeAgentRegistryJSON(c, &request) {
		return
	}
	result, err := h.service.Assign(c.Request.Context(), AssignmentRequest{
		OwnerIdentity:       owner,
		TaskID:              request.TaskID,
		Capabilities:        request.Capabilities,
		Compatibility:       request.Compatibility,
		RequiredAuthority:   request.RequiredAuthority,
		RequiredAutonomy:    request.RequiredAutonomy,
		PolicyMaxAuthority:  request.PolicyMaxAuthority,
		PolicyMaxAutonomy:   request.PolicyMaxAutonomy,
		RequiredTools:       request.RequiredTools,
		RequiredData:        request.RequiredData,
		RequiredFolders:     request.RequiredFolders,
		AllowedAgentTypes:   request.AllowedAgentTypes,
		MaxEstimatedCostEUR: request.MaxEstimatedCostEUR,
		RequireLocal:        request.RequireLocal,
		AllowDegraded:       request.AllowDegraded,
	})
	respondAgentRegistry(c, result, err, http.StatusCreated)
}

func (h *Handler) GetAssignment(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.service.GetAssignment(c.Request.Context(), owner, c.Param("id"))
	respondAgentRegistry(c, result, err, http.StatusOK)
}

func (h *Handler) RecordAssignmentOutcome(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var request recordOutcomeRequest
	if !decodeAgentRegistryJSON(c, &request) {
		return
	}
	if request.ExpectedRevision == 0 {
		respondAgentRegistry(c, nil, fmt.Errorf("expected revision must be positive"), http.StatusOK)
		return
	}
	result, err := h.service.RecordAssignmentOutcome(
		c.Request.Context(),
		owner,
		c.Param("id"),
		request.ExpectedRevision,
		Outcome{
			Success:    request.Success,
			Latency:    request.Latency,
			RecordedAt: request.RecordedAt,
		},
	)
	respondAgentRegistry(c, result, err, http.StatusOK)
}

func (h *Handler) owner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "an authenticated owner session is required for agent registry access",
		})
		return "", false
	}
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent registry is unavailable"})
		return "", false
	}
	return owner, true
}

func decodeAgentRegistryJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAgentRegistryRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "agent registry request is too large"})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent registry request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agent registry request must contain one JSON object",
		})
		return false
	}
	return true
}

func respondAgentRegistry(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}

	status, publicMessage := agentRegistryErrorResponse(err)
	if status == http.StatusInternalServerError {
		errorID := uuid.NewString()
		// Never attach the underlying error to Gin: repository failures may
		// contain credentials or provider payloads. The opaque ID is enough to
		// correlate this response with separately redacted service telemetry.
		_ = c.Error(fmt.Errorf("agent registry operation failed (%s)", errorID))
		c.JSON(status, gin.H{"error": publicMessage, "errorId": errorID})
		return
	}
	c.JSON(status, gin.H{"error": publicMessage})
}

func agentRegistryErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "agent registry record not found"
	case errors.Is(err, ErrNoEligibleAgent):
		return http.StatusUnprocessableEntity, "no eligible agent is available"
	case errors.Is(err, ErrConflict),
		errors.Is(err, ErrAlreadyExists),
		errors.Is(err, ErrAssignmentExists),
		errors.Is(err, ErrInvalidTransition):
		return http.StatusConflict, "agent registry state conflict"
	}
	if isAgentRegistryValidationError(strings.ToLower(strings.TrimSpace(err.Error()))) {
		return http.StatusBadRequest, "agent registry request failed validation"
	}
	return http.StatusInternalServerError, "agent registry operation failed"
}

func isAgentRegistryValidationError(message string) bool {
	for _, fragment := range []string{
		"unsupported ",
		"invalid ",
		" is required",
		" must ",
		" cannot ",
		"at least ",
		"duplicate ",
		" exceeds ",
		"between ",
		"negative",
		"in the future",
		"version ",
		"freshness",
		"allowlist",
		"contains credentials",
		"contains secret material",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
