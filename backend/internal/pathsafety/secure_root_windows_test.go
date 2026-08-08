//go:build windows

package pathsafety

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenSecureRootRejectsWindowsJunction(t *testing.T) {
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "workspace"), 0o755); err != nil {
		t.Fatalf("create outside workspace: %v", err)
	}
	junction := filepath.Join(parent, "junction")
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	if _, err := OpenSecureRoot(filepath.Join(junction, "workspace"), false); !errors.Is(err, ErrPathLink) {
		t.Fatalf("OpenSecureRoot through junction error = %v, want ErrPathLink", err)
	}
}

func TestSecureRootRejectsWindowsJunctionTarget(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "workspace")
	outside := filepath.Join(parent, "outside")
	for _, directory := range []string{base, outside} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	junction := filepath.Join(base, "artifact.txt")
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	root, err := OpenSecureRoot(base, false)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()
	if _, _, err := root.CreateExclusiveFile("artifact.txt", 0o600); !errors.Is(err, ErrPathLink) {
		t.Fatalf("junction target error = %v, want ErrPathLink", err)
	}
}
