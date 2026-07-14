package modelintelligence

import (
	"context"
	"strings"
	"time"
)

// Test-only providers perform genuine, deterministic, local work (classification
// and verification). They are honestly labeled test-only: they never claim to be
// a real cloud provider and never reach the network. They exist so model
// intelligence and lane routing are useful without any real provider configured
// (§10.17: "Model intelligence must be useful without real providers.").

const (
	// ProviderTestFastTriage is a deterministic local triage classifier.
	ProviderTestFastTriage = "test-fast-triage"
	// ProviderTestVerifier is a deterministic local verifier.
	ProviderTestVerifier = "test-verifier"
)

// deterministicTelemetry fills token + tokens/sec telemetry from output size
// without any wall-clock dependency, so results are reproducible.
func deterministicTelemetry(res *InferenceResult, in, out string) {
	res.InputTokensEstimate = estimateTokens(in)
	res.OutputTokensEstimate = estimateTokens(out)
	// A local deterministic provider is modeled at ~200 tokens/sec.
	res.TokensPerSecond = 200
	ms := int64(float64(res.OutputTokensEstimate) / res.TokensPerSecond * 1000)
	if ms < 1 {
		ms = 1
	}
	res.DurationMs = ms
}

// testFastTriageProvider classifies/summarizes background items deterministically.
type testFastTriageProvider struct{}

func (p *testFastTriageProvider) ID() string { return ProviderTestFastTriage }
func (p *testFastTriageProvider) DisplayName() string {
	return "Test Fast Triage (local, deterministic)"
}

func (p *testFastTriageProvider) Profiles() []ModelProfile {
	return []ModelProfile{{
		ProviderID:         ProviderTestFastTriage,
		ModelID:            "triage-rules-v1",
		DisplayName:        "Deterministic triage rules",
		ArchitectureFamily: ArchBidirectionalTokenClassify,
		Lanes:              []RoutingLane{LaneFastTriage, LanePrivacyFilter},
		ContextWindow:      8192,
		Local:              true,
		Paid:               false,
		Status:             ProviderActive, // it runs locally and deterministically
		ClaimLevel:         ClaimExercisedLocalSafeTask,
	}}
}

func (p *testFastTriageProvider) Probe(ctx context.Context, now time.Time) ProbeResult {
	return ProbeResult{ProviderID: p.ID(), Status: ProviderActive, ModelsSeen: 1, DurationMs: 1, Detail: "local deterministic provider", CheckedAt: now}
}

func (p *testFastTriageProvider) Generate(ctx context.Context, req InferenceRequest, now time.Time) (InferenceResult, error) {
	category := triageCategory(req.Prompt)
	summary := boundedSummary(req.Prompt, 160)
	out := "category=" + category + "; summary=" + summary
	res := InferenceResult{ProviderID: p.ID(), ModelID: "triage-rules-v1", Lane: req.Lane, Output: out, OK: true}
	deterministicTelemetry(&res, req.Prompt, out)
	return res, nil
}

func triageCategory(text string) string {
	t := strings.ToLower(text)
	switch {
	case containsAny(t, "invoice", "payment", "pay ", "bank", "tax"):
		return "financial"
	case containsAny(t, "legal", "lawyer", "court", "contract", "dispute"):
		return "legal_admin"
	case containsAny(t, "reply", "draft", "email", "message", "follow up", "follow-up"):
		return "communication"
	case containsAny(t, "note", "organize", "summary", "summarize", "cleanup"):
		return "housekeeping"
	default:
		return "general"
	}
}

// testVerifierProvider decides deterministically whether a claim is grounded in
// provided evidence. The convention: the prompt contains "CLAIM: ..." and
// "EVIDENCE: ..."; the claim is grounded only if every non-trivial claim word
// appears in the evidence.
type testVerifierProvider struct{}

func (p *testVerifierProvider) ID() string          { return ProviderTestVerifier }
func (p *testVerifierProvider) DisplayName() string { return "Test Verifier (local, deterministic)" }

func (p *testVerifierProvider) Profiles() []ModelProfile {
	return []ModelProfile{{
		ProviderID:         ProviderTestVerifier,
		ModelID:            "verifier-rules-v1",
		DisplayName:        "Deterministic grounding verifier",
		ArchitectureFamily: ArchBidirectionalTokenClassify,
		Lanes:              []RoutingLane{LaneVerifier},
		ContextWindow:      8192,
		Local:              true,
		Paid:               false,
		Status:             ProviderActive,
		ClaimLevel:         ClaimExercisedLocalSafeTask,
	}}
}

func (p *testVerifierProvider) Probe(ctx context.Context, now time.Time) ProbeResult {
	return ProbeResult{ProviderID: p.ID(), Status: ProviderActive, ModelsSeen: 1, DurationMs: 1, Detail: "local deterministic provider", CheckedAt: now}
}

func (p *testVerifierProvider) Generate(ctx context.Context, req InferenceRequest, now time.Time) (InferenceResult, error) {
	claim, evidence := splitClaimEvidence(req.Prompt)
	grounded := isGrounded(claim, evidence)
	verdict := "not_grounded"
	if grounded {
		verdict = "grounded"
	}
	out := "verdict=" + verdict
	res := InferenceResult{ProviderID: p.ID(), ModelID: "verifier-rules-v1", Lane: req.Lane, Output: out, OK: true}
	deterministicTelemetry(&res, req.Prompt, out)
	return res, nil
}

func splitClaimEvidence(prompt string) (string, string) {
	claim, evidence := "", ""
	for _, line := range strings.Split(prompt, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(l), "CLAIM:") {
			claim = strings.TrimSpace(l[len("CLAIM:"):])
		} else if strings.HasPrefix(strings.ToUpper(l), "EVIDENCE:") {
			evidence = strings.TrimSpace(l[len("EVIDENCE:"):])
		}
	}
	return claim, evidence
}

func isGrounded(claim, evidence string) bool {
	claim = strings.TrimSpace(claim)
	if claim == "" || strings.TrimSpace(evidence) == "" {
		return false
	}
	ev := strings.ToLower(evidence)
	for _, w := range strings.Fields(strings.ToLower(claim)) {
		if len(w) <= 3 {
			continue // ignore trivial words
		}
		if !strings.Contains(ev, w) {
			return false
		}
	}
	return true
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func boundedSummary(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max]
}
