package llm

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const maintenanceSchedulerIntervalEnv = "LLM_MODEL_MAINTENANCE_SCHEDULER_INTERVAL_MINUTES"

// StartModelMaintenanceScheduler keeps configured local models fresh even when
// no task happens to route through them. Its sweep is intentionally small:
// RunDueModelMaintenance consults durable 24-hour records before it performs
// provider I/O. The caller supplies the emergency-stop check so maintenance
// cannot continue while HAI background processing is paused.
func StartModelMaintenanceScheduler(ctx context.Context, service *Service, backgroundAllowed func() bool) {
	if service == nil || !modelMaintenanceSchedulerEnabled() {
		return
	}
	interval := modelMaintenanceSchedulerInterval()
	run := func() {
		if backgroundAllowed != nil && !backgroundAllowed() {
			return
		}
		reportModelMaintenanceRun(service.RunDueModelMaintenance())
	}
	go func() {
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// reportModelMaintenanceRun emits only aggregate state. It intentionally does
// not log provider endpoints, model output, or individual failure details.
func reportModelMaintenanceRun(run ModelMaintenanceRun) {
	if !modelMaintenanceNeedsReport(run) {
		return
	}
	log.Printf(
		"model maintenance eligible=%d checked=%d updated=%d failed=%d reused=%d",
		run.Eligible,
		run.Checked,
		run.Updated,
		run.Failed,
		run.Reused,
	)
}

func modelMaintenanceNeedsReport(run ModelMaintenanceRun) bool {
	return run.Failed > 0 || run.Updated > 0
}

func modelMaintenanceSchedulerEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("LLM_MODEL_MAINTENANCE_SCHEDULER_ENABLED"))
	if raw == "" {
		return true
	}
	return envEnabled("LLM_MODEL_MAINTENANCE_SCHEDULER_ENABLED")
}

func modelMaintenanceSchedulerInterval() time.Duration {
	minutes := 60
	if raw := strings.TrimSpace(os.Getenv(maintenanceSchedulerIntervalEnv)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			minutes = value
		}
	}
	// A daily due time can fall just after a scheduler tick. Keep this sweep
	// exactly hourly: lower values waste background resources and higher values
	// could delay the due check by nearly another day. Durable per-model records
	// prevent provider I/O until the daily gate is actually due.
	if minutes != 60 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Minute
}
