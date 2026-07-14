package privacyfilter

import (
	"strings"
	"testing"
)

func hasField(r ScanResult, field string) bool {
	for _, f := range r.SensitiveFields {
		if f == field {
			return true
		}
	}
	return false
}

func TestScannerRedactsSecretsAndFlagsCloudUnsafe(t *testing.T) {
	cases := map[string]string{
		"email":       "contact me at john.doe@example.com please",
		"phone":       "call +31 6 12345678 tomorrow",
		"jwt":         "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.abcDEF123",
		"bearer_token": "Authorization: Bearer abcdef1234567890xyz",
		"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIB\n-----END RSA PRIVATE KEY-----",
	}
	for field, content := range cases {
		r := Scan(content, 200)
		if !hasField(r, field) {
			t.Fatalf("expected %s detected in %q, got %+v", field, content, r.SensitiveFields)
		}
		if strings.Contains(r.RedactedPreview, "eyJ") || strings.Contains(r.RedactedPreview, "PRIVATE KEY") {
			// secrets must be redacted out of the preview
			t.Fatalf("secret leaked into preview for %s: %q", field, r.RedactedPreview)
		}
	}
	// Secret content must be cloud-unsafe.
	if Scan("api_key = ABCD1234EFGH5678", 200).SafeForCloudModel {
		t.Fatalf("secret content must be marked unsafe for cloud")
	}
}

func TestScannerRiskEscalationAndReviewFlag(t *testing.T) {
	// IBAN is high risk -> requires review before external provider, not safe for memory.
	r := Scan("payment to NL91ABNA0417164300 now", 200)
	if r.PrivacyRiskLevel != RiskHigh {
		t.Fatalf("IBAN should be high risk, got %s", r.PrivacyRiskLevel)
	}
	if !r.RequiresReviewBeforeExternalProvider || r.SafeForMemory {
		t.Fatalf("high risk must require review and be memory-unsafe: %+v", r)
	}
	// Clean content -> none.
	clean := Scan("summarize the quarterly planning notes", 200)
	if clean.PrivacyRiskLevel != RiskNone || clean.RedactionApplied || !clean.SafeForCloudModel {
		t.Fatalf("clean content should be low-privacy: %+v", clean)
	}
}

func TestScannerPreviewIsBounded(t *testing.T) {
	long := strings.Repeat("word ", 200)
	r := Scan(long, 50)
	if len(r.RedactedPreview) > 50 {
		t.Fatalf("preview must be bounded to 50, got %d", len(r.RedactedPreview))
	}
}
