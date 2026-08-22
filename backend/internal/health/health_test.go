package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/config"
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

func TestRedisProbeExplainsInProcessFallbackWhenSharedRateLimitIsEnabled(t *testing.T) {
	err := RedisProbe(config.Configuration{RateLimitPerMinute: 60}).Run(context.Background())
	if err == nil {
		t.Fatal("RedisProbe error = nil, want an explicit local-rate-limit diagnostic")
	}
	if !strings.Contains(err.Error(), "per-process in-memory") {
		t.Fatalf("RedisProbe error = %q, want a per-process fallback explanation", err)
	}
}
