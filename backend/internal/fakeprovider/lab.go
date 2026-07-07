package fakeprovider

import "sort"

// Lab is a named registry of fake providers for tests — a "fake provider lab"
// where a test can wire several controllable providers and look them up by name.
type Lab struct {
	providers map[string]*Provider
}

// NewLab returns an empty lab.
func NewLab() *Lab {
	return &Lab{providers: map[string]*Provider{}}
}

// Register adds a provider to the lab and returns the lab for chaining.
func (l *Lab) Register(p *Provider) *Lab {
	l.providers[p.name] = p
	return l
}

// Get returns a provider by name.
func (l *Lab) Get(name string) (*Provider, bool) {
	p, ok := l.providers[name]
	return p, ok
}

// Names returns the registered provider names, sorted.
func (l *Lab) Names() []string {
	out := make([]string, 0, len(l.providers))
	for name := range l.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
