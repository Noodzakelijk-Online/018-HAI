package safety

import (
	"strings"
	"testing"
)

func TestRedactSecretsRemovesCommonSecretValues(t *testing.T) {
	input := "password=hunter2 token: abcdefghij Authorization: Bearer secret-token-value"
	output := RedactSecrets(input)

	for _, leaked := range []string{"hunter2", "abcdefghij", "secret-token-value"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("redacted output leaked %q: %s", leaked, output)
		}
	}
	if strings.Count(output, "[REDACTED]") < 2 {
		t.Fatalf("expected redaction markers in %q", output)
	}
}

func TestRedactURLRemovesUserInfoAndSensitiveQueryValues(t *testing.T) {
	output := RedactURL("https://user:pass@example.com/run?token=abc123456&project=hai&api_key=secret-key")

	for _, leaked := range []string{"user:pass", "abc123456", "secret-key"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("redacted URL leaked %q: %s", leaked, output)
		}
	}
	if !strings.Contains(output, "project=hai") {
		t.Fatalf("non-sensitive query value should remain: %s", output)
	}
}
