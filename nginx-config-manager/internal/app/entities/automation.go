package entities

import (
	"fmt"
	"strings"
)

type Automation struct {
	Name       string `json:"name"`
	URLPath    string `json:"urlPath"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	OldUrlPath string `json:"oldUrlPath,omitempty"`
}

func (a *Automation) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.Host == "" {
		return fmt.Errorf("hostname is required")
	}
	if a.URLPath == "" {
		return fmt.Errorf("URL path is required")
	}
	if !safeSlug(a.URLPath) {
		return fmt.Errorf("URL path must contain only letters, numbers, and hyphens")
	}
	if a.OldUrlPath != "" && !safeSlug(a.OldUrlPath) {
		return fmt.Errorf("old URL path must contain only letters, numbers, and hyphens")
	}
	if !safeHost(a.Host) {
		return fmt.Errorf("hostname must be a DNS name or IPv4-style host without schemes, slashes, spaces, or template characters")
	}
	if a.Port <= 0 || a.Port > 65535 {
		return fmt.Errorf("error: Port %d is not valid", a.Port)
	}
	return nil
}

func safeSlug(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func safeHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}
