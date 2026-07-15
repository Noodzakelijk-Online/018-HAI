package phase2

import "testing"

func TestConfigFromEnvDoesNotInventAFeed(t *testing.T) {
	t.Setenv("HAI_PHASE2_FEED_FILES", "")
	cfg := ConfigFromEnv()
	if len(cfg.FeedFiles) != 0 {
		t.Fatalf("FeedFiles = %#v, want no implicit feed", cfg.FeedFiles)
	}
}
