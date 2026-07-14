package googleoauth

import (
	"strings"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	codec, err := NewCodec("a-configured-encryption-secret")
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	secret := "1//refresh-token-value-that-must-stay-private"
	sealed, err := codec.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(string(sealed), secret) {
		t.Fatal("ciphertext must not contain the plaintext token")
	}
	got, err := codec.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip = %q, want %q", got, secret)
	}
}

func TestCodecEmptyRoundTrips(t *testing.T) {
	codec, _ := NewCodec("secret")
	sealed, err := codec.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if len(sealed) != 0 {
		t.Fatalf("empty plaintext should seal to empty, got %d bytes", len(sealed))
	}
	got, err := codec.Decrypt(sealed)
	if err != nil || got != "" {
		t.Fatalf("empty decrypt = %q, %v", got, err)
	}
}

func TestCodecRejectsEmptySecret(t *testing.T) {
	if _, err := NewCodec("   "); err == nil {
		t.Fatal("an empty secret must be rejected, not used as a zero key")
	}
}

func TestCodecWrongKeyFails(t *testing.T) {
	a, _ := NewCodec("secret-one")
	b, _ := NewCodec("secret-two")
	sealed, _ := a.Encrypt("token")
	if _, err := b.Decrypt(sealed); err == nil {
		t.Fatal("decrypting with a different key must fail, not return garbage")
	}
}

func TestCodecTamperedCiphertextFails(t *testing.T) {
	codec, _ := NewCodec("secret")
	sealed, _ := codec.Encrypt("token")
	sealed[len(sealed)-1] ^= 0xff // flip a bit in the auth tag
	if _, err := codec.Decrypt(sealed); err == nil {
		t.Fatal("GCM must reject tampered ciphertext")
	}
}
