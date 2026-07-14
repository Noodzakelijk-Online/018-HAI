package googleoauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
)

// Codec encrypts OAuth tokens at rest with AES-256-GCM. OAuth refresh tokens are
// long-lived credentials to a user's mailbox, so they must never sit in the
// database in plaintext. The key is derived from the configured encryption
// secret the same way the memory engine derives its key (SHA-256), so a single
// configured secret protects both.
type Codec struct {
	key []byte
}

// NewCodec derives an AES-256 key from secret. It returns an error for an empty
// secret rather than silently using a zero key, so misconfiguration fails loudly.
func NewCodec(secret string) (*Codec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("token encryption secret is not configured")
	}
	sum := sha256.Sum256([]byte(secret))
	return &Codec{key: sum[:]}, nil
}

// Encrypt returns a self-describing ciphertext (nonce prefixed) so Decrypt needs
// only the value and the key. An empty plaintext encrypts to an empty string, so
// an absent refresh token round-trips cleanly.
func (c *Codec) Encrypt(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return []byte{}, nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// nonce || ciphertext
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt reverses Encrypt.
func (c *Codec) Decrypt(sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("token decryption failed (wrong key or corrupt data): %w", err)
	}
	return string(plaintext), nil
}
