package memoryengine

import (
	"automation-hub-backend/internal/models"
	"strings"
	"testing"
)

func TestNormalizeImportRejectsMismatchedPlatformHost(t *testing.T) {
	_, _, _, err := normalizeImport(ImportRequest{
		Platform:  "chatgpt",
		SourceURI: "https://gemini.google.com/app/example",
		Messages:  []ChatMessage{{Role: "user", Content: "Build the dashboard."}},
	})
	if err == nil {
		t.Fatalf("mismatched platform host was accepted")
	}
}

func TestNormalizeImportProducesStableHash(t *testing.T) {
	request := ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-1",
		Title:      "Dashboard",
		SourceURI:  "https://chatgpt.com/c/thread-1",
		Messages: []ChatMessage{
			{Role: "user", Content: "We need to build the dashboard."},
			{Role: "assistant", Content: "Decision: use a local-first architecture."},
		},
	}
	_, _, first, err := normalizeImport(request)
	if err != nil {
		t.Fatalf("normalize first: %v", err)
	}
	_, _, second, err := normalizeImport(request)
	if err != nil {
		t.Fatalf("normalize second: %v", err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("hash is not stable: %q/%q", first, second)
	}
}

func TestExtractInsightsRedactsSecretsAndClassifiesActions(t *testing.T) {
	conversation := models.AIConversationArchive{
		Platform:  "chatgpt",
		Title:     "HAI",
		SourceURI: "https://chatgpt.com/c/thread-1",
	}
	insights := extractInsights(conversation, ImportRequest{
		ProjectKey: "018-HAI",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Action: VA should create the Trello card. Never store password=secret-value in memory.",
		}},
	})
	if len(insights) != 2 {
		t.Fatalf("insights = %#v, want action and rule", insights)
	}
	for _, insight := range insights {
		if strings.Contains(insight.Text, "secret-value") {
			t.Fatalf("secret was retained in operational insight: %q", insight.Text)
		}
	}
	if insights[0].Kind != "action" || insights[0].Owner != "VA" || insights[0].ProjectKey != "018-HAI" {
		t.Fatalf("action insight = %#v", insights[0])
	}
}

func TestEncryptedPayloadRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte(`{"messages":[{"role":"user","content":"private"}]}`)
	ciphertext, nonce, err := encryptPayload(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(string(ciphertext), "private") {
		t.Fatalf("plaintext leaked into ciphertext")
	}
	decrypted, err := decryptPayload(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("round trip = %q, want %q", decrypted, plaintext)
	}
}
