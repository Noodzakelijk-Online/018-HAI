package task

import (
	"errors"
	"testing"
	"time"

	"automation-hub-backend/internal/frameworkregistry"
)

type operatingContextProviderStub struct {
	needs         []frameworkregistry.NeedStateAssessment
	capacity      *frameworkregistry.CapacitySnapshot
	needsErr      error
	capacityErr   error
	needsCalls    int
	capacityCalls int
}

func (s *operatingContextProviderStub) LatestNeeds(_ string, _ time.Time) ([]frameworkregistry.NeedStateAssessment, error) {
	s.needsCalls++
	return append([]frameworkregistry.NeedStateAssessment(nil), s.needs...), s.needsErr
}

func (s *operatingContextProviderStub) LatestCapacity(_ string, _ time.Time) (*frameworkregistry.CapacitySnapshot, error) {
	s.capacityCalls++
	if s.capacity == nil {
		return nil, s.capacityErr
	}
	copied := *s.capacity
	copied.Constraints = append([]string(nil), s.capacity.Constraints...)
	return &copied, s.capacityErr
}

func TestLoadOperatingContextUsesOwnerScopedProvider(t *testing.T) {
	provider := &operatingContextProviderStub{
		needs: []frameworkregistry.NeedStateAssessment{{
			ID:         "rest",
			State:      "under_supported",
			Confidence: 0.9,
			Source:     "operator_check_in",
		}},
		capacity: &frameworkregistry.CapacitySnapshot{
			Status:      "limited",
			Energy:      35,
			Attention:   45,
			Constraints: []string{"limit concurrent work"},
			Confidence:  0.9,
			Fresh:       true,
		},
	}
	s := &service{operatingContext: provider}

	got, err := s.loadOperatingContext(IntakeRequest{OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("load operating context: %v", err)
	}
	if len(got.ObservedNeeds) != 1 || got.ObservedNeeds[0].ID != "rest" {
		t.Fatalf("needs = %#v", got.ObservedNeeds)
	}
	if got.Capacity == nil || got.Capacity.Status != "limited" {
		t.Fatalf("capacity = %#v", got.Capacity)
	}
	if provider.needsCalls != 1 || provider.capacityCalls != 1 {
		t.Fatalf("provider calls = needs %d, capacity %d", provider.needsCalls, provider.capacityCalls)
	}
}

func TestLoadOperatingContextPreservesTrustedInProcessValues(t *testing.T) {
	provider := &operatingContextProviderStub{
		needsErr:    errors.New("should not load needs"),
		capacityErr: errors.New("should not load capacity"),
	}
	s := &service{operatingContext: provider}
	request := IntakeRequest{
		OwnerIdentity: "alice",
		ObservedNeeds: []frameworkregistry.NeedStateAssessment{{ID: "safety"}},
		Capacity:      &frameworkregistry.CapacitySnapshot{Status: "available"},
	}

	got, err := s.loadOperatingContext(request)
	if err != nil {
		t.Fatalf("load operating context: %v", err)
	}
	if got.ObservedNeeds[0].ID != "safety" || got.Capacity.Status != "available" {
		t.Fatalf("trusted values changed: %#v / %#v", got.ObservedNeeds, got.Capacity)
	}
	if provider.needsCalls != 0 || provider.capacityCalls != 0 {
		t.Fatalf("provider should not be called, got needs %d capacity %d", provider.needsCalls, provider.capacityCalls)
	}
}

func TestLoadOperatingContextFailsClosedOnProviderError(t *testing.T) {
	provider := &operatingContextProviderStub{needsErr: errors.New("database unavailable")}
	s := &service{operatingContext: provider}

	if _, err := s.loadOperatingContext(IntakeRequest{OwnerIdentity: "alice"}); err == nil {
		t.Fatal("expected provider error")
	}
	if provider.capacityCalls != 0 {
		t.Fatalf("capacity should not load after needs failure, got %d calls", provider.capacityCalls)
	}
}
