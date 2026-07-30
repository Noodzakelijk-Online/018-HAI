package presidio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAnalyzeUsesFixedLocalEndpointAndDoesNotReturnText(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if got, want := request.URL.String(), "http://127.0.0.1:3000/analyze"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := payload["entities"].([]any); len(got) != 2 || got[0] != "EMAIL_ADDRESS" || got[1] != "PERSON" {
			t.Fatalf("entity allowlist = %#v", got)
		}
		if payload["language"] != "en" || payload["text"] != "Email Ada at ada@example.com" {
			t.Fatalf("payload = %#v", payload)
		}
		body := `[{"entity_type":"EMAIL_ADDRESS","start":13,"end":28,"score":0.91},{"entity_type":"CREDIT_CARD","start":0,"end":0,"score":0.9}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:3000", "en", "EMAIL_ADDRESS,PERSON", client)
	result, err := service.Analyze(context.Background(), Request{Text: "Email Ada at ada@example.com"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.EntityCount != 1 || result.Entities[0].Type != "EMAIL_ADDRESS" || result.Entities[0].Start != 13 || result.Entities[0].End != 28 {
		t.Fatalf("result = %#v", result)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "ada@example.com") {
		t.Fatalf("analysis response must not echo text: %s", encoded)
	}
}

func TestLocalConfigRejectsUnsafeOrIncompleteConfiguration(t *testing.T) {
	for _, config := range []struct{ endpoint, language, entities string }{
		{"https://example.com", "en", "EMAIL_ADDRESS"},
		{"https://user:secret@localhost:3000", "en", "EMAIL_ADDRESS"},
		{"https://localhost:3000/?token=secret", "en", "EMAIL_ADDRESS"},
		{"http://8.8.8.8:3000", "en", "EMAIL_ADDRESS"},
		{"http://127.0.0.1:3000", "english", "EMAIL_ADDRESS"},
		{"http://127.0.0.1:3000", "en", ""},
		{"http://127.0.0.1:3000", "en", "email address"},
	} {
		service := NewService(true, config.endpoint, config.language, config.entities, nil)
		if service.Status().Configured {
			t.Fatalf("unsafe config unexpectedly enabled: %#v", config)
		}
	}
}

func TestAnalysisIsDisabledByDefault(t *testing.T) {
	service := NewService(false, "", "", "", nil)
	if _, err := service.Analyze(context.Background(), Request{Text: "anything"}); err != ErrNotConfigured {
		t.Fatalf("Analyze error = %v, want %v", err, ErrNotConfigured)
	}
}

func TestAnalyzeValidatesPresidioCharacterOffsetsForUnicodeText(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(_ *http.Request) (*http.Response, error) {
		body := `[{"entity_type":"PERSON","start":0,"end":2,"score":0.9}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:3000", "en", "PERSON", client)
	result, err := service.Analyze(context.Background(), Request{Text: "Åsa"})
	if err != nil || result.EntityCount != 1 {
		t.Fatalf("unicode Presidio offsets must use characters, result=%#v err=%v", result, err)
	}
}
