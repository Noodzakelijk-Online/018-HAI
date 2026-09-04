package autonomy

import (
	"automation-hub-backend/internal/apierror"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Overview(c *gin.Context) {
	result, err := h.service.Overview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "autonomy overview is unavailable")})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Stress(c *gin.Context) {
	run, results, err := h.service.RunStressSuite()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "autonomy stress suite could not be completed")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run, "results": results})
}
