package outcomeevaluation

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func hashValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("hash outcome evaluation: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func evaluationDigest(value Evaluation) (string, error) {
	value.AuditDigest = ""
	return hashValue(value)
}

// VerifyAuditDigest detects mutation of an evaluation and also enforces the
// advisory-only recommendation contract.
func VerifyAuditDigest(value Evaluation) error {
	if err := value.ValidateNoAuthority(); err != nil {
		return err
	}
	expected, err := evaluationDigest(value)
	if err != nil {
		return err
	}
	provided, err := hex.DecodeString(value.AuditDigest)
	if err != nil || len(provided) != sha256.Size {
		return ErrIntegrityViolation
	}
	expectedBytes, _ := hex.DecodeString(expected)
	if subtle.ConstantTimeCompare(provided, expectedBytes) != 1 {
		return ErrIntegrityViolation
	}
	return nil
}

func equalSHA256(provided, expected string) bool {
	providedBytes, providedErr := hex.DecodeString(provided)
	expectedBytes, expectedErr := hex.DecodeString(expected)
	if providedErr != nil || expectedErr != nil || len(providedBytes) != sha256.Size || len(expectedBytes) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(providedBytes, expectedBytes) == 1
}
