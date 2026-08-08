package lifeontology

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

const maxLifeOntologyRequestBytes = 512 << 10

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

type RouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
	Govern             gin.HandlerFunc
}

func RegisterRoutes(parent *gin.RouterGroup, handler *Handler, guards RouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil {
		return errors.New("life ontology route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil || guards.Govern == nil {
		return errors.New("life ontology routes require authentication, recognized-role, and permission guards")
	}
	routes := parent.Group("/life-ontology")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.GET("/entities", guards.Read, handler.ListEntities)
		routes.POST("/entities", guards.Write, handler.RecordEntity)
		routes.GET("/entities/:id", guards.Read, handler.GetEntity)
		routes.GET("/relations", guards.Read, handler.ListRelations)
		routes.POST("/relations", guards.Write, handler.RecordRelation)
		routes.GET("/relations/:id", guards.Read, handler.GetRelation)
		routes.GET("/merge-proposals", guards.Read, handler.ListMergeProposals)
		routes.POST("/merge-proposals/:id/decisions", guards.Govern, handler.DecideContactMerge)
		routes.POST("/contact-candidates/:id/decisions", guards.Govern, handler.DecideContactCandidate)
		routes.GET("/contact-review-decisions", guards.Read, handler.ListContactReviewDecisions)
		routes.POST("/context/suggest", guards.Read, handler.SuggestContext)
	}
	return nil
}

type recordEntityBody struct {
	Type               EntityType         `json:"type"`
	Domain             Domain             `json:"domain"`
	Name               string             `json:"name"`
	Summary            string             `json:"summary,omitempty"`
	ExternalKeys       []ExternalKey      `json:"externalKeys,omitempty"`
	Attributes         map[string]string  `json:"attributes,omitempty"`
	Status             LifecycleStatus    `json:"status"`
	Priority           int                `json:"priority"`
	DueAt              *time.Time         `json:"dueAt,omitempty"`
	ValidFrom          time.Time          `json:"validFrom"`
	ValidUntil         *time.Time         `json:"validUntil,omitempty"`
	ObservedAt         time.Time          `json:"observedAt"`
	Confidence         float64            `json:"confidence"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Provenance         []Provenance       `json:"provenance"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
}

func (h *Handler) RecordEntity(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	var body recordEntityBody
	if !decodeLifeOntologyJSON(c, &body) {
		return
	}
	result, err := h.service.RecordEntity(c.Request.Context(), RecordEntityRequest{
		OwnerIdentity: owner, Type: body.Type, Domain: body.Domain, Name: body.Name,
		Summary: body.Summary, ExternalKeys: body.ExternalKeys, Attributes: body.Attributes,
		Status: body.Status, Priority: body.Priority, DueAt: body.DueAt, ValidFrom: body.ValidFrom,
		ValidUntil: body.ValidUntil, ObservedAt: body.ObservedAt, Confidence: body.Confidence,
		VerificationStatus: body.VerificationStatus, Provenance: body.Provenance,
		Sensitivity: body.Sensitivity, LocalOnly: body.LocalOnly,
	})
	respondLifeOntology(c, result, err, http.StatusCreated)
}

type recordRelationBody struct {
	Type               RelationType       `json:"type"`
	FromEntityID       string             `json:"fromEntityId"`
	ToEntityID         string             `json:"toEntityId"`
	Summary            string             `json:"summary,omitempty"`
	Attributes         map[string]string  `json:"attributes,omitempty"`
	ValidFrom          time.Time          `json:"validFrom"`
	ValidUntil         *time.Time         `json:"validUntil,omitempty"`
	ObservedAt         time.Time          `json:"observedAt"`
	Confidence         float64            `json:"confidence"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Provenance         []Provenance       `json:"provenance"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
}

func (h *Handler) RecordRelation(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	var body recordRelationBody
	if !decodeLifeOntologyJSON(c, &body) {
		return
	}
	result, err := h.service.RecordRelation(c.Request.Context(), RecordRelationRequest{
		OwnerIdentity: owner, Type: body.Type, FromEntityID: body.FromEntityID, ToEntityID: body.ToEntityID,
		Summary: body.Summary, Attributes: body.Attributes, ValidFrom: body.ValidFrom, ValidUntil: body.ValidUntil,
		ObservedAt: body.ObservedAt, Confidence: body.Confidence, VerificationStatus: body.VerificationStatus,
		Provenance: body.Provenance, Sensitivity: body.Sensitivity, LocalOnly: body.LocalOnly,
	})
	respondLifeOntology(c, result, err, http.StatusCreated)
}

func (h *Handler) GetEntity(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	result, err := h.service.GetEntity(c.Request.Context(), owner, c.Param("id"))
	respondLifeOntology(c, result, err, http.StatusOK)
}

func (h *Handler) GetRelation(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	result, err := h.service.GetRelation(c.Request.Context(), owner, c.Param("id"))
	respondLifeOntology(c, result, err, http.StatusOK)
}

func (h *Handler) ListEntities(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	query, err := entityQueryFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid life ontology entity query"})
		return
	}
	result, err := h.service.QueryEntities(c.Request.Context(), owner, query)
	respondLifeOntology(c, gin.H{"entities": result}, err, http.StatusOK)
}

func (h *Handler) ListRelations(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	query, err := relationQueryFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid life ontology relation query"})
		return
	}
	result, err := h.service.QueryRelations(c.Request.Context(), owner, query)
	respondLifeOntology(c, gin.H{"relations": result}, err, http.StatusOK)
}

func (h *Handler) ListMergeProposals(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merge proposal limit"})
		return
	}
	result, err := h.service.ListMergeProposals(c.Request.Context(), owner, limit)
	respondLifeOntology(c, gin.H{"proposals": result}, err, http.StatusOK)
}

type contactReviewDecisionBody struct {
	Action           ContactReviewAction `json:"action"`
	CanonicalName    string              `json:"canonicalName,omitempty"`
	CanonicalSummary string              `json:"canonicalSummary,omitempty"`
	Reason           string              `json:"reason"`
	IdempotencyKey   string              `json:"idempotencyKey"`
}

func (h *Handler) DecideContactCandidate(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	var body contactReviewDecisionBody
	if !decodeLifeOntologyJSON(c, &body) {
		return
	}
	result, err := h.service.DecideContactCandidate(c.Request.Context(), DecideContactCandidateRequest{
		OwnerIdentity: owner, CandidateID: c.Param("id"), Action: body.Action,
		CanonicalName: body.CanonicalName, CanonicalSummary: body.CanonicalSummary,
		Reason: body.Reason, IdempotencyKey: body.IdempotencyKey,
	})
	respondLifeOntology(c, result, err, http.StatusCreated)
}

func (h *Handler) DecideContactMerge(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	var body contactReviewDecisionBody
	if !decodeLifeOntologyJSON(c, &body) {
		return
	}
	result, err := h.service.DecideContactMerge(c.Request.Context(), DecideContactMergeRequest{
		OwnerIdentity: owner, ProposalID: c.Param("id"), Action: body.Action,
		CanonicalName: body.CanonicalName, CanonicalSummary: body.CanonicalSummary,
		Reason: body.Reason, IdempotencyKey: body.IdempotencyKey,
	})
	respondLifeOntology(c, result, err, http.StatusCreated)
}

func (h *Handler) ListContactReviewDecisions(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contact review decision limit"})
		return
	}
	result, err := h.service.ListContactReviewDecisions(c.Request.Context(), owner, limit)
	respondLifeOntology(c, gin.H{"decisions": result}, err, http.StatusOK)
}

type contextSuggestionBody struct {
	FocusEntityID  string       `json:"focusEntityId,omitempty"`
	Domains        []Domain     `json:"domains,omitempty"`
	Types          []EntityType `json:"types,omitempty"`
	AsOf           time.Time    `json:"asOf"`
	AllowLocalOnly bool         `json:"allowLocalOnly"`
	Limit          int          `json:"limit,omitempty"`
}

func (h *Handler) SuggestContext(c *gin.Context) {
	owner, ok := lifeOntologyOwner(c)
	if !ok {
		return
	}
	var body contextSuggestionBody
	if !decodeLifeOntologyJSON(c, &body) {
		return
	}
	result, err := h.service.SuggestNextContext(c.Request.Context(), ContextSuggestionRequest{
		OwnerIdentity: owner, FocusEntityID: body.FocusEntityID, Domains: body.Domains,
		Types: body.Types, AsOf: body.AsOf, AllowLocalOnly: body.AllowLocalOnly, Limit: body.Limit,
	})
	respondLifeOntology(c, result, err, http.StatusOK)
}

func lifeOntologyOwner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for life ontology access"})
		return "", false
	}
	return owner, true
}

func decodeLifeOntologyJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLifeOntologyRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "life ontology request is too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid life ontology request"})
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "life ontology request must contain one JSON object"})
		return false
	}
	return true
}

func respondLifeOntology(c *gin.Context, value any, err error, status int) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "life ontology record not found"})
	case errors.Is(err, ErrExists):
		c.JSON(http.StatusConflict, gin.H{"error": "life ontology record already exists"})
	case errors.Is(err, ErrContactReviewConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "contact review subject already has a decision"})
	case errors.Is(err, ErrCorruptStorage):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "life ontology storage integrity check failed"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "life ontology request was rejected"})
	}
}

func entityQueryFromRequest(c *gin.Context) (EntityQuery, error) {
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		return EntityQuery{}, err
	}
	asOf, err := queryTime(c.Query("asOf"))
	if err != nil {
		return EntityQuery{}, err
	}
	observed, err := queryTime(c.Query("observedBy"))
	if err != nil {
		return EntityQuery{}, err
	}
	local, err := queryBool(c.Query("allowLocalOnly"))
	if err != nil {
		return EntityQuery{}, err
	}
	query := EntityQuery{AsOf: asOf, ObservedBy: observed, AllowLocalOnly: local, Limit: limit}
	for _, value := range queryValues(c.Query("domains")) {
		query.Domains = append(query.Domains, Domain(value))
	}
	for _, value := range queryValues(c.Query("types")) {
		query.Types = append(query.Types, EntityType(value))
	}
	for _, value := range queryValues(c.Query("statuses")) {
		query.Statuses = append(query.Statuses, LifecycleStatus(value))
	}
	for _, value := range queryValues(c.Query("verification")) {
		query.VerificationStatuses = append(query.VerificationStatuses, VerificationStatus(value))
	}
	return query, nil
}

func relationQueryFromRequest(c *gin.Context) (RelationQuery, error) {
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		return RelationQuery{}, err
	}
	asOf, err := queryTime(c.Query("asOf"))
	if err != nil {
		return RelationQuery{}, err
	}
	observed, err := queryTime(c.Query("observedBy"))
	if err != nil {
		return RelationQuery{}, err
	}
	local, err := queryBool(c.Query("allowLocalOnly"))
	if err != nil {
		return RelationQuery{}, err
	}
	query := RelationQuery{FromEntityID: c.Query("fromEntityId"), ToEntityID: c.Query("toEntityId"), AsOf: asOf, ObservedBy: observed, AllowLocalOnly: local, Limit: limit}
	for _, value := range queryValues(c.Query("types")) {
		query.Types = append(query.Types, RelationType(value))
	}
	return query, nil
}

func queryLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 50, nil
	}
	return strconv.Atoi(raw)
}

func queryTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func queryBool(raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func queryValues(raw string) []string {
	result := []string{}
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
