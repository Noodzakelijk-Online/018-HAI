package resilience

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"
)

// EventHash returns a deterministic SHA-256 hash over a validated event. Map
// order and time-zone representation do not affect the result.
func EventHash(event ControlEvent) (string, error) {
	if err := validateControlEvent(event); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(event.Attributes))
	for key := range event.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	writeHashField(h, "resilience-event/v1")
	writeHashField(h, event.Scope.OwnerID)
	writeHashField(h, event.Scope.WorkspaceID)
	writeHashField(h, event.Type)
	writeHashField(h, event.SubjectID)
	writeHashField(h, event.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	writeHashField(h, fmt.Sprintf("%d", event.Sequence))
	writeHashField(h, event.PreviousHash)
	for _, key := range keys {
		writeHashField(h, key)
		writeHashField(h, event.Attributes[key])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validateControlEvent(event ControlEvent) error {
	if err := validateContract(event.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(event.Scope); err != nil {
		return err
	}
	if err := validateID("event type", event.Type); err != nil {
		return err
	}
	if err := validateID("event subject id", event.SubjectID); err != nil {
		return err
	}
	if err := validateTime("event occurrence time", event.OccurredAt); err != nil {
		return err
	}
	if event.Sequence == 0 {
		return fmt.Errorf("resilience: event sequence must be positive")
	}
	if err := validateHash("previous event hash", event.PreviousHash, true); err != nil {
		return err
	}
	if len(event.Attributes) > 64 {
		return fmt.Errorf("resilience: event attribute count exceeds 64")
	}
	for key, value := range event.Attributes {
		if key != strings.TrimSpace(key) {
			return fmt.Errorf("resilience: event attribute key must be canonical")
		}
		if err := validateID("event attribute key", key); err != nil {
			return err
		}
		if secretKeyPattern.MatchString(key) {
			return fmt.Errorf("resilience: event attribute key is sensitive")
		}
		if len(value) > 2000 || containsControl(value) || containsSecret(value) {
			return fmt.Errorf("resilience: event attribute value must be bounded and secret-free")
		}
	}
	return nil
}

func hashFields(fields ...string) string {
	h := sha256.New()
	for _, field := range fields {
		writeHashField(h, field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeHashField(h hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.Write([]byte(value))
}
