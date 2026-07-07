// Package i18n provides a tiny, dependency-free message catalog for the two
// languages the product targets: English and Dutch. Lookups fall back to
// English and then to the key itself, so a missing translation is visible
// rather than blank.
package i18n

import (
	"sort"
	"strings"
)

// Lang is a supported language code.
type Lang string

const (
	EN Lang = "en"
	NL Lang = "nl"
)

var catalog = map[Lang]map[string]string{
	EN: {
		"app.title":        "Personal AI Operating System",
		"nav.memory":       "Memory",
		"nav.workflows":    "Workflows",
		"action.approve":   "Approve",
		"action.reject":    "Reject",
		"status.ready":     "Ready",
		"status.not_ready": "Not ready",
	},
	NL: {
		"app.title":        "Persoonlijk AI-besturingssysteem",
		"nav.memory":       "Geheugen",
		"nav.workflows":    "Werkstromen",
		"action.approve":   "Goedkeuren",
		"action.reject":    "Afwijzen",
		"status.ready":     "Gereed",
		"status.not_ready": "Niet gereed",
	},
}

// Normalize maps arbitrary input (e.g. "NL", "nl-NL") to a supported language,
// defaulting to English.
func Normalize(lang string) Lang {
	code := strings.ToLower(strings.TrimSpace(lang))
	if len(code) >= 2 && code[:2] == "nl" {
		return NL
	}
	return EN
}

// T returns the translation for key in lang, falling back to English and then
// to the key itself.
func T(lang Lang, key string) string {
	if messages, ok := catalog[lang]; ok {
		if v, ok := messages[key]; ok {
			return v
		}
	}
	if v, ok := catalog[EN][key]; ok {
		return v
	}
	return key
}

// Supported returns the supported languages, sorted.
func Supported() []Lang {
	out := make([]Lang, 0, len(catalog))
	for l := range catalog {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
