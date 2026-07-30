package pathsafety

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinAllowsNestedPaths(t *testing.T) {
	got, err := SafeJoin("/data/uploads", "images/photo.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Clean("/data/uploads/images/photo.png")
	if got != want {
		t.Fatalf("SafeJoin = %q, want %q", got, want)
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	for _, rel := range []string{
		"../etc/passwd",
		"images/../../secret",
		"..",
		"a/b/../../../c",
		`..\etc\passwd`,
		`images\..\..\secret`,
	} {
		if _, err := SafeJoin("/data/uploads", rel); err == nil {
			t.Fatalf("SafeJoin(%q) should have been rejected", rel)
		}
	}
}

func TestSafeJoinRejectsAbsoluteAndEmpty(t *testing.T) {
	for _, rel := range []string{
		"/etc/passwd",
		`\Windows\System32`,
		`C:\Windows\System32`,
		`C:Windows\System32`,
		`\\server\share\file`,
		`\\?\C:\Windows\System32`,
	} {
		if _, err := SafeJoin("/data/uploads", rel); err == nil {
			t.Fatalf("absolute or volume-qualified rel path %q should be rejected", rel)
		}
	}
	if _, err := SafeJoin("/data/uploads", ""); err == nil {
		t.Fatalf("empty rel path should be rejected")
	}
	if _, err := SafeJoin("/data/uploads", "safe\x00unsafe"); err == nil {
		t.Fatalf("NUL-containing rel path should be rejected")
	}
}

func TestIsSafeRelative(t *testing.T) {
	safe := []string{"a", "a/b", "a/b/c.txt", "./a", `a\b\c.txt`}
	unsafe := []string{
		"",
		"/abs",
		`\abs`,
		`C:\abs`,
		`C:drive-relative`,
		`\\server\share`,
		"../x",
		`..\x`,
		"a/../../b",
		`a\..\..\b`,
		"..",
		"safe\x00unsafe",
	}
	for _, s := range safe {
		if !IsSafeRelative(s) {
			t.Fatalf("%q should be safe", s)
		}
	}
	for _, u := range unsafe {
		if IsSafeRelative(u) {
			t.Fatalf("%q should be unsafe", u)
		}
	}
}

func TestResolveWithinBaseAllowsExistingNestedTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "nested", "runtime")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create nested target: %v", err)
	}

	resolved, err := ResolveWithinBase(base, target)
	if err != nil {
		t.Fatalf("ResolveWithinBase returned error: %v", err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve expected target: %v", err)
	}
	if resolved != want {
		t.Fatalf("ResolveWithinBase = %q, want %q", resolved, want)
	}
}

func TestResolveWithinBaseRejectsSiblingPrefixAndRelativeInputs(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "allowed")
	sibling := filepath.Join(parent, "allowed-outside")
	for _, directory := range []string{base, sibling} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}

	for _, test := range []struct {
		name   string
		base   string
		target string
	}{
		{name: "sibling prefix", base: base, target: sibling},
		{name: "relative base", base: "allowed", target: sibling},
		{name: "relative target", base: base, target: "nested"},
		{name: "empty base", base: "", target: sibling},
		{name: "empty target", base: base, target: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveWithinBase(test.base, test.target); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("ResolveWithinBase(%q, %q) error = %v, want ErrUnsafePath", test.base, test.target, err)
			}
		})
	}
}

func TestResolveWithinBaseRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "allowed")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create base: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside target: %v", err)
	}
	link := filepath.Join(base, "runtime-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := ResolveWithinBase(base, link); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink escape error = %v, want ErrUnsafePath", err)
	}
}
