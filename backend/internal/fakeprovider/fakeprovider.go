// Package fakeprovider is a controllable LLM/provider stub for tests only. It
// simulates provider failures (errors, or failing after N calls) so failure
// handling can be exercised deterministically without any real provider.
package fakeprovider

import (
	"errors"
	"sync"
)

// ErrSimulated is returned when the stub is configured to fail.
var ErrSimulated = errors.New("fakeprovider: simulated failure")

// Provider is a fake generation provider.
type Provider struct {
	mu        sync.Mutex
	name      string
	response  string
	err       error
	failAfter int // -1 = never; 0 = always; N = fail once call count exceeds N
	calls     int
}

// New returns a provider that always succeeds with the given response.
func New(name, response string) *Provider {
	return &Provider{name: name, response: response, failAfter: -1}
}

// AlwaysFail configures the provider to fail on every call with err (or a
// default simulated error).
func (p *Provider) AlwaysFail(err error) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAfter = 0
	p.err = orDefault(err)
	return p
}

// FailAfter configures the provider to succeed for the first n calls, then fail.
func (p *Provider) FailAfter(n int, err error) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAfter = n
	p.err = orDefault(err)
	return p
}

// Generate returns the configured response or a simulated failure.
func (p *Provider) Generate(prompt string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	switch {
	case p.failAfter == 0:
		return "", p.err
	case p.failAfter > 0 && p.calls > p.failAfter:
		return "", p.err
	default:
		return p.response, nil
	}
}

// Calls returns how many times Generate was invoked.
func (p *Provider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func orDefault(err error) error {
	if err == nil {
		return ErrSimulated
	}
	return err
}
