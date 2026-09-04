package ambient

import "testing"

func TestAmbientPollIntervalUsesSafeBounds(t *testing.T) {
	for _, value := range []string{"", "1", "14", "3601", "invalid"} {
		t.Setenv("AMBIENT_WORKER_POLL_SECONDS", value)
		if got := ambientPollInterval(); got != defaultAmbientPoll {
			t.Fatalf("ambientPollInterval() with %q = %s, want default %s", value, got, defaultAmbientPoll)
		}
	}
	t.Setenv("AMBIENT_WORKER_POLL_SECONDS", "15")
	if got := ambientPollInterval(); got != minAmbientPollInterval {
		t.Fatalf("ambientPollInterval() = %s, want %s", got, minAmbientPollInterval)
	}
	t.Setenv("AMBIENT_WORKER_POLL_SECONDS", "3600")
	if got := ambientPollInterval(); got != maxAmbientPollInterval {
		t.Fatalf("ambientPollInterval() = %s, want %s", got, maxAmbientPollInterval)
	}
}
