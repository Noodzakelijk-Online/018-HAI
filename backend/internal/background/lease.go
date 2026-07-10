package background

import "sync"

// lease serializes background runs so only one RunOnce executes at a time
// within a process (§10.16). It is a non-blocking try-lock.
type lease struct {
	mu   sync.Mutex
	held bool
}

func newLease() *lease { return &lease{} }

// acquire returns true if the caller took the lease, false if already held.
func (l *lease) acquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return false
	}
	l.held = true
	return true
}

func (l *lease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held = false
}
