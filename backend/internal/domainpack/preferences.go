package domainpack

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

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
	preference.OwnerIdentity = owner
	if preference.UpdatedAt.IsZero() {
		preference.UpdatedAt = repository.now().UTC()
	} else {
		preference.UpdatedAt = preference.UpdatedAt.UTC()
	}
	preference = clonePreference(preference)

	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.items[preferenceKey(owner, preference.PackID)] = preference
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
	if preference.Enabled != nil {
		view.Enabled = *preference.Enabled
	}
	view.LocalOnly = view.LocalOnly || preference.ForceLocalOnly
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
	return copy
}
