package modelintelligence

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the Model Intelligence API (§10.19).
type Handler struct {
	svc *Service
}

// NewHandler builds a handler over a service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// DefaultHandler builds a handler over the default service.
func DefaultHandler() *Handler { return NewHandler(DefaultService()) }

// Service exposes the underlying service (for wiring into the background loop).
func (h *Handler) Service() *Service { return h.svc }

func (h *Handler) Overview(c *gin.Context) { c.JSON(http.StatusOK, h.svc.Overview()) }

func (h *Handler) Profiles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"profiles": h.svc.Profiles()})
}

func (h *Handler) Profile(c *gin.Context) {
	prof, ok := h.svc.Profile(c.Param("providerId"), c.Param("modelId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	c.JSON(http.StatusOK, prof)
}

func (h *Handler) Benchmark(c *gin.Context) {
	res, err := h.svc.Benchmark(c.Request.Context(), c.Param("providerId"), c.Param("modelId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Benchmarks returns the profiles that have been benchmarked (truthful: only
// those with a real benchmark timestamp).
func (h *Handler) Benchmarks(c *gin.Context) {
	var out []ModelProfile
	for _, p := range h.svc.Profiles() {
		if p.LastBenchmarkedAt != nil {
			out = append(out, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"benchmarks": out})
}

func (h *Handler) Telemetry(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"telemetry": h.svc.Telemetry()})
}

func (h *Handler) Calibration(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Calibration())
}

func (h *Handler) LaneWinners(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"laneWinners": h.svc.LaneWinners()})
}

func (h *Handler) Cache(c *gin.Context) {
	hits, misses := h.svc.Cache().Stats()
	c.JSON(http.StatusOK, gin.H{"records": h.svc.Cache().Records(), "hits": hits, "misses": misses})
}

func (h *Handler) DeleteCache(c *gin.Context) {
	if !h.svc.Cache().Delete(c.Param("id")) {
		c.JSON(http.StatusNotFound, gin.H{"error": "cache record not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": c.Param("id")})
}

func (h *Handler) TokenBudgets(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.TokenBudgetDefaults())
}

func (h *Handler) UpdateTokenBudgets(c *gin.Context) {
	var b OperationBudget
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.svc.SetTokenBudgetDefaults(b)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}
