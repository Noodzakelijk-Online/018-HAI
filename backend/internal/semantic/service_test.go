package semantic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateLocalURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8080", "http://host.docker.internal:8080", "https://localhost/v1"} {
		if err := validateLocalURL(raw); err != nil {
			t.Fatalf("%s should be allowed: %v", raw, err)
		}
	}
	for _, raw := range []string{"https://api.example.com", "http://169.254.169.254", "http://user:secret@localhost"} {
		if err := validateLocalURL(raw); err == nil {
			t.Fatalf("%s should be rejected", raw)
		}
	}
}

func TestVectorLiteralAndInputTrimming(t *testing.T) {
	if got := vectorLiteral([]float64{1, -0.5, 0.25}); got != "[1,-0.5,0.25]" {
		t.Fatalf("vector literal = %q", got)
	}
	if got := trimRunes("abcdef", 3); got != "abc" {
		t.Fatalf("trimmed input = %q", got)
	}

}

func TestEmbedUsesLocalEndpointAndBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer local-key" {
			t.Fatalf("unexpected embedding request: %s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.25,0.75]}]}`))
	}))
	defer server.Close()

	service := &service{config: Config{BaseURL: server.URL, Model: "local-embed", APIKey: "local-key", InputLimit: 12000}, client: server.Client()}
	vector, err := service.embed(context.Background(), "source text")
	if err != nil || len(vector) != 2 || vector[0] != 0.25 {
		t.Fatalf("embedding = %#v, %v", vector, err)
	}
}
