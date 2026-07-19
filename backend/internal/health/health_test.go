package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMProviderProbeUsesConfiguredLocalAIEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("path = %q, want root probe", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("LLAMA_CPP_BASE_URL", "")
	t.Setenv("LOCALAI_BASE_URL", server.URL)
	t.Setenv("LITELLM_ENABLED", "false")
	t.Setenv("LITELLM_BASE_URL", "")
	t.Setenv("FREE_CLOUD_OPENAI_BASE_URL", "")

	if err := LLMProviderProbe().Run(context.Background()); err != nil {
		t.Fatalf("LocalAI health probe failed: %v", err)
	}
}

func TestLLMProviderProbeUsesConfiguredVLLMEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("LLAMA_CPP_BASE_URL", "")
	t.Setenv("LOCALAI_BASE_URL", "")
	t.Setenv("VLLM_BASE_URL", server.URL)
	t.Setenv("LITELLM_ENABLED", "false")
	t.Setenv("LITELLM_BASE_URL", "")
	t.Setenv("FREE_CLOUD_OPENAI_BASE_URL", "")

	if err := LLMProviderProbe().Run(context.Background()); err != nil {
		t.Fatalf("vLLM health probe failed: %v", err)
	}
}

func TestLLMProviderProbeUsesConfiguredMistralRSEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("LLAMA_CPP_BASE_URL", "")
	t.Setenv("LOCALAI_BASE_URL", "")
	t.Setenv("VLLM_BASE_URL", "")
	t.Setenv("MISTRAL_RS_BASE_URL", server.URL)
	t.Setenv("LITELLM_ENABLED", "false")
	t.Setenv("LITELLM_BASE_URL", "")
	t.Setenv("FREE_CLOUD_OPENAI_BASE_URL", "")

	if err := LLMProviderProbe().Run(context.Background()); err != nil {
		t.Fatalf("mistral.rs health probe failed: %v", err)
	}
}
