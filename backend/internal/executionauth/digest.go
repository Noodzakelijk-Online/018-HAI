package executionauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func digest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func requestDigest(request Request) (string, error) {
	// RequestedAt is operational metadata, not action identity. Excluding it
	// makes a caller retry with the same idempotency key deterministic.
	request.RequestedAt = time.Time{}
	return digest(struct {
		ContractVersion int     `json:"contractVersion"`
		Request         Request `json:"request"`
	}{
		ContractVersion: ContractVersion,
		Request:         request,
	})
}

func finishDigest(receipt Receipt) (string, error) {
	receipt.DecisionDigest = ""
	receipt.LifeGraphProjection = nil
	receipt.LifeGraphProjectionWarning = ""
	value, err := digest(receipt)
	if err != nil {
		return "", fmt.Errorf("digest authorization receipt: %w", err)
	}
	return value, nil
}
