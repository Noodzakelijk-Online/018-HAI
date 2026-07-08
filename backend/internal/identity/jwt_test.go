package identity

import (
	"testing"
	"time"
)

const secret = "test-secret"

func TestVerifyValidToken(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := SignToken(Claims{Subject: "user-1", Role: "operator", Expiry: now.Add(time.Hour).Unix()}, secret)
	claims, err := Verify(tok, secret, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.Role != "operator" {
		t.Fatalf("claims wrong: %+v", claims)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	now := time.Now()
	tok := SignToken(Claims{Subject: "u", Role: "owner"}, secret)
	if _, err := Verify(tok, "other-secret", now); err != ErrSignature {
		t.Fatalf("wrong secret should fail with ErrSignature, got %v", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := SignToken(Claims{Subject: "u", Role: "owner", Expiry: now.Add(-time.Minute).Unix()}, secret)
	if _, err := Verify(tok, secret, now); err != ErrExpired {
		t.Fatalf("expired token should fail with ErrExpired, got %v", err)
	}
}

func TestVerifyRejectsNonHS256(t *testing.T) {
	// A token with alg:none must be rejected even if the rest is well-formed.
	// header {"alg":"none","typ":"JWT"} base64url:
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1Iiwicm9sZSI6Im93bmVyIn0."
	if _, err := Verify(noneToken, secret, time.Now()); err != ErrAlgorithm {
		t.Fatalf("alg:none must be rejected with ErrAlgorithm, got %v", err)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "a.b", "a.b.c.d", "not-a-token"} {
		if _, err := Verify(bad, secret, time.Now()); err == nil {
			t.Fatalf("malformed token %q should fail", bad)
		}
	}
}
