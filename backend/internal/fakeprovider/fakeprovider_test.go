package fakeprovider

import (
	"errors"
	"testing"
)

func TestSucceedsByDefault(t *testing.T) {
	p := New("local", "hello")
	out, err := p.Generate("hi")
	if err != nil || out != "hello" {
		t.Fatalf("default should succeed: %q %v", out, err)
	}
}

func TestAlwaysFail(t *testing.T) {
	p := New("local", "hi").AlwaysFail(nil)
	if _, err := p.Generate("x"); !errors.Is(err, ErrSimulated) {
		t.Fatalf("expected simulated error, got %v", err)
	}
}

func TestFailAfterNCalls(t *testing.T) {
	custom := errors.New("boom")
	p := New("local", "ok").FailAfter(2, custom)
	if _, err := p.Generate("1"); err != nil {
		t.Fatalf("call 1 should succeed")
	}
	if _, err := p.Generate("2"); err != nil {
		t.Fatalf("call 2 should succeed")
	}
	if _, err := p.Generate("3"); !errors.Is(err, custom) {
		t.Fatalf("call 3 should fail with custom error, got %v", err)
	}
	if p.Calls() != 3 {
		t.Fatalf("calls = %d, want 3", p.Calls())
	}
}
