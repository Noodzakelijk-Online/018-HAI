// Package pathsafety contains pure helpers that prevent path-traversal escapes
// when combining a trusted base directory with an untrusted relative path.
package pathsafety

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a relative path would escape its base
// directory or is absolute.
var ErrUnsafePath = errors.New("unsafe path: escapes base directory")

// IsSafeRelative reports whether rel is a relative path that stays within its
// base once cleaned (no absolute paths, no ".." escapes).
func IsSafeRelative(rel string) bool {
	if rel == "" {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// SafeJoin joins base and rel, guaranteeing the result stays inside base.
// It rejects absolute rel values and any ".." traversal that would escape base.
func SafeJoin(base, rel string) (string, error) {
	if !IsSafeRelative(rel) {
		return "", ErrUnsafePath
	}
	cleanBase := filepath.Clean(base)
	joined := filepath.Clean(filepath.Join(cleanBase, rel))

	// Defense in depth: confirm the cleaned join is still within base.
	if joined != cleanBase &&
		!strings.HasPrefix(joined, cleanBase+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return joined, nil
}
