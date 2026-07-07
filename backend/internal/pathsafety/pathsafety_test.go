package pathsafety

import (
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
	for _, rel := range []string{"../etc/passwd", "images/../../secret", "..", "a/b/../../../c"} {
		if _, err := SafeJoin("/data/uploads", rel); err == nil {
			t.Fatalf("SafeJoin(%q) should have been rejected", rel)
		}
	}
}

func TestSafeJoinRejectsAbsoluteAndEmpty(t *testing.T) {
	if _, err := SafeJoin("/data/uploads", "/etc/passwd"); err == nil {
		t.Fatalf("absolute rel path should be rejected")
	}
	if _, err := SafeJoin("/data/uploads", ""); err == nil {
		t.Fatalf("empty rel path should be rejected")
	}
}

func TestIsSafeRelative(t *testing.T) {
	safe := []string{"a", "a/b", "a/b/c.txt", "./a"}
	unsafe := []string{"", "/abs", "../x", "a/../../b", ".."}
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
