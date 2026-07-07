package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestSnapshotIsSelfDescribing(t *testing.T) {
	info := Snapshot()
	if info.Version == "" || info.Commit == "" || info.BuildTime == "" {
		t.Fatalf("snapshot has empty fields: %+v", info)
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Fatalf("GoVersion should look like a Go version: %q", info.GoVersion)
	}
}
