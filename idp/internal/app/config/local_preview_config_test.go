package config

import (
	"strings"
	"testing"
)

func TestLocalPreviewConfigRequiresExplicitLoopbackBind(t *testing.T) {
	for _, hostBind := range []string{"127.0.0.1", "::1", "[::1]", "localhost"} {
		t.Run(strings.ReplaceAll(hostBind, ":", "_"), func(t *testing.T) {
			t.Setenv(localLoginBypassEnabled, "true")
			t.Setenv(firstRunAdminEmail, "owner@example.com")
			t.Setenv(gatewayHostBind, hostBind)

			cfg, err := newLocalPreviewConfig()
			if err != nil {
				t.Fatalf("newLocalPreviewConfig() error = %v", err)
			}
			if !cfg.Enabled {
				t.Fatal("local preview should be enabled for an explicit loopback bind")
			}
		})
	}
}

func TestLocalPreviewConfigRejectsUnsafeOrMissingBind(t *testing.T) {
	for _, hostBind := range []string{"", "0.0.0.0", "::", "192.168.1.20", "example.com"} {
		t.Run(strings.ReplaceAll(hostBind, ":", "_"), func(t *testing.T) {
			t.Setenv(localLoginBypassEnabled, "true")
			t.Setenv(firstRunAdminEmail, "owner@example.com")
			t.Setenv(gatewayHostBind, hostBind)

			if _, err := newLocalPreviewConfig(); err == nil {
				t.Fatalf("expected unsafe bind %q to be rejected", hostBind)
			}
		})
	}
}

func TestLocalPreviewConfigAllowsPublicBindWhenBypassIsDisabled(t *testing.T) {
	t.Setenv(localLoginBypassEnabled, "false")
	t.Setenv(gatewayHostBind, "0.0.0.0")

	cfg, err := newLocalPreviewConfig()
	if err != nil {
		t.Fatalf("newLocalPreviewConfig() error = %v", err)
	}
	if cfg.Enabled {
		t.Fatal("local preview must remain disabled")
	}
}
