package ambient

import (
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type Scheduler struct {
	service Service
	running atomic.Bool
}

// StartScheduler starts ambient scanning.
//
// It prefers the durable path (persisted, retried, crash-recovered — see
// durable_scheduler.go) and falls back to the legacy in-process ticker, saying
// so, if the durable queue cannot be reached.
func StartScheduler(ctx context.Context, service Service) {
	policy := policyFromEnv()
	if !policy.SchedulerEnabled {
		return
	}
	interval := time.Duration(policy.ScanIntervalSeconds) * time.Second
	if interval < 30*time.Second {
		interval = 5 * time.Minute
	}
	if durableSchedulerEnabled() {
		if err := startDurableScheduler(ctx, service, interval); err != nil {
			log.Printf("ambient scheduler: durable queue unavailable (%v); falling back to the in-process ticker", err)
		} else {
			return
		}
	}
	scheduler := &Scheduler{service: service}
	go scheduler.Start(ctx, interval)
}

func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 5 * time.Minute
	}
	if runOnStartup() {
		s.runOnce()
	}
	ticker := time.NewTicker(interval)
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

func runOnStartup() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("AMBIENT_RUN_ON_STARTUP")))
	return value == "true" || value == "1" || value == "yes"
}

func (s *Scheduler) runOnce() {
	if s.service == nil || !s.running.CompareAndSwap(false, true) {
		return
	}
	defer s.running.Store(false)
	scan, err := s.service.Scan("scheduler")
	if err != nil {
		log.Printf("ambient scan failed: %v", err)
		return
	}
	if scan.Created > 0 || scan.Updated > 0 || scan.Advanced > 0 {
		log.Printf("ambient scan examined=%d created=%d updated=%d deduplicated=%d advanced=%d filtered=%d skipped=%d blocked=%d", scan.ItemsExamined, scan.Created, scan.Updated, scan.Deduplicated, scan.Advanced, scan.Filtered, scan.Skipped, scan.Blocked)
	}
}
