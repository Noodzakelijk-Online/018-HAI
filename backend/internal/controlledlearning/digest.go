package controlledlearning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func digestValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode controlled learning digest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
