package safety

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	keyValueSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|authorization)\b\s*[:=]\s*['"]?[^'"\s,;]+`)
	bearerSecretPattern   = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`)
	providerTokenPattern  = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{20,}|gh[pousr]_[a-z0-9_]{20,}|xox[baprs]-[a-z0-9-]{20,})\b`)
	privateKeyPattern     = regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
)

func RedactSecrets(value string) string {
	if value == "" {
		return ""
	}
	redacted := privateKeyPattern.ReplaceAllString(value, "[REDACTED_PRIVATE_KEY]")
	redacted = bearerSecretPattern.ReplaceAllString(redacted, "Bearer [REDACTED]")
	redacted = providerTokenPattern.ReplaceAllString(redacted, "[REDACTED_PROVIDER_TOKEN]")
	redacted = keyValueSecretPattern.ReplaceAllStringFunc(redacted, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED_SECRET]"
		}
		return strings.TrimSpace(match[:separator+1]) + " [REDACTED]"
	})
	return redacted
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return RedactSecrets(raw)
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveKey(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "password") ||
		strings.Contains(key, "passwd") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		key == "key" ||
		strings.Contains(key, "authorization")
}
