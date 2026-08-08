package pathsafety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrPathLink is returned when an existing path component is a symbolic
	// link, junction, mount-point reparse record, or other Windows reparse
	// point.
	ErrPathLink = errors.New("unsafe path: link or reparse point")
	// ErrPathExists is returned when an exclusive target already exists.
	ErrPathExists = errors.New("unsafe path: target already exists")
	// ErrPathSubstituted is returned when a path no longer names the file or
	// directory handle that was validated.
	ErrPathSubstituted = errors.New("unsafe path: path was substituted")
)

// SecureRoot is an opened, identity-pinned workspace directory. Files are
// accessed through the os.Root handle so concurrent renames cannot redirect an
// operation outside the directory that was validated.
type SecureRoot struct {
	path string
	root *os.Root
	info os.FileInfo
}

// ValidateNoLinks rejects every existing symlink or Windows reparse point in
// name. If allowMissing is true, a non-existing suffix is allowed after all
// existing components have been checked.
func ValidateNoLinks(name string, allowMissing bool) (string, error) {
	absolute, err := cleanAbsolute(name)
	if err != nil {
		return "", err
	}
	_, complete, err := walkAbsoluteNoLinks(absolute)
	if err != nil {
		return "", err
	}
	if !complete && !allowMissing {
		return "", fmt.Errorf("%w: path does not exist", ErrUnsafePath)
	}
	return absolute, nil
}

// OpenSecureRoot opens base as a rooted filesystem handle after rejecting
// links and reparse points in every existing component. When create is true,
// a missing suffix is created through an already-opened ancestor root.
func OpenSecureRoot(base string, create bool) (*SecureRoot, error) {
	absolute, err := cleanAbsolute(base)
	if err != nil {
		return nil, err
	}
	deepest, complete, err := walkAbsoluteNoLinks(absolute)
	if err != nil {
		return nil, err
	}
	if !complete && !create {
		return nil, fmt.Errorf("%w: workspace root does not exist", ErrUnsafePath)
	}

	ancestor, ancestorInfo, err := openPinnedRoot(deepest)
	if err != nil {
		return nil, err
	}
	if complete {
		secured := &SecureRoot{path: absolute, root: ancestor, info: ancestorInfo}
		if err := secured.CheckIdentity(); err != nil {
			_ = secured.Close()
			return nil, err
		}
		return secured, nil
	}

	relative, err := filepath.Rel(deepest, absolute)
	if err != nil || !IsSafeRelative(relative) {
		_ = ancestor.Close()
		return nil, ErrUnsafePath
	}
	child, childInfo, err := createAndOpenDirectoryChain(ancestor, relative)
	if err != nil {
		return nil, err
	}

	secured := &SecureRoot{path: absolute, root: child, info: childInfo}
	if err := secured.CheckIdentity(); err != nil {
		_ = secured.Close()
		return nil, err
	}
	return secured, nil
}

// Path returns the absolute path used to open this root.
func (r *SecureRoot) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Close releases the rooted directory handle.
func (r *SecureRoot) Close() error {
	if r == nil || r.root == nil {
		return nil
	}
	return r.root.Close()
}

// CheckIdentity confirms that the configured path still contains no links and
// still names the directory handle opened for this SecureRoot.
func (r *SecureRoot) CheckIdentity() error {
	if r == nil || r.root == nil || r.info == nil {
		return ErrPathSubstituted
	}
	if _, complete, err := walkAbsoluteNoLinks(r.path); err != nil {
		return err
	} else if !complete {
		return ErrPathSubstituted
	}
	current, err := os.Lstat(r.path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPathSubstituted, err)
	}
	if hasLinkOrReparse(current) || !current.IsDir() || !os.SameFile(current, r.info) {
		return ErrPathSubstituted
	}
	opened, err := r.root.Stat(".")
	if err != nil || !os.SameFile(opened, r.info) {
		return ErrPathSubstituted
	}
	return nil
}

// CreateExclusiveFile creates a new regular file without following an existing
// target. Existing targets are always rejected, including links and reparse
// points.
func (r *SecureRoot) CreateExclusiveFile(name string, perm os.FileMode) (*os.File, os.FileInfo, error) {
	if err := r.validateRelative(name); err != nil {
		return nil, nil, err
	}
	if err := r.CheckIdentity(); err != nil {
		return nil, nil, err
	}
	if info, err := r.root.Lstat(name); err == nil {
		if hasLinkOrReparse(info) {
			return nil, nil, ErrPathLink
		}
		return nil, nil, ErrPathExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("inspect exclusive target: %w", err)
	}
	if err := checkRelativeComponents(r.root, filepath.Dir(name), true); err != nil {
		return nil, nil, err
	}

	file, err := r.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, nil, ErrPathExists
		}
		return nil, nil, fmt.Errorf("create exclusive target: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		r.removeIfSame(name, info)
		return nil, nil, fmt.Errorf("%w: target is not a regular file", ErrUnsafePath)
	}
	if err := r.VerifyFile(name, file, info); err != nil {
		r.removeIfSame(name, info)
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

// OpenExistingFile opens an existing regular file through the rooted handle
// and rejects links, reparse points, and substitutions.
func (r *SecureRoot) OpenExistingFile(name string) (*os.File, os.FileInfo, error) {
	if err := r.validateRelative(name); err != nil {
		return nil, nil, err
	}
	if err := r.CheckIdentity(); err != nil {
		return nil, nil, err
	}
	pathInfo, err := r.root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if hasLinkOrReparse(pathInfo) || !pathInfo.Mode().IsRegular() {
		return nil, nil, ErrPathLink
	}
	if err := checkRelativeComponents(r.root, name, false); err != nil {
		return nil, nil, err
	}
	file, err := r.root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) {
		_ = file.Close()
		return nil, nil, ErrPathSubstituted
	}
	if err := r.VerifyFile(name, file, info); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

// VerifyFile confirms that name still resolves, through the opened root, to
// the same regular file represented by file and original.
func (r *SecureRoot) VerifyFile(name string, file *os.File, original os.FileInfo) error {
	if file == nil || original == nil {
		return ErrPathSubstituted
	}
	if err := r.CheckIdentity(); err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, original) {
		return ErrPathSubstituted
	}
	current, err := r.root.Lstat(name)
	if err != nil || hasLinkOrReparse(current) || !current.Mode().IsRegular() ||
		!os.SameFile(current, original) {
		return ErrPathSubstituted
	}
	if err := checkRelativeComponents(r.root, name, false); err != nil {
		return err
	}
	probe, err := r.root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return ErrPathSubstituted
	}
	probeInfo, statErr := probe.Stat()
	closeErr := probe.Close()
	if statErr != nil || closeErr != nil || !os.SameFile(probeInfo, original) {
		return ErrPathSubstituted
	}
	return r.CheckIdentity()
}

// RemoveIfSame removes name only if it still identifies original. This avoids
// deleting an attacker-substituted path during failure cleanup.
func (r *SecureRoot) RemoveIfSame(name string, original os.FileInfo) {
	r.removeIfSame(name, original)
}

func (r *SecureRoot) removeIfSame(name string, original os.FileInfo) {
	if r == nil || r.root == nil || original == nil {
		return
	}
	current, err := r.root.Lstat(name)
	if err == nil && !hasLinkOrReparse(current) && os.SameFile(current, original) {
		_ = r.root.Remove(name)
	}
}

func (r *SecureRoot) validateRelative(name string) error {
	if r == nil || r.root == nil || !IsSafeRelative(name) {
		return ErrUnsafePath
	}
	cleaned := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(name, `\`, "/")))
	if cleaned == "." {
		return ErrUnsafePath
	}
	return nil
}

func openPinnedRoot(name string) (*os.Root, os.FileInfo, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if hasLinkOrReparse(before) || !before.IsDir() {
		return nil, nil, ErrPathLink
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	after, err := os.Lstat(name)
	if err != nil || hasLinkOrReparse(after) || !os.SameFile(before, opened) ||
		!os.SameFile(after, opened) {
		_ = root.Close()
		return nil, nil, ErrPathSubstituted
	}
	return root, opened, nil
}

func createAndOpenDirectoryChain(ancestor *os.Root, relative string) (*os.Root, os.FileInfo, error) {
	current := ancestor
	var currentInfo os.FileInfo
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if err := current.Mkdir(part, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			_ = current.Close()
			return nil, nil, fmt.Errorf("create secure root component %s: %w", part, err)
		}
		pathInfo, err := current.Lstat(part)
		if err != nil || hasLinkOrReparse(pathInfo) || !pathInfo.IsDir() {
			_ = current.Close()
			return nil, nil, fmt.Errorf("%w: secure root component %s", ErrPathLink, part)
		}
		child, err := current.OpenRoot(part)
		if err != nil {
			_ = current.Close()
			return nil, nil, fmt.Errorf("open secure root component %s: %w", part, err)
		}
		childInfo, err := child.Stat(".")
		if err != nil {
			_ = child.Close()
			_ = current.Close()
			return nil, nil, fmt.Errorf("stat secure root component %s: %w", part, err)
		}
		after, err := current.Lstat(part)
		if err != nil || hasLinkOrReparse(after) || !os.SameFile(pathInfo, childInfo) ||
			!os.SameFile(after, childInfo) {
			_ = child.Close()
			_ = current.Close()
			return nil, nil, ErrPathSubstituted
		}
		_ = current.Close()
		current = child
		currentInfo = childInfo
	}
	if currentInfo == nil {
		_ = current.Close()
		return nil, nil, ErrUnsafePath
	}
	return current, currentInfo, nil
}

func checkRelativeComponents(root *os.Root, relative string, requireDirectory bool) error {
	if relative == "" || relative == "." {
		return nil
	}
	cleaned := filepath.Clean(relative)
	if !IsSafeRelative(cleaned) {
		return ErrUnsafePath
	}
	parts := strings.Split(cleaned, string(filepath.Separator))
	current := ""
	for index, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if hasLinkOrReparse(info) {
			return fmt.Errorf("%w: %s", ErrPathLink, current)
		}
		if (index < len(parts)-1 || requireDirectory) && !info.IsDir() {
			return fmt.Errorf("%w: non-directory component %s", ErrUnsafePath, current)
		}
	}
	return nil
}

func cleanAbsolute(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return "", ErrUnsafePath
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	return filepath.Clean(absolute), nil
}

// walkAbsoluteNoLinks returns the deepest existing component and whether the
// complete path exists. Every existing component is checked without following
// links.
func walkAbsoluteNoLinks(absolute string) (string, bool, error) {
	prefixes, err := absolutePrefixes(absolute)
	if err != nil || len(prefixes) == 0 {
		return "", false, ErrUnsafePath
	}
	deepest := prefixes[0]
	for index, current := range prefixes {
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return deepest, false, nil
		}
		if statErr != nil {
			return "", false, statErr
		}
		if hasLinkOrReparse(info) {
			return "", false, fmt.Errorf("%w: %s", ErrPathLink, current)
		}
		if index < len(prefixes)-1 && !info.IsDir() {
			return "", false, fmt.Errorf("%w: non-directory component %s", ErrUnsafePath, current)
		}
		deepest = current
	}
	return deepest, true, nil
}

func absolutePrefixes(absolute string) ([]string, error) {
	if !filepath.IsAbs(absolute) {
		return nil, ErrUnsafePath
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimLeft(remainder, `/\`)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	prefixes := []string{filepath.Clean(root)}
	current := root
	if remainder == "" {
		return prefixes, nil
	}
	for _, part := range strings.FieldsFunc(remainder, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		current = filepath.Join(current, part)
		prefixes = append(prefixes, current)
	}
	return prefixes, nil
}
