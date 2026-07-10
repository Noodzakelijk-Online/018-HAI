package idempotency

import (
	"encoding/json"
	"testing"
)

func TestCanonicalJSONStableKeyOrdering(t *testing.T) {
	a, err := CanonicalJSONString(map[string]any{"b": 1, "a": 2, "c": map[string]any{"z": 1, "y": 2}})
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	b, _ := CanonicalJSONString(map[string]any{"c": map[string]any{"y": 2, "z": 1}, "a": 2, "b": 1})
	if a != b {
		t.Fatalf("canonical JSON must be key-order independent:\n%s\n%s", a, b)
	}
	want := `{"a":2,"b":1,"c":{"y":2,"z":1}}`
	if a != want {
		t.Fatalf("canonical = %s, want %s", a, want)
	}
}

func TestCanonicalJSONPreservesLargeNumbers(t *testing.T) {
	// A big integer must not be corrupted by float64 rounding.
	got, err := CanonicalJSONString(map[string]any{"n": json.Number("123456789012345678")})
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if got != `{"n":123456789012345678}` {
		t.Fatalf("large number corrupted: %s", got)
	}
}

func TestActionPayloadHashIsOrderIndependentAndBinds(t *testing.T) {
	h1, err := ActionPayloadHash(map[string]any{"to": "x", "body": "hi"}, "rev1", "gmail", "send")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	h2, _ := ActionPayloadHash(map[string]any{"body": "hi", "to": "x"}, "rev1", "gmail", "send")
	if h1 != h2 {
		t.Fatalf("payload hash must be key-order independent")
	}
	// A changed source revision -> different hash (stale payload fails).
	h3, _ := ActionPayloadHash(map[string]any{"to": "x", "body": "hi"}, "rev2", "gmail", "send")
	if h1 == h3 {
		t.Fatalf("payload hash must incorporate source revision")
	}
	// A changed target system -> different hash.
	h4, _ := ActionPayloadHash(map[string]any{"to": "x", "body": "hi"}, "rev1", "trello", "send")
	if h1 == h4 {
		t.Fatalf("payload hash must incorporate target system")
	}
}

func TestDedupeKeysAreDeterministicAndDistinct(t *testing.T) {
	k1 := OperationDedupeKey("ws1", "email", "gmail", "id1", "rev1")
	k2 := OperationDedupeKey("ws1", "email", "gmail", "id1", "rev1")
	if k1 != k2 {
		t.Fatalf("operation dedupe key must be deterministic")
	}
	if OperationDedupeKey("ws2", "email", "gmail", "id1", "rev1") == k1 {
		t.Fatalf("different workspace must yield a different dedupe key")
	}
	if FeedItemDedupeKey("gmail", "acct", "id1", "rev1") == FeedItemDedupeKey("gmail", "acct", "id1", "rev2") {
		t.Fatalf("revision change must change feed dedupe key")
	}
	if SourceRevisionHash("a", "b") == SourceRevisionHash("ab", "") {
		t.Fatalf("delimiter must prevent boundary collision")
	}
}
