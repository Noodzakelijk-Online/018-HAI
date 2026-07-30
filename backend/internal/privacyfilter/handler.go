package privacyfilter

import (
	"automation-hub-backend/internal/identity"
	"net/http"
	"strings"

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
	ownerIdentity, ok := verifiedOwner(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "verified owner identity is required"})
		return
	}
	var req scanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}
	rec := h.svc.ScanForOwner(ownerIdentity, req.Content, req.SourceID, req.OperationID, req.MaxPreview)
	c.JSON(http.StatusOK, rec)
}

func (h *Handler) Scans(c *gin.Context) {
	ownerIdentity, ok := verifiedOwner(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "verified owner identity is required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"scans": h.svc.RecordsForOwner(ownerIdentity)})
}

func (h *Handler) ScanByID(c *gin.Context) {
	ownerIdentity, ok := verifiedOwner(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "verified owner identity is required"})
		return
	}
	rec, ok := h.svc.RecordForOwner(ownerIdentity, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func verifiedOwner(c *gin.Context) (string, bool) {
	value, ok := c.Get(identity.ContextSubjectKey)
	if !ok {
		return "", false
	}
	ownerIdentity, ok := value.(string)
	if !ok {
		return "", false
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	return ownerIdentity, ownerIdentity != ""
}
