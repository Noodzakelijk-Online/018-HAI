// Package templates provides reusable presets/defaults that seed common
// records so users do not start from a blank form. Templates are pure data with
// a small registry; applying a template fills only the fields the caller left
// empty, never overwriting explicit input.
package templates

import (
	"sort"
	"strings"
)

// MemoryTemplate is a preset for creating a context memory.
type MemoryTemplate struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags"`
	Confidence float64  `json:"confidence"`
}

// Draft is the mutable memory input a template fills in.
type Draft struct {
	Kind       string
	Summary    string
	Tags       []string
	Confidence float64
}

// Apply returns a copy of draft with any empty field populated from the
// template. Non-empty caller values are preserved (no overwrite).
func (t MemoryTemplate) Apply(draft Draft) Draft {
	out := draft
	if strings.TrimSpace(out.Kind) == "" {
		out.Kind = t.Kind
	}
	if strings.TrimSpace(out.Summary) == "" {
		out.Summary = t.Summary
	}
	if len(out.Tags) == 0 {
		out.Tags = append([]string{}, t.Tags...)
	}
	if out.Confidence == 0 {
		out.Confidence = t.Confidence
	}
	return out
}

// Registry holds named templates.
type Registry struct {
	memory map[string]MemoryTemplate
}

// DefaultRegistry returns a registry seeded with common built-in presets.
func DefaultRegistry() *Registry {
	r := &Registry{memory: map[string]MemoryTemplate{}}
	for _, t := range []MemoryTemplate{
		{Name: "preference", Kind: "preference", Summary: "User preference", Tags: []string{"preference"}, Confidence: 0.8},
		{Name: "decision", Kind: "decision", Summary: "Recorded decision", Tags: []string{"decision"}, Confidence: 0.85},
		{Name: "contact", Kind: "contact", Summary: "Person or organization", Tags: []string{"contact"}, Confidence: 0.7},
	} {
		r.memory[t.Name] = t
	}
	return r
}

// Memory returns a memory template by name.
func (r *Registry) Memory(name string) (MemoryTemplate, bool) {
	t, ok := r.memory[strings.TrimSpace(strings.ToLower(name))]
	return t, ok
}

// ListMemory returns all memory templates sorted by name.
func (r *Registry) ListMemory() []MemoryTemplate {
	out := make([]MemoryTemplate, 0, len(r.memory))
	for _, t := range r.memory {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
