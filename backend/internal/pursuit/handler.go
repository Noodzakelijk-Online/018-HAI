package pursuit

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/rbac"
	"net/http"
	"strconv"
	"strings"

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

// RequireAuthenticatedOwner protects the personal pursuit API boundary. The
// service still supports ownerless calls for controlled in-process workers,
// but browser and API traffic must carry a verified IDP principal before it
// can read or mutate a person's pursuits.
func RequireAuthenticatedOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		if pursuitOwner(c) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for pursuit access"})
			return
		}
		c.Next()
	}
}

func (h *Handler) Create(c *gin.Context) {
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.OwnerIdentity = verifiedActor(c, "")
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
	records, err := h.service.ListForOwner(pursuitOwner(c), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (h *Handler) Dashboard(c *gin.Context) {
	record, err := h.service.DashboardForOwner(pursuitOwner(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Brief(c *gin.Context) {
	record, err := h.service.BriefForOwner(pursuitOwner(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Decisions(c *gin.Context) {
	decisions, err := h.service.DecisionsForOwner(pursuitOwner(c))
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
	record, err := h.service.DetailForOwner(pursuitOwner(c), id)
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
	if !h.ensurePursuitVisible(c, id) {
		return
	}
	uri := c.Query("uri")
	record, err := h.service.ResolveEvidenceForOwner(pursuitOwner(c), id, uri)
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
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.UpdateForOwner(pursuitOwner(c), id, request)
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
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request struct {
		Archived *bool  `json:"archived"`
		Actor    string `json:"actor,omitempty"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if request.Archived == nil || !*request.Archived {
		c.JSON(http.StatusBadRequest, gin.H{"error": "archive requests must set archived=true; use the explicit reopen action to reactivate a pursuit"})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	record, err := h.service.ArchiveForOwner(pursuitOwner(c), id, true, request.Actor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Reopen(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request struct {
		Note string `json:"note,omitempty"`
	}
	_ = c.ShouldBindJSON(&request)
	record, err := h.service.ReopenForOwner(pursuitOwner(c), id, verifiedActor(c, "operator"), request.Note)
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
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request LinkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.OwnerIdentity = pursuitOwner(c)
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
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	linkID, err := uuid.Parse(c.Param("linkId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid link id"})
		return
	}
	if err := h.service.DeleteLinkForOwner(pursuitOwner(c), id, linkID, verifiedActor(c, "operator")); err != nil {
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
	request.OwnerIdentity = pursuitOwner(c)
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
	request.OwnerIdentity = pursuitOwner(c)
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
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.OwnerIdentity = pursuitOwner(c)
	request.Actor = verifiedActor(c, "operator")
	_, err := h.service.IntakeForOwner(pursuitOwner(c), id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusCreated)
}

func (h *Handler) Plan(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request PlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if detail, err := h.service.DetailForOwner(pursuitOwner(c), id); err != nil || detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return
	} else if isPursuitCandidate(detail.Pursuit) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pursuit candidate acceptance requires the explicit approval action"})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	_, err := h.service.PlanForOwner(pursuitOwner(c), id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusCreated)
}

// AcceptCandidate is deliberately separate from generic planning. Accepting an
// auto-created pursuit candidate is an auditable approval decision, so its
// route requires approval capability before it may create or unlock work.
func (h *Handler) AcceptCandidate(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !pursuitApprovalAllowed(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "approval permission is required to accept a pursuit candidate"})
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request PlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	detail, err := h.service.DetailForOwner(pursuitOwner(c), id)
	if err != nil || detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return
	}
	if !isPursuitCandidate(detail.Pursuit) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only an unaccepted pursuit candidate can use the candidate acceptance action"})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	if _, err := h.service.AcceptCandidateForOwner(pursuitOwner(c), id, request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusCreated)
}

func (h *Handler) ResolveDecision(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	// Decision resolution can approve a next action, mark verified completion,
	// or create a governed recovery workflow. Keep the approval boundary here
	// as well as in route registration so alternate Gin wiring cannot weaken it.
	if !pursuitApprovalAllowed(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "approval permission is required to resolve a pursuit decision"})
		return
	}
	var request DecisionResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	_, err := h.service.ResolveDecisionForOwner(pursuitOwner(c), id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusOK)
}

func (h *Handler) RefreshSummary(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request struct {
		Actor string `json:"actor,omitempty"`
	}
	_ = c.ShouldBindJSON(&request)
	request.Actor = verifiedActor(c, "system")
	_, err := h.service.RefreshSummaryForOwner(pursuitOwner(c), id, request.Actor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusOK)
}

func (h *Handler) Review(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	if !h.ensurePursuitMutable(c, id) {
		return
	}
	var request ReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedActor(c, "operator")
	_, err := h.service.ReviewForOwner(pursuitOwner(c), id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedDetail(c, id, http.StatusOK)
}

func (h *Handler) Activity(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	records, err := h.service.ActivityForOwner(pursuitOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (h *Handler) NextActions(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.DetailForOwner(pursuitOwner(c), id)
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
	record, err := h.service.DetailForOwner(pursuitOwner(c), id)
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
	if !h.ensurePursuitVisible(c, id) {
		return
	}
	record, err := h.service.ApprovalsForOwner(pursuitOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) DelegationPackage(c *gin.Context) {
	id, ok := parsePursuitID(c)
	if !ok {
		return
	}
	record, err := h.service.DelegationPackageForOwner(pursuitOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
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

func (h *Handler) ensurePursuitVisible(c *gin.Context, id uuid.UUID) bool {
	if _, err := h.service.DetailForOwner(pursuitOwner(c), id); err != nil {
		// Keep an inaccessible record indistinguishable from one that does not
		// exist so UUID probing does not disclose another user's work.
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return false
	}
	return true
}

func (h *Handler) ensurePursuitMutable(c *gin.Context, id uuid.UUID) bool {
	record, err := h.service.DetailForOwner(pursuitOwner(c), id)
	if err != nil || !pursuitMutableBy(record.Pursuit, pursuitOwner(c)) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return false
	}
	return true
}

func (h *Handler) respondScopedDetail(c *gin.Context, id uuid.UUID, status int) {
	record, err := h.service.DetailForOwner(pursuitOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pursuit not found"})
		return
	}
	c.JSON(status, record)
}

func pursuitOwner(c *gin.Context) string {
	return verifiedActor(c, "")
}

func pursuitApprovalAllowed(c *gin.Context) bool {
	value, _ := c.Get(identity.ContextRoleKey)
	role, _ := value.(string)
	role = strings.ToLower(strings.TrimSpace(role))
	return rbac.Can(rbac.Role(role), rbac.PermApprove)
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
