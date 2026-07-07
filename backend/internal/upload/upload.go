// Package upload validates uploaded files before they are stored: the filename
// must be a safe relative path, its extension must be allowlisted, and its size
// must be within the configured limit. Pure and side-effect free.
package upload

import (
	"fmt"
	"path/filepath"
	"strings"

	"automation-hub-backend/internal/pathsafety"
)

// Policy describes upload constraints.
type Policy struct {
	MaxBytes     int64
	AllowedExts  []string // e.g. [".jpg", ".png"]; matched case-insensitively
}

// Validate checks filename and size against the policy. It returns a descriptive
// error for the first violation, or nil when the upload is acceptable.
func (p Policy) Validate(filename string, sizeBytes int64) error {
	if strings.TrimSpace(filename) == "" {
		return fmt.Errorf("filename is required")
	}
	if !pathsafety.IsSafeRelative(filename) {
		return fmt.Errorf("unsafe filename: %q", filename)
	}
	if sizeBytes <= 0 {
		return fmt.Errorf("empty upload")
	}
	if p.MaxBytes > 0 && sizeBytes > p.MaxBytes {
		return fmt.Errorf("file too large: %d bytes exceeds limit %d", sizeBytes, p.MaxBytes)
	}
	if !p.extAllowed(filename) {
		return fmt.Errorf("extension not allowed for %q", filename)
	}
	return nil
}

func (p Policy) extAllowed(filename string) bool {
	if len(p.AllowedExts) == 0 {
		return true // no allowlist configured => any extension
	}
	ext := strings.ToLower(filepath.Ext(filename))
	for _, allowed := range p.AllowedExts {
		if ext == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}
