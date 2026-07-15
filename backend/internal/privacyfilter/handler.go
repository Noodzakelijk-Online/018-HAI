package privacyfilter

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the Privacy API (§10.19). It never returns raw sensitive
// content — only the bounded, redacted scan result.
type Handler struct {
	svc *Service
}

// NewHandler builds a handler over a service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// DefaultHandler builds a handler over the default service.
func DefaultHandler() *Handler { return NewHandler(DefaultService()) }

type scanRequest struct {
	Content     string `json:"content"`
	SourceID    string `json:"sourceId"`
	OperationID string `json:"operationId"`
	MaxPreview  int    `json:"maxPreview"`
}

func (h *Handler) ScanContent(c *gin.Context) {
	var req scanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}
	rec := h.svc.Scan(req.Content, req.SourceID, req.OperationID, req.MaxPreview)
	c.JSON(http.StatusOK, rec)
}

func (h *Handler) Scans(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"scans": h.svc.Records()})
}

func (h *Handler) ScanByID(c *gin.Context) {
	rec, ok := h.svc.Record(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}
