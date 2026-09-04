package hostruntime

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxCompletionRequestBytes = maxResultBytes*2 + 2048

type Config struct {
	Enabled  bool
	Token    string
	WorkerID string
}

func DefaultConfig() Config {
	return Config{
		Enabled:  strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_HOST_RUNTIME_BRIDGE_ENABLED")), "true"),
		Token:    strings.TrimSpace(os.Getenv("HAI_HOST_RUNTIME_BRIDGE_TOKEN")),
		WorkerID: strings.TrimSpace(os.Getenv("HAI_HOST_RUNTIME_BRIDGE_WORKER_ID")),
	}
}

type Handler struct {
	service  *Service
	config   Config
	enabled  bool
	workerID string
}

func NewHandler(service *Service, config Config) *Handler {
	workerID := strings.TrimSpace(config.WorkerID)
	if workerID == "" {
		workerID = "windows-dsh"
	}
	return &Handler{
		service:  service,
		config:   config,
		enabled:  config.Enabled && len(strings.TrimSpace(config.Token)) >= 32,
		workerID: workerID,
	}
}

func (h *Handler) RegisterRoutes(routes gin.IRoutes) {
	routes.POST("/leases", h.Lease)
	routes.POST("/leases/:id/confirm", h.Confirm)
	routes.POST("/leases/:id/complete", h.Complete)
}

func (h *Handler) Lease(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	lease, err := h.service.Lease(h.workerID, "deepseek-harness")
	if errors.Is(err, ErrEmergencyStopped) {
		// Keep the local worker alive and polling, but do not hand it any new
		// work until the operator clears the stop.
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host runtime lease is unavailable"})
		return
	}
	if lease == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, lease)
}

type completionRequest struct {
	LeaseToken string `json:"leaseToken"`
	ExitCode   int    `json:"exitCode"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

type confirmRequest struct {
	LeaseToken string `json:"leaseToken"`
}

func (h *Handler) Confirm(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host runtime job id is invalid"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request confirmRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.LeaseToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host runtime confirmation is invalid"})
		return
	}
	if err := h.service.ConfirmLease(h.workerID, id, request.LeaseToken); err != nil {
		switch {
		case errors.Is(err, ErrEmergencyStopped):
			c.JSON(http.StatusLocked, gin.H{"error": "host runtime execution is blocked by emergency stop"})
		case errors.Is(err, ErrStaleLease):
			c.JSON(http.StatusConflict, gin.H{"error": "host runtime lease is no longer valid"})
		default:
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host runtime confirmation is unavailable"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Complete(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host runtime job id is invalid"})
		return
	}
	if c.Request.ContentLength > maxCompletionRequestBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "host runtime completion is too large"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCompletionRequestBytes)
	var request completionRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.LeaseToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host runtime completion is invalid"})
		return
	}
	job, err := h.service.Complete(h.workerID, id, request.LeaseToken, Completion{
		ExitCode: request.ExitCode,
		Output:   request.Output,
		Error:    request.Error,
	})
	if errors.Is(err, ErrStaleLease) {
		c.JSON(http.StatusConflict, gin.H{"error": "host runtime lease is no longer valid"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host runtime completion was rejected"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *Handler) authorized(c *gin.Context) bool {
	if h == nil || h.service == nil || !h.enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host runtime bridge is disabled"})
		return false
	}
	provided := bearerToken(c.GetHeader("Authorization"))
	expected := strings.TrimSpace(h.config.Token)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "host runtime bridge token is required"})
		return false
	}
	return true
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) <= 7 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}
