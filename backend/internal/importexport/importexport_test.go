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

func TestWrapRequiresPositiveVersion(t *testing.T) {
	if _, err := Wrap("018-hai-memories", 0, sample{}); err == nil {
		t.Fatalf("non-positive version must be rejected")
	}
}

func TestUnwrapRequiresExpectedFormatAndValidVersion(t *testing.T) {
	if _, err := Unwrap([]byte(`{"format":"x","version":1,"payload":{}}`), " "); err == nil {
		t.Fatalf("empty expected format must be rejected")
	}
	if _, err := Unwrap([]byte(`{"format":"x","version":0,"payload":{}}`), "x"); err == nil {
		t.Fatalf("non-positive envelope version must be rejected")
	}
}

func TestUnwrapRejectsEmptyAndOversizedInput(t *testing.T) {
	if _, err := Unwrap(nil, "x"); err == nil {
		t.Fatalf("empty envelope must be rejected")
	}
	if _, err := Unwrap(make([]byte, maxEnvelopeBytes+1), "x"); err == nil {
		t.Fatalf("oversized envelope must be rejected")
	}
}
