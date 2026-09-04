package router

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/idempotency"
	"automation-hub-backend/internal/metrics"
	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

const (
	maxRequestReadDuration   = 15 * time.Minute
	maxUploadWriteDuration   = 15 * time.Minute
	defaultIdleConnectionTTL = 60 * time.Second
)

func Initialize() error {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	return initializeWithContext(ctx)
}

func initializeWithContext(ctx context.Context) error {
	// Startup config guard: refuse to serve with a broken configuration so a
	// misconfigured deployment fails fast and loudly rather than half-working.
	// Warnings (e.g. empty optional keys) do not block startup.
	if report := doctor.Diagnose(config.AppConfig); report.HasFailures() {
		_, _, fail := report.Counts()
		return fmt.Errorf("configuration not ready: %d failing check(s); run `backend doctor` for details", fail)
	}

	// Keep production startup quiet and efficient. RUN_MODE already controls
	// whether real side effects are permitted, so use the same source of truth
	// for Gin instead of requiring a second deployment-only switch.
	if strings.EqualFold(strings.TrimSpace(config.AppConfig.RunMode), "production") {
		gin.SetMode(gin.ReleaseMode)
	}

	// initialize Router
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		return err
	}
	router.Use(securityHeadersMiddleware())
	router.Use(rateLimitMiddleware(newRateLimitEnforcer()))
	router.Use(idempotencyMiddleware(idempotency.New(10 * time.Minute)))
	router.Use(jsonRequestBodyLimitMiddleware(maxJSONAPIRequestBytes))
	router.Use(localCaptureCORSMiddleware())
	metricsExporter, err := metrics.NewFromEnv()
	if err != nil {
		return err
	}
	router.Use(metricsExporter.Middleware())

	// initialize routes
	err = initializeRoutesWithContext(router, ctx)
	if err != nil {
		return err
	}
	if metricsExporter.Enabled() {
		router.GET("/metrics", metricsExporter.RequireBearerToken(), gin.WrapH(metricsExporter.Handler()))
	}

	server := newHTTPServer(config.AppConfig.ServerPort, router)
	return serveWithContext(ctx, server, nil)
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       maxRequestReadDuration,
		IdleTimeout:       defaultIdleConnectionTTL,
		WriteTimeout:      maxUploadWriteDuration,
	}
}

// serveWithContext gives the API and the schedulers that derive from its
// lifecycle one termination signal. It supports a listener in tests and uses
// the configured server address in production.
func serveWithContext(ctx context.Context, server *http.Server, listener net.Listener) error {
	if server == nil {
		return errors.New("HTTP server is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	shutdownDone := make(chan struct{})
	defer close(shutdownDone)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("server shutdown: %v", err)
			}
		case <-shutdownDone:
		}
	}()

	var err error
	if listener != nil {
		err = server.Serve(listener)
	} else {
		err = server.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// newRateLimitEnforcer selects where rate-limit counters live. When REDIS_ADDR
// is set and reachable, counters are shared through Redis so the limit survives
// restarts and holds across multiple backend instances. Otherwise it falls back
// to the in-process limiter — correct for a single instance, but per-process and
// reset on restart. The fallback is deliberate: a misconfigured or briefly
// unreachable Redis at startup degrades the limiter rather than failing the boot.
func newRateLimitEnforcer() ratelimit.Enforcer {
	limit := config.AppConfig.RateLimitPerMinute
	window := time.Minute

	addr := strings.TrimSpace(config.AppConfig.RedisAddr)
	if addr == "" {
		return ratelimit.Memory(limit, window)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	redisLimiter, err := ratelimit.NewRedisLimiter(ctx, addr, limit, window)
	if err != nil {
		log.Printf("ratelimit: falling back to in-process limiter: %v", err)
		return ratelimit.Memory(limit, window)
	}
	log.Printf("ratelimit: using shared Redis store at %s", addr)
	return redisLimiter
}
