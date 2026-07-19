package wasiexec

import (
	"automation-hub-backend/internal/identity"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type Handler struct{ service *service }

func NewHandler(s *service) *Handler      { return &Handler{service: s} }
func (h *Handler) Status(c *gin.Context)  { c.JSON(200, h.service.Status()) }
func (h *Handler) Modules(c *gin.Context) { c.JSON(200, gin.H{"modules": h.service.Modules()}) }
func (h *Handler) Run(c *gin.Context) {
	r, e := h.service.Run(c.Request.Context(), owner(c), c.Param("id"))
	if errors.Is(e, ErrNotConfigured) {
		c.JSON(http.StatusConflict, gin.H{"error": e.Error(), "status": h.service.Status()})
		return
	}
	if errors.Is(e, ErrUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": e.Error(), "run": r})
		return
	}
	if e != nil {
		c.JSON(400, gin.H{"error": e.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}
func (h *Handler) Runs(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	r, e := h.service.Runs(owner(c), n)
	if e != nil {
		c.JSON(500, gin.H{"error": "could not read WASI runs"})
		return
	}
	c.JSON(200, gin.H{"runs": r})
}
func owner(c *gin.Context) string {
	v, _ := c.Get(identity.ContextSubjectKey)
	s, _ := v.(string)
	return s
}
