package importexport

import "testing"

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWrapUnwrapRoundTrip(t *testing.T) {
	data, err := Wrap("018-hai-memories", 1, sample{Name: "x", Count: 3})
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	env, err := Unwrap(data, "018-hai-memories")
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if env.Version != 1 {
		t.Fatalf("version = %d, want 1", env.Version)
	}
	var out sample
	if err := env.DecodePayload(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Name != "x" || out.Count != 3 {
		t.Fatalf("payload round-trip wrong: %+v", out)
	}
}

func TestUnwrapRejectsFormatMismatch(t *testing.T) {
	data, _ := Wrap("format-a", 1, sample{})
	if _, err := Unwrap(data, "format-b"); err == nil {
		t.Fatalf("mismatched format must be rejected")
	}
}

func TestWrapRequiresFormat(t *testing.T) {
	if _, err := Wrap("  ", 1, sample{}); err == nil {
		t.Fatalf("empty format must be rejected")
	}
}
