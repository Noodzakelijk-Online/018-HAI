package wasiexec

import (
	"errors"
	"net/http"
	"strconv"

	"automation-hub-backend/internal/identity"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *service }

func NewHandler(s *service) *Handler      { return &Handler{service: s} }
func (h *Handler) Status(c *gin.Context)  { c.JSON(http.StatusOK, h.service.Status()) }
func (h *Handler) Modules(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"modules": h.service.Modules()}) }

func (h *Handler) Run(c *gin.Context) {
	var request RunRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "taskId, projectKey, approvalSourceId, and approvalBindingDigest are required",
		})
		return
	}
	request.ModuleID = c.Param("id")
	run, err := h.service.Run(c.Request.Context(), owner(c), request)
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "status": h.service.Status()})
	case errors.Is(err, ErrEmergencyStopActive):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "run": run})
	case errors.Is(err, ErrAuthorizationUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "run": run})
	case errors.Is(err, ErrNotAuthorized):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "run": run})
	case errors.Is(err, ErrUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "run": run})
	case err != nil:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusCreated, run)
	}
}

func (h *Handler) Runs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	runs, err := h.service.Runs(owner(c), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read WASI runs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func owner(c *gin.Context) string {
	value, _ := c.Get(identity.ContextSubjectKey)
	ownerIdentity, _ := value.(string)
	return ownerIdentity
}
