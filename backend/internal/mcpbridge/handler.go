package mcpbridge

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const bridgeTokenHeader = "X-HAI-MCP-Bridge-Token"

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Status is served through the normal authenticated HAI API so an owner can
// see whether the optional MCP bridge is configured without seeing either
// token or opening the bridge's data endpoints.
func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Overview(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	result, err := h.service.Overview()
	if h.respondError(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Actionable(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	limit := 5
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 8"})
			return
		}
		limit = parsed
	}
	result, err := h.service.Actionable(limit)
	if h.respondError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": result,
		"scope": "Read-only, bounded operational summaries for one configured owner. No task, approval, execution, source-content, memory, or policy action is exposed.",
	})
}

func (h *Handler) GitHubRepositories(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	limit, ok := bridgeLimit(c)
	if !ok {
		return
	}
	result, err := h.service.GitHubRepositories(limit)
	if h.respondError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"repositories": result,
		"scope": "Read-only, owner-scoped GitHub repository configuration and sync freshness. No issue, pull request, commit, file, source URI, raw source item, credential, or repository action is exposed.",
	})
}

func (h *Handler) ModelMaintenanceReadiness(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	limit, ok := bridgeLimit(c)
	if !ok {
		return
	}
	result, err := h.service.ModelMaintenanceReadiness(limit)
	if h.respondError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"models": result,
		"scope":  "Read-only latest per-model daily maintenance status for one configured owner. No endpoint, digest, token, prompt, completion, token count, quota, cost, route, refresh, or generation control is exposed.",
	})
}

func bridgeLimit(c *gin.Context) (int, bool) {
	limit := 5
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 8"})
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func (h *Handler) authorized(c *gin.Context) bool {
	if h.service.Authorize(c.GetHeader(bridgeTokenHeader)) {
		return true
	}
	// A disabled local service should not advertise an endpoint. A bad token is
	// intentionally indistinguishable from a disabled service.
	c.JSON(http.StatusNotFound, gin.H{"error": "local MCP bridge unavailable"})
	return false
}

func (h *Handler) respondError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnavailable) {
		c.JSON(http.StatusNotFound, gin.H{"error": "local MCP bridge unavailable"})
		return true
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": "local MCP bridge could not read the workflow summary"})
	return true
}
