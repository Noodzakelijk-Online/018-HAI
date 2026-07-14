package ambient

import (
	"automation-hub-backend/internal/identity"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Overview(c *gin.Context) {
	result, err := h.service.OverviewForOwner(verifiedOwner(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Scan(c *gin.Context) {
	ownerIdentity := verifiedOwner(c)
	if ownerIdentity == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required to run a personal ambient scan"})
		return
	}
	result, err := h.service.ScanForOwner(ownerIdentity, "manual")
	if err != nil {
		if errors.Is(err, ErrScanInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "scan": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) UpdateNeed(c *gin.Context) {
	ownerIdentity := verifiedOwner(c)
	if ownerIdentity == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "an authenticated owner session is required to update ambient planning preferences"})
		return
	}
	var request NeedUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.UpdateNeedForOwner(ownerIdentity, c.Param("key"), request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Accept(c *gin.Context) {
	h.resolve(c, true)
}

func (h *Handler) Dismiss(c *gin.Context) {
	h.resolve(c, false)
}

func (h *Handler) resolve(c *gin.Context, accept bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid opportunity id"})
		return
	}
	var request ResolutionRequest
	if bindErr := c.ShouldBindJSON(&request); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": bindErr.Error()})
		return
	}
	request.OwnerIdentity = verifiedOwner(c)
	request.Actor = verifiedActor(c, "operator")
	var result interface{}
	if accept {
		result, err = h.service.Accept(id, request)
	} else {
		result, err = h.service.Dismiss(id, request)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func verifiedOwner(c *gin.Context) string {
	return verifiedActor(c, "")
}

func verifiedActor(c *gin.Context, fallback string) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok {
			if subject = strings.TrimSpace(subject); subject != "" {
				return subject
			}
		}
	}
	return fallback
}
