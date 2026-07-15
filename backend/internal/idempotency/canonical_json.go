package idempotency

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalJSON produces a deterministic JSON encoding of v (§10.10):
//   - recursively stable (alphabetical) map key ordering;
//   - original number formatting preserved (json.Number, no float precision loss);
//   - no insignificant whitespace;
//   - deterministic null handling.
//
// It is the basis for dedupe keys and action payload hashing, so the same
// logical payload always hashes to the same value regardless of key order.
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonicaljson: marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("canonicaljson: decode: %w", err)
	}
	var buf bytes.Buffer
	if err := encodeCanonical(&buf, decoded); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CanonicalJSONString is CanonicalJSON returning a string.
func CanonicalJSONString(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func encodeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := encodeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		// scalars: string, bool, json.Number, nil — json.Marshal is canonical.
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("canonicaljson: encode scalar: %w", err)
		}
		buf.Write(b)
		return nil
	}
}
