package main

import "testing"

func TestParseMigrationTarget(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantDir     string
		wantVersion string
		wantError   bool
	}{
		{name: "legacy post default", target: "0002_durable_jobs_indexes", wantDir: "post", wantVersion: "0002_durable_jobs_indexes"},
		{name: "pre target", target: "pre/0003_framework_registry", wantDir: "pre", wantVersion: "0003_framework_registry"},
		{name: "windows separator", target: `post\0001_conversation_owner_identity`, wantDir: "post", wantVersion: "0001_conversation_owner_identity"},
		{name: "invalid phase", target: "other/0001_anything", wantError: true},
		{name: "nested path", target: "pre/../secret", wantError: true},
		{name: "empty", target: " ", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir, version, err := parseMigrationTarget(test.target)
			if test.wantError {
				if err == nil {
					t.Fatalf("parseMigrationTarget(%q) unexpectedly succeeded: %s/%s", test.target, dir, version)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMigrationTarget(%q): %v", test.target, err)
			}
			if dir != test.wantDir || version != test.wantVersion {
				t.Fatalf("parseMigrationTarget(%q) = %s/%s, want %s/%s", test.target, dir, version, test.wantDir, test.wantVersion)
			}
		})
	}
}
