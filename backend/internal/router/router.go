package router

import (
	"fmt"
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/idempotency"
	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

func Initialize() error {
	// Startup config guard: refuse to serve with a broken configuration so a
	// misconfigured deployment fails fast and loudly rather than half-working.
	// Warnings (e.g. empty optional keys) do not block startup.
	if report := doctor.Diagnose(config.AppConfig); report.HasFailures() {
		_, _, fail := report.Counts()
		return fmt.Errorf("configuration not ready: %d failing check(s); run `backend doctor` for details", fail)
	}

	// initialize Router
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		return err
	}
	router.Use(securityHeadersMiddleware())
	router.Use(rateLimitMiddleware(ratelimit.New(config.AppConfig.RateLimitPerMinute, time.Minute)))
	router.Use(idempotencyMiddleware(idempotency.New(10 * time.Minute)))
	router.Use(localCaptureCORSMiddleware())

	// initialize routes
	err := initializeRoutes(router)
	if err != nil {
		return err
	}

	// run server
	port := config.AppConfig.ServerPort
	err = router.Run(port)
	if err != nil {
		return err
	}

	return nil
}
