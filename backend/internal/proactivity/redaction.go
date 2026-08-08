package proactivity

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const redactedValue = "[redacted]"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AIza[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16})\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^/@\s:]+:[^/@\s]+@`),
	regexp.MustCompile(`(?i)([?&](?:api[-_]?key|access[-_]?token|refresh[-_]?token|token|password|secret)=)[^&\s]+`),
	regexp.MustCompile(`(?i)\b(password|passwd|secret|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|authorization)\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`),
}

func containsSecret(value string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func redactAndBound(value string, limit int) string {
	value = strings.TrimSpace(value)
	for _, pattern := range secretPatterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			if strings.HasPrefix(match, "?") || strings.HasPrefix(match, "&") {
				if index := strings.Index(match, "="); index >= 0 {
					return match[:index+1] + redactedValue
				}
			}
			if index := strings.IndexAny(match, ":="); index >= 0 {
				return strings.TrimSpace(match[:index]) + "=" + redactedValue
			}
			return redactedValue
		})
	}
	value = strings.Map(func(char rune) rune {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return -1
		}
		return char
	}, value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
