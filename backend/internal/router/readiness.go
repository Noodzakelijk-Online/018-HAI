package router

import (
	"context"
	"net/http"

	"automation-hub-backend/internal/doctor"

	"github.com/gin-gonic/gin"
)

// readinessHandler exposes the configuration self-diagnostic *and* live
// dependency probes as an HTTP readiness probe.
//
// The three states are distinct on purpose, because an operator needs to tell
// them apart:
//
//	ready     — configuration is sound and every dependency answered.
//	degraded  — serving, but something is missing or unused: no LLM provider,
//	            Kafka down (events dropped), a placeholder secret still in place.
//	not_ready — a critical dependency is unreachable. 503, so an orchestrator
//	            stops sending traffic to a process that cannot serve it.
//
// Only a critical failure returns 503. A degraded service is still a serving
// service, and reporting it as down would be its own kind of dishonesty.
//
// The diagnose function is injected so the handler is testable without touching
// global configuration or opening real sockets.
func readinessHandler(diagnose func(ctx context.Context) doctor.Report) gin.HandlerFunc {
	return func(c *gin.Context) {
		report := diagnose(c.Request.Context())
		ok, warn, fail := report.Counts()

		status := "ready"
		code := http.StatusOK
		switch {
		case fail > 0:
			status = "not_ready"
			code = http.StatusServiceUnavailable
		case warn > 0:
			status = "degraded"
		}

		c.JSON(code, gin.H{
			"status":  status,
			"service": "backend",
			"summary": gin.H{"ok": ok, "warn": warn, "fail": fail},
			"checks":  report.Checks,
		})
	}
}
