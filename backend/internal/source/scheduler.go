package source

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type Scheduler struct {
	service  Service
	interval time.Duration
	running  atomic.Bool
}

func NewScheduler(service Service, interval time.Duration) *Scheduler {
	if interval < 15*time.Second {
		interval = 10 * time.Minute
	}
	return &Scheduler{service: service, interval: interval}
}

// StartScheduler starts scheduled source syncing.
//
// It prefers the durable path: scans and per-source syncs become persisted jobs
// with backoff retry and crash recovery (see durable_scheduler.go). If the
// durable queue cannot be reached, the default is to leave the scheduler
// stopped and make the loss of crash recovery explicit in the logs. Operators
// may opt into the legacy in-process ticker only for local development. An
// explicit SOURCE_SCHEDULER_DURABLE=false remains a deliberate legacy choice.
func StartScheduler(ctx context.Context, service Service) {
	if !schedulerEnabled() {
		return
	}
	interval := schedulerInterval()
	if durableSchedulerEnabled() {
		if err := startDurableScheduler(ctx, service, interval); err != nil {
			if !legacyFallbackEnabled() {
				log.Printf("source scheduler: durable queue unavailable (%v); scheduler not started; set DURABLE_SCHEDULER_LEGACY_FALLBACK_ENABLED=true only for local development", err)
				return
			}
			log.Printf("source scheduler: durable queue unavailable (%v); using the explicitly enabled in-process fallback", err)
		} else {
			return
		}
	}
	scheduler := NewScheduler(service, interval)
	go scheduler.Start(ctx)
}

func legacyFallbackEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("DURABLE_SCHEDULER_LEGACY_FALLBACK_ENABLED")))
	return value == "true" || value == "1" || value == "yes"
}

func durableSchedulerEnabled() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("SOURCE_SCHEDULER_DURABLE"))) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	if schedulerRunOnStartup() {
		s.runOnce()
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce()
		}
	}
}

func (s *Scheduler) runOnce() {
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	defer s.running.Store(false)
	result, err := s.service.RunDueScheduledSyncs(time.Now().UTC())
	if err != nil {
		log.Printf("source scheduler failed: %v", err)
		return
	}
	if result.Due > 0 || result.Failed > 0 {
		log.Printf("source scheduler checked=%d due=%d completed=%d failed=%d", result.Checked, result.Due, result.Completed, result.Failed)
	}
}

func schedulerEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("SOURCE_SCHEDULER_ENABLED")))
	return value == "" || value == "true" || value == "1" || value == "yes"
}

func schedulerInterval() time.Duration {
	value := strings.TrimSpace(os.Getenv("SOURCE_SCHEDULER_INTERVAL_SECONDS"))
	if value == "" {
		return 10 * time.Minute
	}
	var seconds int64
	if _, err := fmt.Sscanf(value, "%d", &seconds); err != nil || seconds < 15 {
		return 10 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func schedulerRunOnStartup() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("SOURCE_SCHEDULER_RUN_ON_STARTUP")))
	return value == "true" || value == "1" || value == "yes"
}
