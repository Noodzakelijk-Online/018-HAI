package ambient

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

type Scheduler struct {
	service Service
	running atomic.Bool
}

func StartScheduler(ctx context.Context, service Service) {
	policy := policyFromEnv()
	if !policy.SchedulerEnabled {
		return
	}
	scheduler := &Scheduler{service: service}
	go scheduler.Start(ctx, time.Duration(policy.ScanIntervalSeconds)*time.Second)
}

func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 5 * time.Minute
	}
	s.runOnce()
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
