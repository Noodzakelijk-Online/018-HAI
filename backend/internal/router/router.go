package router

import (
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

func Initialize() error {
	// initialize Router
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		return err
	}
	router.Use(securityHeadersMiddleware())
	router.Use(rateLimitMiddleware(ratelimit.New(config.AppConfig.RateLimitPerMinute, time.Minute)))
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
