package phase2

import (
	"net/http"
	"strconv"
	"time"

	"automation-hub-backend/internal/background"
	"automation-hub-backend/internal/operations"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler serves the Phase 2 Background Operations API.
type Handler struct {
	m *Module
}

// NewHandler builds a handler over a module.
func NewHandler(m *Module) *Handler { return &Handler{m: m} }

// owner resolves the caller's operator identity from the verified JWT subject,
// falling back to the module's configured single operator.
func (h *Handler) owner(c *gin.Context) (string, string) {
	if sub, ok := c.Get("subject"); ok {
		if s, ok := sub.(string); ok && s != "" {
			return s, h.m.cfg.WorkspaceID
		}
	}
	return h.m.cfg.OwnerUserID, h.m.cfg.WorkspaceID
}

// ListOperations returns operations for the caller, optionally filtered.
func (h *Handler) ListOperations(c *gin.Context) {
	owner, workspace := h.owner(c)
	f := operations.Filter{OwnerUserID: owner, WorkspaceID: workspace}
	if s := c.Query("status"); s != "" {
		f.Status = operations.OperationStatus(s)
	}
	if r := c.Query("risk"); r != "" {
		f.RiskLevel = operations.RiskLevel(r)
	}
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			f.Limit = n
		}
	}
	ops, err := h.m.svc.List(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"operations": ops})
}

// Dashboard returns the Background Operations roll-up.
func (h *Handler) Dashboard(c *gin.Context) {
	owner, workspace := h.owner(c)
	d, err := h.m.svc.Dashboard(owner, workspace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

// GetOperation returns a single operation.
func (h *Handler) GetOperation(c *gin.Context) {
	owner, workspace := h.owner(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	op, err := h.m.svc.Get(owner, workspace, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, op)
}

// OperationEvents returns an operation's audit trail.
func (h *Handler) OperationEvents(c *gin.Context) {
	owner, workspace := h.owner(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if _, err := h.m.svc.Get(owner, workspace, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	events, err := h.m.svc.Events(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

// Approve records a human approval, moving an awaiting-approval operation to
// approved. It does not fake execution: Phase 2A has no real runtime for
// high-risk/external work, so an approved operation waits for a future runtime.
func (h *Handler) Approve(c *gin.Context) {
	owner, workspace := h.owner(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	op, err := h.m.svc.Get(owner, workspace, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	op.OwnerType = string(operations.OwnerRobert)
	approved, err := h.m.svc.Transition(*op, operations.StatusApproved, string(operations.OwnerRobert), owner, "approved by operator")
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, approved)
}

// RunOperation executes an operation via the local safe worker and verifies it.
// It refuses operations that are not safe-executable (returns 409), so no
// high-risk/external work is ever fake-completed.
func (h *Handler) RunOperation(c *gin.Context) {
	owner, workspace := h.owner(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	op, err := h.m.svc.Get(owner, workspace, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	outcome, err := background.ExecuteSafeOperation(c.Request.Context(), h.m.svc, h.m.broker, *op, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"operation": outcome.Operation, "verified": outcome.Verified, "failed": outcome.Failed})
}

// RunBackground triggers a single background pass and returns its report.
func (h *Handler) RunBackground(c *gin.Context) {
	rep, err := h.m.worker.RunOnce(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rep)
}

// ListFeeds returns the configured account feeds.
func (h *Handler) ListFeeds(c *gin.Context) {
	type feedView struct {
		Name         string `json:"name"`
		Provider     string `json:"provider"`
		AccountLabel string `json:"accountLabel"`
		SourceType   string `json:"sourceType"`
		Enabled      bool   `json:"enabled"`
	}
	views := make([]feedView, 0, len(h.m.readers))
	for _, r := range h.m.readers {
		f := r.Feed()
		views = append(views, feedView{f.Name, f.Provider, f.AccountLabel, string(f.SourceType), f.Enabled})
	}
	c.JSON(http.StatusOK, gin.H{"feeds": views})
}
