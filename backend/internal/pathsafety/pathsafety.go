// Package pathsafety contains pure helpers that prevent path-traversal escapes
// when combining a trusted base directory with an untrusted relative path.
package pathsafety

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a relative path would escape its base
// directory or is absolute.
var ErrUnsafePath = errors.New("unsafe path: escapes base directory")

// IsSafeRelative reports whether rel is a relative path that stays within its
// base once cleaned (no absolute paths, no ".." escapes).
func IsSafeRelative(rel string) bool {
	if rel == "" || strings.IndexByte(rel, 0) >= 0 {
		return false
	}
	if isAbsoluteOnAnyPlatform(rel) {
		return false
	}
	cleaned := path.Clean(strings.ReplaceAll(rel, `\`, "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
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
	if strings.TrimSpace(base) == "" || strings.IndexByte(base, 0) >= 0 {
		return "", ErrUnsafePath
	}
	cleanBase := filepath.Clean(base)
	nativeRel := filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/"))
	joined := filepath.Clean(filepath.Join(cleanBase, nativeRel))

	// Defense in depth: confirm the cleaned join is still within base.
	relativeToBase, err := filepath.Rel(cleanBase, joined)
	if err != nil || relativeToBase == ".." ||
		strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relativeToBase) {
		return "", ErrUnsafePath
	}
	return joined, nil
}

// ResolveWithinBase resolves filesystem links for an existing target and
// verifies that the resolved target remains within an existing absolute base
// directory. Use it when a caller can select an executable, workspace, upload,
// or ecosystem path that will later be read or executed.
func ResolveWithinBase(base, target string) (string, error) {
	base = strings.TrimSpace(base)
	target = strings.TrimSpace(target)
	if base == "" || target == "" ||
		strings.IndexByte(base, 0) >= 0 || strings.IndexByte(target, 0) >= 0 ||
		!filepath.IsAbs(base) || !filepath.IsAbs(target) {
		return "", ErrUnsafePath
	}

	resolvedBase, err := filepath.EvalSymlinks(filepath.Clean(base))
	if err != nil {
		return "", fmt.Errorf("%w: resolve base: %v", ErrUnsafePath, err)
	}
	baseInfo, err := os.Stat(resolvedBase)
	if err != nil || !baseInfo.IsDir() {
		return "", fmt.Errorf("%w: base is not an existing directory", ErrUnsafePath)
	}

	resolvedTarget, err := filepath.EvalSymlinks(filepath.Clean(target))
	if err != nil {
		return "", fmt.Errorf("%w: resolve target: %v", ErrUnsafePath, err)
	}
	relativeToBase, err := filepath.Rel(resolvedBase, resolvedTarget)
	if err != nil || filepath.IsAbs(relativeToBase) ||
		relativeToBase == ".." ||
		strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return resolvedTarget, nil
}

func isAbsoluteOnAnyPlatform(value string) bool {
	normalized := strings.ReplaceAll(value, `\`, "/")
	if strings.HasPrefix(normalized, "/") {
		return true
	}
	if len(normalized) >= 2 && normalized[1] == ':' &&
		((normalized[0] >= 'a' && normalized[0] <= 'z') ||
			(normalized[0] >= 'A' && normalized[0] <= 'Z')) {
		return true
	}
	return filepath.IsAbs(value)
}
