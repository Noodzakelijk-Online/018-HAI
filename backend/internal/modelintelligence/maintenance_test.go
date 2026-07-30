package modelintelligence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type modelMaintenanceGateStub struct {
	endpoint string
	modelID  string
	err      error
}

func (s *modelMaintenanceGateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error {
	s.endpoint = endpointURL
	s.modelID = modelID
	return s.err
}

type maintainedStaticProvider struct {
	profile  ModelProfile
	endpoint string
	modelID  string
	calls    int
}

func (p *maintainedStaticProvider) ID() string          { return p.profile.ProviderID }
func (p *maintainedStaticProvider) DisplayName() string { return p.profile.DisplayName }
func (p *maintainedStaticProvider) Profiles() []ModelProfile {
	return []ModelProfile{p.profile}
}
func (p *maintainedStaticProvider) Probe(context.Context, time.Time) ProbeResult {
	return ProbeResult{ProviderID: p.profile.ProviderID, Status: ProviderActive}
}
func (p *maintainedStaticProvider) Generate(_ context.Context, req InferenceRequest, _ time.Time) (InferenceResult, error) {
	p.calls++
	return InferenceResult{ProviderID: p.profile.ProviderID, ModelID: p.profile.ModelID, Lane: req.Lane, Output: "category=general; summary=ok", OK: true}, nil
}
func (p *maintainedStaticProvider) ModelMaintenanceIdentity() (string, string, bool) {
	return p.endpoint, p.modelID, true
}

func TestLocalModelIntelligenceCallsRequireCanonicalMaintenance(t *testing.T) {
	provider := &maintainedStaticProvider{
		profile:  ModelProfile{ProviderID: "local-test", ModelID: "qwen-local", DisplayName: "Local test", Local: true, Status: ProviderActive, Lanes: []RoutingLane{LaneFastTriage}},
		endpoint: "http://127.0.0.1:11434",
		modelID:  "qwen-local",
	}
	gate := &modelMaintenanceGateStub{err: errors.New("model refresh failed")}
	service := NewService(&Registry{providers: []Provider{provider}}).WithModelMaintenance(gate)

	decision, result, err := service.RunLane(context.Background(), LaneFastTriage, LaneInput{}, "classify this", "operation-1")
	if err == nil || result != nil || !decision.Routable || provider.calls != 0 {
		t.Fatalf("maintenance failure must prevent inference: decision=%#v result=%#v err=%v calls=%d", decision, result, err, provider.calls)
	}
	if gate.endpoint != provider.endpoint || gate.modelID != provider.modelID {
		t.Fatalf("gate received endpoint=%q model=%q", gate.endpoint, gate.modelID)
	}

	benchmark, err := service.Benchmark(context.Background(), provider.profile.ProviderID, provider.profile.ModelID)
	if err != nil || benchmark.OK || provider.calls != 0 || !strings.Contains(benchmark.Detail, "daily model maintenance") {
		t.Fatalf("benchmark must report maintenance block without inference: %#v err=%v calls=%d", benchmark, err, provider.calls)
	}

	gate.err = nil
	_, result, err = service.RunLane(context.Background(), LaneFastTriage, LaneInput{}, "classify this", "operation-2")
	if err != nil || result == nil || !result.OK || provider.calls != 1 {
		t.Fatalf("maintained local inference = %#v err=%v calls=%d", result, err, provider.calls)
	}
}

func TestMaintainedLocalModelFailsClosedWhenGateIsNotWired(t *testing.T) {
	provider := &maintainedStaticProvider{
		profile:  ModelProfile{ProviderID: "local-test", ModelID: "qwen-local", DisplayName: "Local test", Local: true, Status: ProviderActive, Lanes: []RoutingLane{LaneFastTriage}},
		endpoint: "http://127.0.0.1:11434",
		modelID:  "qwen-local",
	}
	service := NewService(&Registry{providers: []Provider{provider}})
	_, result, err := service.RunLane(context.Background(), LaneFastTriage, LaneInput{}, "classify this", "operation-1")
	if err == nil || result != nil || provider.calls != 0 || !strings.Contains(err.Error(), "maintenance gate is unavailable") {
		t.Fatalf("unwired maintenance gate must fail closed: result=%#v err=%v calls=%d", result, err, provider.calls)
	}
}
