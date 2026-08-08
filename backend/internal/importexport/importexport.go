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

const maxEnvelopeBytes = 64 << 20

// Envelope is the on-the-wire container for exported data.
type Envelope struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

// Wrap serializes a caller-provided payload in memory. It performs no
// filesystem, database, network, or data-loading effect; the owning export
// service must authorize before calling it.
func Wrap(format string, version int, payload any) ([]byte, error) {
	if strings.TrimSpace(format) == "" {
		return nil, fmt.Errorf("format is required")
	}
	if version < 1 {
		return nil, fmt.Errorf("version must be positive")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxEnvelopeBytes {
		return nil, fmt.Errorf("payload size is invalid")
	}
	encoded, err := json.Marshal(Envelope{
		Format:  strings.TrimSpace(format),
		Version: version,
		Payload: raw,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	if len(encoded) > maxEnvelopeBytes {
		return nil, fmt.Errorf("envelope is too large")
	}
	return encoded, nil
}

// Unwrap parses an envelope and verifies its format matches expectedFormat.
func Unwrap(data []byte, expectedFormat string) (Envelope, error) {
	expectedFormat = strings.TrimSpace(expectedFormat)
	if expectedFormat == "" {
		return Envelope{}, fmt.Errorf("expected format is required")
	}
	if len(data) == 0 || len(data) > maxEnvelopeBytes {
		return Envelope{}, fmt.Errorf("envelope size is invalid")
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("invalid envelope: %w", err)
	}
	if env.Format != expectedFormat {
		return Envelope{}, fmt.Errorf("format mismatch: got %q, want %q", env.Format, expectedFormat)
	}
	if env.Version < 1 {
		return Envelope{}, fmt.Errorf("envelope version is invalid")
	}
	if len(env.Payload) == 0 || len(env.Payload) > maxEnvelopeBytes {
		return Envelope{}, fmt.Errorf("envelope payload size is invalid")
	}
	return env, nil
}

// DecodePayload unmarshals the envelope payload into target.
func (e Envelope) DecodePayload(target any) error {
	return json.Unmarshal(e.Payload, target)
}
