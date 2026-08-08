package migrations

import (
	"strings"
	"testing"
)

func TestModelOutcomeCalibrationMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0031_model_outcome_calibration.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ALTER TABLE public.model_run_telemetries",
		"validation_status",
		"schema_validated",
		"source_supported",
		"test_passed",
		"human_approved",
		"needs_review",
		"estimated_cost_eur >= 0",
		"fallback_depth BETWEEN 0 AND 32",
		"idx_model_run_telemetries_lane_calibration",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("model outcome calibration migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("model outcome calibration migration must not use CASCADE")
	}
	if strings.Contains(up, `\x00`) {
		t.Fatal("PostgreSQL text cannot contain NUL; an E-string NUL escape makes the migration itself invalid")
	}

	downBytes, err := Files.ReadFile("pre/0031_model_outcome_calibration.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty model outcome calibration data") {
		t.Fatal("model outcome calibration rollback must refuse to discard observed outcomes")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("model outcome calibration rollback must not use CASCADE")
	}
}
