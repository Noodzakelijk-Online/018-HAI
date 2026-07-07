// Package importexport provides a versioned envelope for importing and exporting
// data safely. Wrapping stamps a format tag and version; unwrapping refuses a
// mismatched format so data from a different system can never be silently
// imported.
package importexport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Envelope is the on-the-wire container for exported data.
type Envelope struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

// Wrap serializes payload inside a versioned, format-tagged envelope.
func Wrap(format string, version int, payload any) ([]byte, error) {
	if strings.TrimSpace(format) == "" {
		return nil, fmt.Errorf("format is required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return json.Marshal(Envelope{Format: format, Version: version, Payload: raw})
}

// Unwrap parses an envelope and verifies its format matches expectedFormat.
func Unwrap(data []byte, expectedFormat string) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("invalid envelope: %w", err)
	}
	if env.Format != expectedFormat {
		return Envelope{}, fmt.Errorf("format mismatch: got %q, want %q", env.Format, expectedFormat)
	}
	return env, nil
}

// DecodePayload unmarshals the envelope payload into target.
func (e Envelope) DecodePayload(target any) error {
	return json.Unmarshal(e.Payload, target)
}
