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

func TestOpenSecureRootCreatesMissingNestedRoot(t *testing.T) {
	base := filepath.Join(t.TempDir(), "one", "two", "workspace")
	root, err := OpenSecureRoot(base, true)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()
	if root.Path() != base {
		t.Fatalf("root path = %q, want %q", root.Path(), base)
	}
	if err := root.CheckIdentity(); err != nil {
		t.Fatalf("created root identity: %v", err)
	}
}

func TestOpenSecureRootRejectsSymlinkComponent(t *testing.T) {
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "workspace"), 0o755); err != nil {
		t.Fatalf("create outside workspace: %v", err)
	}
	link := filepath.Join(parent, "linked-parent")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := OpenSecureRoot(filepath.Join(link, "workspace"), false); !errors.Is(err, ErrPathLink) {
		t.Fatalf("OpenSecureRoot through symlink error = %v, want ErrPathLink", err)
	}
}

func TestSecureRootExclusiveCreateRejectsTraversalAndExistingTarget(t *testing.T) {
	base := t.TempDir()
	existing := filepath.Join(base, "existing.txt")
	const original = "do not overwrite"
	if err := os.WriteFile(existing, []byte(original), 0o600); err != nil {
		t.Fatalf("seed existing target: %v", err)
	}
	root, err := OpenSecureRoot(base, false)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()

	if _, _, err := root.CreateExclusiveFile("../escape.txt", 0o600); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("traversal error = %v, want ErrUnsafePath", err)
	}
	if _, _, err := root.CreateExclusiveFile("existing.txt", 0o600); !errors.Is(err, ErrPathExists) {
		t.Fatalf("existing target error = %v, want ErrPathExists", err)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if string(data) != original {
		t.Fatalf("existing target changed to %q", data)
	}
}

func TestSecureRootRejectsSymlinkTarget(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "workspace")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("create outside file: %v", err)
	}
	link := filepath.Join(base, "artifact.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root, err := OpenSecureRoot(base, false)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()
	if _, _, err := root.CreateExclusiveFile("artifact.txt", 0o600); !errors.Is(err, ErrPathLink) {
		t.Fatalf("symlink target error = %v, want ErrPathLink", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside target was changed: data=%q err=%v", data, err)
	}
}

func TestSecureRootDetectsTargetSubstitution(t *testing.T) {
	base := t.TempDir()
	root, err := OpenSecureRoot(base, false)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()
	file, originalInfo, err := root.CreateExclusiveFile("artifact.txt", 0o600)
	if err != nil {
		t.Fatalf("CreateExclusiveFile: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString("expected"); err != nil {
		t.Fatalf("write original: %v", err)
	}

	target := filepath.Join(base, "artifact.txt")
	moved := filepath.Join(base, "original.txt")
	if err := os.Rename(target, moved); err != nil {
		t.Fatalf("move original target: %v", err)
	}
	if err := os.WriteFile(target, []byte("expected"), 0o600); err != nil {
		t.Fatalf("create byte-identical substitute: %v", err)
	}
	if err := root.VerifyFile("artifact.txt", file, originalInfo); !errors.Is(err, ErrPathSubstituted) {
		t.Fatalf("VerifyFile substitution error = %v, want ErrPathSubstituted", err)
	}
}

func TestSecureRootDetectsWorkspacePathSubstitution(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "workspace")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	root, err := OpenSecureRoot(base, false)
	if err != nil {
		t.Fatalf("OpenSecureRoot: %v", err)
	}
	defer root.Close()

	moved := filepath.Join(parent, "workspace-original")
	if err := os.Rename(base, moved); err != nil {
		t.Skipf("platform does not permit renaming an opened directory: %v", err)
	}
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("create replacement workspace: %v", err)
	}
	if err := root.CheckIdentity(); !errors.Is(err, ErrPathSubstituted) {
		t.Fatalf("CheckIdentity error = %v, want ErrPathSubstituted", err)
	}
}
