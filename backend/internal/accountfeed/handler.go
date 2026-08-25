package accountfeed

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler serves the Account Feeds API (§10.19). Feeds are read-only bridges;
// no route ever fakes provider access or connected status.
type Handler struct {
	reg   *Registry
	perms *PermissionRegistry
	space string
}

// NewHandler builds a handler over a registry.
func NewHandler(reg *Registry, _ string, workspaceID string) *Handler {
	return &Handler{reg: reg, perms: NewPermissionRegistry(), space: workspaceID}
}

func (h *Handler) ownerID(c *gin.Context) string {
	if sub, ok := c.Get("subject"); ok {
		if s, ok := sub.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// List returns feed health for all registered feeds.
func (h *Handler) List(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"feeds": h.reg.HealthForOwner(owner)})
}

// Bridges returns the provider bridge contracts with truthful connection status.
func (h *Handler) Bridges(c *gin.Context) {
	bridges := Bridges()
	type view struct {
		BridgeContract
		Status ConnectionStatus `json:"connectionStatus"`
	}
	out := make([]view, 0, len(bridges))
	for _, b := range bridges {
		out = append(out, view{BridgeContract: b, Status: b.ConnectionStatus()})
	}
	c.JSON(http.StatusOK, gin.H{"bridges": out})
}

// Permissions returns the account permission registry.
func (h *Handler) Permissions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"permissions": h.perms.Permissions()})
}

type registerRequest struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	AccountLabel  string `json:"accountLabel"`
	SourceType    string `json:"sourceType"`
	Path          string `json:"path"`
	URL           string `json:"url"`
	ProjectKey    string `json:"projectKey"`
	OperationType string `json:"operationType"`
	Enabled       bool   `json:"enabled"`
}

// Create registers a new feed.
func (h *Handler) Create(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	feed := Feed{
		Name:          req.Name,
		Provider:      req.Provider,
		AccountLabel:  req.AccountLabel,
		SourceType:    SourceType(req.SourceType),
		Path:          req.Path,
		URL:           req.URL,
		ProjectKey:    req.ProjectKey,
		OperationType: req.OperationType,
		OwnerUserID:   owner,
		WorkspaceID:   h.space,
		Enabled:       req.Enabled,
	}
	created, err := h.reg.Register(feed)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

// Get returns a single feed.
func (h *Handler) Get(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	feed, ok := h.reg.GetForOwner(id, owner)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	c.JSON(http.StatusOK, feed)
}

type patchRequest struct {
	Enabled       *bool   `json:"enabled"`
	Name          *string `json:"name"`
	OperationType *string `json:"operationType"`
}

// Patch updates mutable feed fields.
func (h *Handler) Patch(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	feed, ok := h.reg.PatchForOwner(id, owner, req.Enabled, req.Name, req.OperationType)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	c.JSON(http.StatusOK, feed)
}

// Sync syncs a single feed.
func (h *Handler) Sync(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rep, ok := h.reg.SyncForOwner(c.Request.Context(), id, owner)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	c.JSON(http.StatusOK, rep)
}

// SyncDue syncs all enabled feeds.
func (h *Handler) SyncDue(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"reports": h.reg.SyncDueForOwner(c.Request.Context(), owner)})
}

// Audit returns a feed's audit trail.
func (h *Handler) Audit(c *gin.Context) {
	owner, ok := h.requireOwner(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if _, ok := h.reg.GetForOwner(id, owner); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	audit, ok := h.reg.AuditForOwner(id, owner)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"audit": audit})
}

func (h *Handler) requireOwner(c *gin.Context) (string, bool) {
	owner := h.ownerID(c)
	if owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated owner identity is required"})
		return "", false
	}
	return owner, true
}
