package router

import (
	"net/http"

	"automation-hub-backend/internal/doctor"

	"github.com/gin-gonic/gin"
)

// readinessHandler exposes the configuration self-diagnostic as an HTTP
// readiness probe. It returns 200 when no check failed and 503 otherwise, so
// orchestrators can distinguish "process is up" (liveness, /healthz) from
// "process is correctly configured and ready to serve" (readiness, /readyz).
//
// The diagnose function is injected so the handler is testable without touching
// global configuration.
func readinessHandler(diagnose func() doctor.Report) gin.HandlerFunc {
	return func(c *gin.Context) {
		report := diagnose()
		ok, warn, fail := report.Counts()
		status := "ready"
		code := http.StatusOK
		if fail > 0 {
			status = "not_ready"
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{
			"status":  status,
			"service": "backend",
			"summary": gin.H{"ok": ok, "warn": warn, "fail": fail},
			"checks":  report.Checks,
		})
	}
}
