package source

import (
	"testing"
)

func TestSourceSchedulerIntervalUsesSafeBounds(t *testing.T) {
	for _, value := range []string{"", "1", "14", "86401", "invalid"} {
		t.Setenv("SOURCE_SCHEDULER_INTERVAL_SECONDS", value)
		if got := schedulerInterval(); got != defaultSchedulerInterval {
			t.Fatalf("schedulerInterval() with %q = %s, want default %s", value, got, defaultSchedulerInterval)
		}
	}
	t.Setenv("SOURCE_SCHEDULER_INTERVAL_SECONDS", "15")
	if got := schedulerInterval(); got != minSchedulerInterval {
		t.Fatalf("schedulerInterval() = %s, want %s", got, minSchedulerInterval)
	}
	t.Setenv("SOURCE_SCHEDULER_INTERVAL_SECONDS", "86400")
	if got := schedulerInterval(); got != maxSchedulerInterval {
		t.Fatalf("schedulerInterval() = %s, want %s", got, maxSchedulerInterval)
	}
}
