package infra

import "testing"

func TestMigrationsEnabledDefaultsToTrueAndCanBeDisabled(t *testing.T) {
	t.Setenv("DB_MIGRATIONS_ENABLED", "")
	if !migrationsEnabled() {
		t.Fatal("migrationsEnabled() = false, want legacy-compatible default true")
	}

	t.Setenv("DB_MIGRATIONS_ENABLED", "false")
	if migrationsEnabled() {
		t.Fatal("migrationsEnabled() = true, want false when explicitly disabled")
	}
}

func TestRuntimeRoleConfigurationRequiresBothValuesAndSafeIdentifier(t *testing.T) {
	t.Setenv("DB_RUNTIME_USER", "")
	t.Setenv("DB_RUNTIME_PASSWORD", "")
	if _, _, err := runtimeRoleConfiguration(); err != nil {
		t.Fatalf("empty runtime role configuration error = %v", err)
	}

	t.Setenv("DB_RUNTIME_USER", "hai_runtime")
	t.Setenv("DB_RUNTIME_PASSWORD", "")
	if _, _, err := runtimeRoleConfiguration(); err == nil {
		t.Fatal("runtime role configuration without password unexpectedly succeeded")
	}

	t.Setenv("DB_RUNTIME_USER", "unsafe-role")
	t.Setenv("DB_RUNTIME_PASSWORD", "safe-password")
	if _, _, err := runtimeRoleConfiguration(); err == nil {
		t.Fatal("runtime role configuration with unsafe identifier unexpectedly succeeded")
	}

	t.Setenv("DB_RUNTIME_USER", "hai_runtime")
	if user, password, err := runtimeRoleConfiguration(); err != nil || user != "hai_runtime" || password != "safe-password" {
		t.Fatalf("runtime role configuration = (%q, %q, %v), want configured values", user, password, err)
	}
}
