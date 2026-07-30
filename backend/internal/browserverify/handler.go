package browserverify

import (
	"automation-hub-backend/internal/identity"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct{ service *service }

func NewHandler(service *service) *Handler { return &Handler{service: service} }
func (h *Handler) Status(c *gin.Context)   { c.JSON(http.StatusOK, h.service.Status()) }
func (h *Handler) Profiles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"profiles": h.service.Profiles()})
}
func (h *Handler) Run(c *gin.Context) {
	run, err := h.service.Run(c.Request.Context(), owner(c), c.Param("id"))
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "status": h.service.Status()})
		return
	}
	if errors.Is(err, ErrUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "run": run})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, run)
}
func (h *Handler) Runs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	runs, err := h.service.Runs(owner(c), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read browser verification runs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}
func owner(c *gin.Context) string {
	value, _ := c.Get(identity.ContextSubjectKey)
	owner, _ := value.(string)
	return strings.TrimSpace(owner)
}
