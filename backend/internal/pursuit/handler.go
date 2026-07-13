package pursuit

import (
	"automation-hub-backend/internal/identity"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func DefaultHandler() *Handler {
	return NewHandler(DefaultService())
}

func (h *Handler) Create(c *gin.Context) {
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Create(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func (h *Handler) List(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	records, err := h.service.List(includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (h *Handler) Dashboard(c *gin.Context) {
	record, err := h.service.Dashboard()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Brief(c *gin.Context) {
	record, err := h.service.Brief()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Decisions(c *gin.Context) {
	decisions, err := h.service.Decisions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, decisions)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.Detail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) ResolveEvidence(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	uri := c.Query("uri")
	record, err := h.service.ResolveEvidence(id, uri)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Update(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Archive(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request struct {
		Archived bool   `json:"archived"`
		Actor    string `json:"actor,omitempty"`
	}
	_ = c.ShouldBindJSON(&request)
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Archive(id, request.Archived, request.Actor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Link(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request LinkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Link(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func (h *Handler) DeleteLink(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	linkID, err := uuid.Parse(c.Param("linkId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid link id"})
		return
	}
	if err := h.service.DeleteLink(id, linkID, verifiedActor(c, "operator")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Match(c *gin.Context) {
	var request MatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Match(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RouteIntake(c *gin.Context) {
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	result, err := h.service.RouteIntake(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) Intake(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Intake(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func (h *Handler) Plan(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request PlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Plan(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func (h *Handler) ResolveDecision(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request DecisionResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.ResolveDecision(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) RefreshSummary(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request struct {
		Actor string `json:"actor,omitempty"`
	}
	_ = c.ShouldBindJSON(&request)
	request.Actor = verifiedActor(c, "system")
	record, err := h.service.RefreshSummary(id, request.Actor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Review(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	var request ReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.Review(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Activity(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	records, err := h.service.Activity(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (h *Handler) NextActions(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.Detail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record.NextActions)
}

func (h *Handler) Blockers(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.Detail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record.Blockers)
}

func (h *Handler) Approvals(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.Approvals(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func parsePursuitID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pursuit id"})
		return uuid.UUID{}, false
	}
	return id, true
}

// verifiedActor deliberately ignores client-provided actor labels. When HAI is
// running behind its local IDP gateway, the backend has independently verified
// the signed session token and stores its subject in the Gin context. Local
// development without that identity path is recorded honestly as an operator,
// never as a user-supplied name such as "Robert".
func verifiedActor(c *gin.Context, fallback string) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok && subject != "" {
			return subject
		}
	}
	return fallback
}
