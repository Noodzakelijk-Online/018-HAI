package checkpoint

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	c := Checkpoint{Task: "goal-run", Phase: "022", Step: 3, Note: "in progress"}
	data, err := c.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Task != "goal-run" || got.Phase != "022" || got.Step != 3 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestEncodeRequiresTask(t *testing.T) {
	if _, err := (Checkpoint{Phase: "x"}).Encode(); err == nil {
		t.Fatalf("encode without task should error")
	}
	if _, err := Decode([]byte(`{"phase":"x"}`)); err == nil {
		t.Fatalf("decode without task should error")
	}
}

func TestMarkCompleteIsIdempotentAndPure(t *testing.T) {
	c := Checkpoint{Task: "t"}
	c1 := c.MarkComplete("a")
	c2 := c1.MarkComplete("a") // duplicate ignored
	if len(c2.Completed) != 1 {
		t.Fatalf("duplicate should be ignored: %+v", c2.Completed)
	}
	if len(c.Completed) != 0 {
		t.Fatalf("original checkpoint must not be mutated")
	}
}
