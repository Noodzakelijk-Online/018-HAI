package domainpack

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrPreferenceConflict = errors.New("domain pack preference revision conflict")

var compatibleCatalogVersions = map[string]struct{}{
	"1.1.0":        {},
	CatalogVersion: {},
}

type PreferenceRepository interface {
	Upsert(preference PackPreference) (PackPreference, error)
	Get(ownerIdentity string, packID PackID) (PackPreference, bool, error)
	List(ownerIdentity string) ([]PackPreference, error)
	Delete(ownerIdentity string, packID PackID) error
}

type MemoryPreferenceRepository struct {
	mu    sync.RWMutex
	now   func() time.Time
	items map[string]PackPreference
}

func NewMemoryPreferenceRepository(now func() time.Time) *MemoryPreferenceRepository {
	if now == nil {
		now = time.Now
	}
	return &MemoryPreferenceRepository{now: now, items: map[string]PackPreference{}}
}

func (repository *MemoryPreferenceRepository) Upsert(preference PackPreference) (PackPreference, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	key := preferenceKey(strings.TrimSpace(preference.OwnerIdentity), preference.PackID)
	existing, exists := repository.items[key]
	normalized, err := normalizePreference(preference, existing, exists, repository.now().UTC())
	if err != nil {
		return PackPreference{}, err
	}
	repository.items[key] = normalized
	preference = normalized
	return clonePreference(preference), nil
}

func (repository *MemoryPreferenceRepository) Get(ownerIdentity string, packID PackID) (PackPreference, bool, error) {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return PackPreference{}, false, fmt.Errorf("owner identity is required")
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	preference, exists := repository.items[preferenceKey(owner, packID)]
	return clonePreference(preference), exists, nil
}

func (repository *MemoryPreferenceRepository) List(ownerIdentity string) ([]PackPreference, error) {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	prefix := owner + "\x00"
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]PackPreference, 0)
	for key, preference := range repository.items {
		if strings.HasPrefix(key, prefix) {
			result = append(result, clonePreference(preference))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackID < result[j].PackID })
	return result, nil
}

func (repository *MemoryPreferenceRepository) Delete(ownerIdentity string, packID PackID) error {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return fmt.Errorf("owner identity is required")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	delete(repository.items, preferenceKey(owner, packID))
	return nil
}

func (registry *Registry) Resolve(ownerIdentity string, packID PackID, preferences PreferenceRepository) (PackView, error) {
	pack, exists := registry.Lookup(packID)
	if !exists {
		return PackView{}, fmt.Errorf("domain pack %q not found", packID)
	}
	view := PackView{Pack: pack, Enabled: pack.DefaultEnabled, LocalOnly: pack.Retention.LocalOnly}
	if preferences == nil || strings.TrimSpace(ownerIdentity) == "" {
		return view, nil
	}
	preference, exists, err := preferences.Get(ownerIdentity, packID)
	if err != nil {
		return PackView{}, fmt.Errorf("load domain pack preference: %w", err)
	}
	if !exists {
		return view, nil
	}
	view.Preference = &preference
	if preference.Status != PreferenceStatusActive {
		return view, nil
	}
	if preference.Enabled != nil {
		view.Enabled = *preference.Enabled
	}
	view.LocalOnly = view.LocalOnly || preference.ForceLocalOnly
	effective, err := applyAdaptation(view.Pack, preference.Adaptation)
	if err != nil {
		return PackView{}, fmt.Errorf("apply domain pack adaptation: %w", err)
	}
	view.Pack = effective
	return view, nil
}

func preferenceKey(ownerIdentity string, packID PackID) string {
	return ownerIdentity + "\x00" + string(packID)
}

func clonePreference(preference PackPreference) PackPreference {
	copy := preference
	if preference.Enabled != nil {
		enabled := *preference.Enabled
		copy.Enabled = &enabled
	}
	copy.Adaptation = cloneAdaptation(preference.Adaptation)
	return copy
}

func normalizePreference(
	preference PackPreference,
	existing PackPreference,
	exists bool,
	now time.Time,
) (PackPreference, error) {
	owner := strings.TrimSpace(preference.OwnerIdentity)
	if owner == "" {
		return PackPreference{}, fmt.Errorf("owner identity is required")
	}
	if strings.TrimSpace(string(preference.PackID)) == "" {
		return PackPreference{}, fmt.Errorf("domain pack id is required")
	}
	if preference.ClassificationBoost < -25 || preference.ClassificationBoost > 25 {
		return PackPreference{}, fmt.Errorf("classification boost must be between -25 and 25")
	}
	if preference.Status == "" {
		preference.Status = PreferenceStatusActive
	}
	if !validPreferenceStatus(preference.Status) {
		return PackPreference{}, fmt.Errorf("invalid domain pack preference status %q", preference.Status)
	}
	if preference.CatalogVersion != "" {
		if _, compatible := compatibleCatalogVersions[preference.CatalogVersion]; !compatible {
			return PackPreference{}, fmt.Errorf("domain pack preference catalog version must be compatible with %s", CatalogVersion)
		}
	}
	registry, err := NewBuiltinRegistry()
	if err != nil {
		return PackPreference{}, fmt.Errorf("load domain pack catalog: %w", err)
	}
	if _, exists := registry.Lookup(preference.PackID); !exists {
		return PackPreference{}, fmt.Errorf("domain pack %q not found", preference.PackID)
	}
	if exists && preference.Revision > 0 && preference.Revision != existing.Revision {
		return PackPreference{}, ErrPreferenceConflict
	}
	pack, _ := registry.Lookup(preference.PackID)
	if _, err := applyAdaptation(pack, preference.Adaptation); err != nil {
		return PackPreference{}, fmt.Errorf("invalid domain pack adaptation: %w", err)
	}
	preference.OwnerIdentity = owner
	preference.CatalogVersion = CatalogVersion
	preference.UpdatedAt = now.UTC()
	if exists {
		preference.CreatedAt = existing.CreatedAt
		preference.Revision = existing.Revision + 1
	} else {
		preference.CreatedAt = now.UTC()
		preference.Revision = 1
	}
	return clonePreference(preference), nil
}

func validPreferenceStatus(status PreferenceStatus) bool {
	return status == PreferenceStatusDraft ||
		status == PreferenceStatusActive ||
		status == PreferenceStatusArchived
}

func applyAdaptation(pack DomainPack, adaptation PackAdaptation) (DomainPack, error) {
	effective := clonePack(pack)
	for _, rule := range adaptation.AdditionalApprovalRules {
		if !rule.Required {
			return DomainPack{}, fmt.Errorf("additional approval rule %q cannot weaken approval", rule.Action)
		}
	}
	effective.ClassificationSignals = append(effective.ClassificationSignals, cloneSignals(adaptation.AdditionalClassificationSignals)...)
	effective.IntakeQuestions = append(effective.IntakeQuestions, adaptation.AdditionalIntakeQuestions...)
	effective.RiskTriggers = append(effective.RiskTriggers, adaptation.AdditionalRiskTriggers...)
	effective.ApprovalRules = append(effective.ApprovalRules, adaptation.AdditionalApprovalRules...)
	effective.EvidenceRequirements = append(effective.EvidenceRequirements, adaptation.AdditionalEvidenceRequirements...)
	effective.DeterministicValidators = append(effective.DeterministicValidators, adaptation.AdditionalValidators...)
	effective.StopEscalationConditions = append(effective.StopEscalationConditions, adaptation.AdditionalStopConditions...)
	effective.SuitableAgentCapabilities = append(effective.SuitableAgentCapabilities, adaptation.AdditionalAgentCapabilities...)
	if err := ValidatePack(effective); err != nil {
		return DomainPack{}, err
	}
	return effective, nil
}

func cloneAdaptation(value PackAdaptation) PackAdaptation {
	copy := value
	copy.AdditionalClassificationSignals = cloneSignals(value.AdditionalClassificationSignals)
	copy.AdditionalIntakeQuestions = append([]IntakeQuestion(nil), value.AdditionalIntakeQuestions...)
	copy.AdditionalRiskTriggers = append([]RiskTrigger(nil), value.AdditionalRiskTriggers...)
	copy.AdditionalApprovalRules = append([]ApprovalRule(nil), value.AdditionalApprovalRules...)
	copy.AdditionalEvidenceRequirements = append([]EvidenceRequirement(nil), value.AdditionalEvidenceRequirements...)
	for index := range copy.AdditionalEvidenceRequirements {
		copy.AdditionalEvidenceRequirements[index].RequiredForActions =
			cloneStrings(value.AdditionalEvidenceRequirements[index].RequiredForActions)
	}
	copy.AdditionalValidators = append([]DeterministicValidator(nil), value.AdditionalValidators...)
	copy.AdditionalStopConditions = append([]StopCondition(nil), value.AdditionalStopConditions...)
	copy.AdditionalAgentCapabilities = cloneStrings(value.AdditionalAgentCapabilities)
	return copy
}
