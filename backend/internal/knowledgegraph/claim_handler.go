package knowledgegraph

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

const maxClaimRequestBytes = 256 << 10

type ClaimHandler struct {
	service *Service
}

func NewClaimHandler(service *Service) *ClaimHandler {
	return &ClaimHandler{service: service}
}

type ClaimRouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
	Approve            gin.HandlerFunc
}

func RegisterClaimRoutes(parent *gin.RouterGroup, handler *ClaimHandler, guards ClaimRouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil {
		return errors.New("knowledge claim route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil || guards.Approve == nil {
		return errors.New("knowledge claim routes require authentication, recognized-role, and permission guards")
	}
	routes := parent.Group("/knowledge/claims")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.GET("", guards.Read, handler.List)
		routes.POST("", guards.Write, handler.Record)
		routes.GET("/review-queue", guards.Read, handler.ReviewQueue)
		routes.GET("/:id", guards.Read, handler.Get)
		routes.GET("/:id/lifecycle", guards.Read, handler.Lifecycle)
		routes.GET("/:id/assessment", guards.Read, handler.Assessment)
		routes.POST("/:id/corrections", guards.Approve, handler.Correct)
	}
	return nil
}

func (h *ClaimHandler) ReviewQueue(c *gin.Context) {
	owner, workspace, ok := claimScope(c)
	if !ok {
		return
	}
	effectiveAt, err := parseClaimTime(c.Query("effectiveAt"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effectiveAt timestamp"})
		return
	}
	observedBy, err := parseClaimTime(c.Query("observedBy"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid observedBy timestamp"})
		return
	}
	queue, err := h.service.ReviewClaims(c.Request.Context(), owner, workspace, ClaimAssessmentQuery{
		EffectiveAt: effectiveAt, ObservedBy: observedBy,
	})
	respondClaim(c, queue, err, http.StatusOK)
}

type recordClaimBody struct {
	WorkspaceID        string             `json:"workspaceId"`
	Subject            string             `json:"subject"`
	Predicate          string             `json:"predicate"`
	Object             string             `json:"object"`
	EffectiveFrom      time.Time          `json:"effectiveFrom"`
	EffectiveUntil     *time.Time         `json:"effectiveUntil,omitempty"`
	ObservedAt         time.Time          `json:"observedAt"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Provenance         []ClaimProvenance  `json:"provenance"`
	SupersedesClaimIDs []string           `json:"supersedesClaimIds,omitempty"`
	ConflictsWithIDs   []string           `json:"conflictsWithIds,omitempty"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
}

func (h *ClaimHandler) Record(c *gin.Context) {
	owner, ok := claimOwner(c)
	if !ok {
		return
	}
	var body recordClaimBody
	if !decodeClaimJSON(c, &body) {
		return
	}
	if !callerAssignableClaimStatus(body.VerificationStatus) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "verification status requires a trusted verification or approval workflow"})
		return
	}
	claim, err := h.service.RecordClaim(c.Request.Context(), RecordClaimRequest{
		OwnerIdentity: owner, WorkspaceID: body.WorkspaceID,
		Subject: body.Subject, Predicate: body.Predicate, Object: body.Object,
		EffectiveFrom: body.EffectiveFrom, EffectiveUntil: body.EffectiveUntil,
		ObservedAt: body.ObservedAt, VerificationStatus: body.VerificationStatus,
		Provenance: body.Provenance, SupersedesClaimIDs: body.SupersedesClaimIDs,
		ConflictsWithIDs: body.ConflictsWithIDs, Sensitivity: body.Sensitivity,
		LocalOnly: body.LocalOnly,
	})
	respondClaim(c, claim, err, http.StatusCreated)
}

type correctClaimBody struct {
	WorkspaceID     string     `json:"workspaceId"`
	RequestID       string     `json:"requestId"`
	CorrectedObject string     `json:"correctedObject"`
	Reason          string     `json:"reason"`
	EffectiveFrom   *time.Time `json:"effectiveFrom,omitempty"`
}

func (h *ClaimHandler) Correct(c *gin.Context) {
	owner, ok := claimOwner(c)
	if !ok {
		return
	}
	var body correctClaimBody
	if !decodeClaimJSON(c, &body) {
		return
	}
	claim, err := h.service.CorrectClaim(
		c.Request.Context(), owner, body.WorkspaceID, c.Param("id"),
		CorrectClaimRequest{
			RequestID: body.RequestID, CorrectedObject: body.CorrectedObject,
			Reason: body.Reason, EffectiveFrom: body.EffectiveFrom,
		},
	)
	respondClaim(c, claim, err, http.StatusCreated)
}

func callerAssignableClaimStatus(status VerificationStatus) bool {
	switch status {
	case "", VerificationUnverified, VerificationUncertain, VerificationUnsupported, VerificationNeedsReview:
		return true
	default:
		return false
	}
}

func (h *ClaimHandler) Get(c *gin.Context) {
	owner, workspace, ok := claimScope(c)
	if !ok {
		return
	}
	claim, err := h.service.GetClaim(c.Request.Context(), owner, workspace, c.Param("id"))
	respondClaim(c, claim, err, http.StatusOK)
}

func (h *ClaimHandler) List(c *gin.Context) {
	owner, workspace, ok := claimScope(c)
	if !ok {
		return
	}
	query, err := claimQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid knowledge claim query"})
		return
	}
	claims, err := h.service.ListClaims(c.Request.Context(), owner, workspace, query)
	respondClaim(c, gin.H{"claims": claims}, err, http.StatusOK)
}

func (h *ClaimHandler) Lifecycle(c *gin.Context) {
	owner, workspace, ok := claimScope(c)
	if !ok {
		return
	}
	lifecycle, err := h.service.GetClaimLifecycle(c.Request.Context(), owner, workspace, c.Param("id"))
	respondClaim(c, lifecycle, err, http.StatusOK)
}

func (h *ClaimHandler) Assessment(c *gin.Context) {
	owner, workspace, ok := claimScope(c)
	if !ok {
		return
	}
	effectiveAt, err := parseClaimTime(c.Query("effectiveAt"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effectiveAt timestamp"})
		return
	}
	observedBy, err := parseClaimTime(c.Query("observedBy"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid observedBy timestamp"})
		return
	}
	assessment, err := h.service.AssessClaim(c.Request.Context(), owner, workspace, c.Param("id"), ClaimAssessmentQuery{
		EffectiveAt: effectiveAt,
		ObservedBy:  observedBy,
	})
	respondClaim(c, assessment, err, http.StatusOK)
}

func claimScope(c *gin.Context) (string, string, bool) {
	owner, ok := claimOwner(c)
	if !ok {
		return "", "", false
	}
	workspace := strings.TrimSpace(c.Query("workspaceId"))
	if workspace == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceId is required"})
		return "", "", false
	}
	return owner, workspace, true
}

func claimOwner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for knowledge claims"})
		return "", false
	}
	return owner, true
}

func claimQuery(c *gin.Context) (ClaimQuery, error) {
	limit := defaultClaimLimit
	var err error
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return ClaimQuery{}, err
		}
	}
	effectiveAt, err := parseClaimTime(c.Query("effectiveAt"))
	if err != nil {
		return ClaimQuery{}, err
	}
	observedBy, err := parseClaimTime(c.Query("observedBy"))
	if err != nil {
		return ClaimQuery{}, err
	}
	query := ClaimQuery{Limit: limit, EffectiveAt: effectiveAt, ObservedBy: observedBy}
	for _, status := range strings.Split(c.Query("verification"), ",") {
		if status = strings.TrimSpace(status); status != "" {
			query.VerificationStatuses = append(query.VerificationStatuses, VerificationStatus(status))
		}
	}
	return query, nil
}

func parseClaimTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	value = value.UTC()
	return &value, nil
}

func decodeClaimJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxClaimRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "knowledge claim request is too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid knowledge claim request"})
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge claim request must contain one JSON object"})
		return false
	}
	return true
}

func respondClaim(c *gin.Context, value any, err error, status int) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge claim not found"})
	case errors.Is(err, ErrExists):
		c.JSON(http.StatusConflict, gin.H{"error": "knowledge claim already exists"})
	case errors.Is(err, ErrCorruptStorage):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "knowledge claim integrity check failed"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge claim request was rejected"})
	}
}
