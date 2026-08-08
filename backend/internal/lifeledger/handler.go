package lifeledger

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/lifeontology"

	"github.com/gin-gonic/gin"
)

const maxRequestBytes = 512 << 10

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

type RouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
}

func RegisterRoutes(parent *gin.RouterGroup, handler *Handler, guards RouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil {
		return errors.New("life ledger route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil {
		return errors.New("life ledger routes require authentication, role, read, and write guards")
	}
	routes := parent.Group("/life-ledger")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.GET("/commitments", guards.Read, handler.ListCommitments)
		routes.GET("/commitments/:key", guards.Read, handler.GetCommitment)
		routes.GET("/commitments/:key/history", guards.Read, handler.CommitmentHistory)
		routes.POST("/commitments/:key/revisions", guards.Write, handler.RecordCommitment)
		routes.GET("/costs", guards.Read, handler.ListCosts)
		routes.POST("/costs", guards.Write, handler.RecordCost)
	}
	return nil
}

type commitmentBody struct {
	ExpectedRevision uint64              `json:"expectedRevision"`
	Domain           lifeontology.Domain `json:"domain"`
	Title            string              `json:"title"`
	Summary          string              `json:"summary,omitempty"`
	Status           CommitmentStatus    `json:"status"`
	Counterparty     string              `json:"counterparty,omitempty"`
	ProjectKey       string              `json:"projectKey,omitempty"`
	DueAt            *time.Time          `json:"dueAt,omitempty"`
	Verification     VerificationStatus  `json:"verification"`
	Evidence         []EvidenceReference `json:"evidence"`
	IdempotencyKey   string              `json:"idempotencyKey"`
	ObservedAt       time.Time           `json:"observedAt"`
}

func (h *Handler) RecordCommitment(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	var body commitmentBody
	if !decode(c, &body) {
		return
	}
	result, err := h.service.RecordCommitment(c.Request.Context(), RecordCommitmentRequest{
		OwnerIdentity: owner, CommitmentKey: c.Param("key"), ExpectedRevision: body.ExpectedRevision,
		Domain: body.Domain, Title: body.Title, Summary: body.Summary, Status: body.Status,
		Counterparty: body.Counterparty, ProjectKey: body.ProjectKey, DueAt: body.DueAt,
		Verification: body.Verification, Evidence: body.Evidence,
		IdempotencyKey: body.IdempotencyKey, ObservedAt: body.ObservedAt,
	})
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	respond(c, result, err, status)
}

type costBody struct {
	Domain         lifeontology.Domain `json:"domain"`
	Title          string              `json:"title"`
	Summary        string              `json:"summary,omitempty"`
	Kind           CostKind            `json:"kind"`
	AmountMinor    int64               `json:"amountMinor"`
	Currency       string              `json:"currency"`
	CommitmentKey  string              `json:"commitmentKey,omitempty"`
	ProjectKey     string              `json:"projectKey,omitempty"`
	Verification   VerificationStatus  `json:"verification"`
	Evidence       []EvidenceReference `json:"evidence"`
	IdempotencyKey string              `json:"idempotencyKey"`
	ObservedAt     time.Time           `json:"observedAt"`
}

func (h *Handler) RecordCost(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	var body costBody
	if !decode(c, &body) {
		return
	}
	result, err := h.service.RecordCost(c.Request.Context(), RecordCostRequest{
		OwnerIdentity: owner, Domain: body.Domain, Title: body.Title, Summary: body.Summary,
		Kind: body.Kind, AmountMinor: body.AmountMinor, Currency: body.Currency,
		CommitmentKey: body.CommitmentKey, ProjectKey: body.ProjectKey,
		Verification: body.Verification, Evidence: body.Evidence,
		IdempotencyKey: body.IdempotencyKey, ObservedAt: body.ObservedAt,
	})
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	respond(c, result, err, status)
}

func (h *Handler) GetCommitment(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	record, err := h.service.GetCommitment(c.Request.Context(), owner, c.Param("key"))
	respond(c, record, err, http.StatusOK)
}

func (h *Handler) ListCommitments(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	limit, ok := requestLimit(c)
	if !ok {
		return
	}
	records, err := h.service.ListCommitments(c.Request.Context(), owner, limit)
	respond(c, gin.H{"commitments": records}, err, http.StatusOK)
}

func (h *Handler) CommitmentHistory(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	limit, ok := requestLimit(c)
	if !ok {
		return
	}
	records, err := h.service.CommitmentHistory(c.Request.Context(), owner, c.Param("key"), limit)
	respond(c, gin.H{"revisions": records}, err, http.StatusOK)
}

func (h *Handler) ListCosts(c *gin.Context) {
	owner, ok := ledgerOwner(c)
	if !ok {
		return
	}
	limit, ok := requestLimit(c)
	if !ok {
		return
	}
	records, err := h.service.ListCosts(c.Request.Context(), owner, limit)
	respond(c, gin.H{"costs": records}, err, http.StatusOK)
}

func ledgerOwner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for life ledger access"})
		return "", false
	}
	return owner, true
}

func decode(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "life ledger request is too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid life ledger request"})
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "life ledger request must contain one JSON object"})
		return false
	}
	return true
}

func respond(c *gin.Context, value any, err error, status int) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "life ledger record not found"})
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrIdempotencyConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "life ledger state conflict"})
	case errors.Is(err, ErrCorruptRecord):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "life ledger integrity verification failed"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "life ledger request was rejected"})
	}
}

func requestLimit(c *gin.Context) (int, bool) {
	raw, exists := c.GetQuery("limit")
	if !exists {
		return 50, true
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer between 1 and 200"})
		return 0, false
	}
	return value, true
}
