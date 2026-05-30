package task

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func DefaultHandler() (*Handler, error) {
	service, err := DefaultService()
	if err != nil {
		return nil, err
	}
	return &Handler{service: service}, nil
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
	request.ExecuteAllowed = true
	plan, err := h.service.Run(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *Handler) Logs(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Logs())
}

func (h *Handler) ReviewQueue(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ReviewQueue())
}
