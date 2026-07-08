// Package identity verifies IDP-issued HS256 JSON Web Tokens using only the
// standard library, and exposes the caller's identity + role. This is how a
// per-user identity (from the IDP, signed with the shared JWT secret) is mapped
// to an RBAC role in the backend — no third-party JWT dependency required.
package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("identity: malformed token")
	ErrAlgorithm = errors.New("identity: unexpected signing algorithm")
	ErrSignature = errors.New("identity: invalid signature")
	ErrExpired   = errors.New("identity: token expired")
)

// Claims are the subset of JWT claims the backend cares about.
type Claims struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	Expiry  int64  `json:"exp,omitempty"`
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Verify validates an HS256 JWT against secret at time now and returns its
// claims. It rejects any algorithm other than HS256 (blocking "alg: none" and
// algorithm-confusion attacks), a bad signature, and expired tokens.
func Verify(token, secret string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var h header
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		return Claims{}, ErrMalformed
	}
	if h.Alg != "HS256" {
		return Claims{}, ErrAlgorithm
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	if !hmac.Equal(sig, sign(parts[0]+"."+parts[1], secret)) {
		return Claims{}, ErrSignature
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return Claims{}, ErrMalformed
	}
	if claims.Expiry != 0 && now.Unix() >= claims.Expiry {
		return Claims{}, ErrExpired
	}
	return claims, nil
}

// SignToken builds an HS256 JWT for the given claims. Production tokens are
// issued by the IDP; this is used by tests and any trusted local issuer.
func SignToken(claims Claims, secret string) string {
	headerPart := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := headerPart + "." + payloadPart
	sigPart := base64.RawURLEncoding.EncodeToString(sign(signingInput, secret))
	return signingInput + "." + sigPart
}

func sign(input, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	return mac.Sum(nil)
}
