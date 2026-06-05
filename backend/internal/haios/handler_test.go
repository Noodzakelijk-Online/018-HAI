package haios

import (
	"strings"
	"testing"

	"automation-hub-backend/internal/llm"
)

func TestLiveProviderConfiguredIgnoresApprovalGatedRuntimeOnlyProvider(t *testing.T) {
	policy := llm.Policy{
		LocalModelsAllowed: true,
		Providers: []llm.Provider{
			{
				ID:         "odysseus",
				Name:       "Odysseus AI Workspace",
				Enabled:    true,
				Configured: true,
				Local:      true,
				Models: []llm.Model{
					{ID: "odysseus-workspace-agent", Enabled: true, Tier: llm.TierFree, RequiresApproval: true},
				},
			},
		},
	}

	if liveProviderConfigured(policy) {
		t.Fatalf("probe-only or approval-gated runtime provider should not satisfy executable LLM readiness")
	}
	evidence := liveProviderEvidence(policy)
	if !strings.Contains(evidence, "not executable") || !strings.Contains(evidence, "Odysseus AI Workspace") {
		t.Fatalf("evidence = %q, want non-executable Odysseus explanation", evidence)
	}
}

func TestLiveProviderConfiguredAcceptsNoApprovalLocalModel(t *testing.T) {
	policy := llm.Policy{
		LocalModelsAllowed: true,
		Providers: []llm.Provider{
			{
				ID:         "ollama",
				Name:       "Ollama local",
				Enabled:    true,
				Configured: true,
				Local:      true,
				Models: []llm.Model{
					{ID: "qwen2.5-coder:7b", Enabled: true, Tier: llm.TierFree},
				},
			},
		},
	}

	if !liveProviderConfigured(policy) {
		t.Fatalf("configured local no-approval model should satisfy executable LLM readiness")
	}
	if evidence := liveProviderEvidence(policy); !strings.Contains(evidence, "Ollama local") {
		t.Fatalf("evidence = %q, want executable provider name", evidence)
	}
}

func TestLiveProviderConfiguredIgnoresFreeCloudWithoutQuota(t *testing.T) {
	policy := llm.Policy{
		FreeCloudQuotaAllowed: true,
		Providers: []llm.Provider{
			{
				ID:             "free-cloud",
				Name:           "Configured free cloud quota",
				Enabled:        true,
				Configured:     true,
				Local:          false,
				Paid:           false,
				QuotaRemaining: 0,
				Models: []llm.Model{
					{ID: "free-best-available", Enabled: true, Tier: llm.TierFree},
				},
			},
		},
	}

	if liveProviderConfigured(policy) {
		t.Fatalf("free-cloud provider with exhausted or unknown quota should not satisfy executable readiness")
	}
	if evidence := liveProviderEvidence(policy); !strings.Contains(evidence, "quota") {
		t.Fatalf("evidence = %q, want quota explanation", evidence)
	}
}
