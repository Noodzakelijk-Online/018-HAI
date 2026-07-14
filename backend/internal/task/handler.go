package task

import (
	"automation-hub-backend/internal/identity"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func DefaultHandler() (*Handler, error) {
	service, err := DefaultService()
	if err != nil {
		return nil, err
	}
	return NewHandler(service), nil
}

func (h *Handler) Plan(c *gin.Context) {
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if request.Request == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request is required"})
		return
	}
	ownerIdentity, ok := requireTaskOwner(c)
	if !ok {
		return
	}
	request.ExecuteAllowed = false
	request.OwnerIdentity = ownerIdentity
	request.HumanApproved = false
	request.ApprovalNote = ""
	plan, err := h.service.Plan(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *Handler) Run(c *gin.Context) {
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if request.Request == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request is required"})
		return
	}
	ownerIdentity, ok := requireTaskOwner(c)
	if !ok {
		return
	}
	request.ExecuteAllowed = true
	request.OwnerIdentity = ownerIdentity
	request.HumanApproved = false
	request.ApprovalNote = ""
	plan, err := h.service.Run(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func verifiedTaskOwner(c *gin.Context) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok {
			return strings.TrimSpace(subject)
		}
	}
	return ""
}

// requireTaskOwner keeps HTTP operator requests separate from in-process
// system workers. Task plans, review queues, and resolution decisions can
// contain source-derived private context, so they must never fall back to an
// ownerless/global view when the identity boundary is unavailable.
func requireTaskOwner(c *gin.Context) (string, bool) {
	ownerIdentity := verifiedTaskOwner(c)
	if ownerIdentity == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required for task operations"})
		return "", false
	}
	return ownerIdentity, true
}

func (h *Handler) Logs(c *gin.Context) {
	ownerIdentity, ok := requireTaskOwner(c)
	if !ok {
		return
	}
	scoped, ok := h.service.(OwnerScopedService)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "owner-scoped task history is unavailable"})
		return
	}
	c.JSON(http.StatusOK, scoped.LogsForOwner(ownerIdentity))
}

func (h *Handler) ReviewQueue(c *gin.Context) {
	ownerIdentity, ok := requireTaskOwner(c)
	if !ok {
		return
	}
	scoped, ok := h.service.(OwnerScopedService)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "owner-scoped task review is unavailable"})
		return
	}
	c.JSON(http.StatusOK, scoped.ReviewQueueForOwner(ownerIdentity))
}

func (h *Handler) ResolveReviewItem(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "review item id is required"})
		return
	}
	var decision ApprovalDecision
	if err := c.ShouldBindJSON(&decision); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ownerIdentity, ok := requireTaskOwner(c)
	if !ok {
		return
	}
	scoped, ok := h.service.(OwnerScopedService)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "owner-scoped task review is unavailable"})
		return
	}
	result, err := scoped.ResolveReviewItemForOwner(ownerIdentity, id, decision)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "review item not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}
