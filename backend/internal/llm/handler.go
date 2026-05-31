package llm

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func DefaultHandler() (*Handler, error) {
	service, err := NewServiceFromEnv()
	if err != nil {
		return nil, err
	}
	return &Handler{service: service}, nil
}

func (h *Handler) Policy(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Policy())
}

func (h *Handler) Route(c *gin.Context) {
	var request RouteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	decision, err := h.service.Route(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, decision)
}

func (h *Handler) Generate(c *gin.Context) {
	var request GenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Generate(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Logs(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Logs())
}
