package infra

import (
	"strings"
	"testing"
)

func TestConfiguredFirstRunAdminPasswordRejectsMissingAndPlaceholderValues(t *testing.T) {
	for _, password := range []string{"", "ChangeMe123!", "change-this-local-admin-password", "short"} {
		t.Run(password, func(t *testing.T) {
			t.Setenv("FIRST_RUN_ADMIN_PASSWORD", password)
			_, err := configuredFirstRunAdminPassword()
			if err == nil {
				t.Fatalf("password %q unexpectedly accepted", password)
			}
		})
	}
}

func TestConfiguredFirstRunAdminPasswordAcceptsStrongOperatorValue(t *testing.T) {
	t.Setenv("FIRST_RUN_ADMIN_PASSWORD", "correct-horse-battery-staple-2026")
	password, err := configuredFirstRunAdminPassword()
	if err != nil {
		t.Fatal(err)
	}
	if password != "correct-horse-battery-staple-2026" {
		t.Fatalf("password = %q", password)
	}
}

func TestConfiguredFirstRunAdminPasswordDoesNotExposeConfiguredValueInError(t *testing.T) {
	const secret = "ChangeMe123!"
	t.Setenv("FIRST_RUN_ADMIN_PASSWORD", secret)
	_, err := configuredFirstRunAdminPassword()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v, must not expose configured password", err)
	}
}
