package frameworkregistry

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/safety"
)

const (
	frameworkMigrationCompatible = "compatible"
	frameworkMigrationTransform  = "transform"
	frameworkMigrationManual     = "manual"
	frameworkMigrationBreaking   = "breaking"
)

type frameworkCatalogLifecycle struct {
	mu       sync.RWMutex
	order    []string
	versions map[string]map[string]FrameworkVersionRecord
	active   map[string]string
	history  map[string][]FrameworkLifecycleEvent
}

type frameworkLifecycleEventFingerprint struct {
	Sequence              uint64                     `json:"sequence"`
	FrameworkID           string                     `json:"frameworkId"`
	Version               string                     `json:"version"`
	Action                string                     `json:"action"`
	PreviousActiveVersion string                     `json:"previousActiveVersion,omitempty"`
	ActiveVersion         string                     `json:"activeVersion,omitempty"`
	Actor                 string                     `json:"actor"`
	Reason                string                     `json:"reason"`
	Migration             FrameworkMigrationMetadata `json:"migration"`
	Provenance            FrameworkVersionProvenance `json:"provenance"`
	OccurredAt            time.Time                  `json:"occurredAt"`
	PreviousEventDigest   string                     `json:"previousEventDigest,omitempty"`
}

func newFrameworkCatalogLifecycle(
	items []Framework,
	now time.Time,
) (*frameworkCatalogLifecycle, []Framework, error) {
	if err := ValidateCatalog(items); err != nil {
		return nil, nil, err
	}
	now = normalizedLifecycleTime(now)
	lifecycle := &frameworkCatalogLifecycle{
		order:    make([]string, 0, len(items)),
		versions: make(map[string]map[string]FrameworkVersionRecord, len(items)),
		active:   make(map[string]string, len(items)),
		history:  make(map[string][]FrameworkLifecycleEvent, len(items)),
	}
	seenIDs := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = cloneFramework(item)
		id := normalizeFrameworkID(item.ID)
		if _, seen := seenIDs[id]; !seen {
			lifecycle.order = append(lifecycle.order, id)
			seenIDs[id] = struct{}{}
		}
		if lifecycle.versions[id] == nil {
			lifecycle.versions[id] = map[string]FrameworkVersionRecord{}
		}
		digest, err := frameworkContentDigest(item)
		if err != nil {
			return nil, nil, err
		}
		lifecycle.versions[id][item.Version] = FrameworkVersionRecord{
			Framework:      item,
			LifecycleState: FrameworkVersionStaged,
			Migration: FrameworkMigrationMetadata{
				Strategy:           frameworkMigrationCompatible,
				ChangeSummary:      "Imported immutable catalog version.",
				CompatibilityNotes: "Catalog bootstrap preserves the framework contract.",
				ValidationCriteria: []string{"framework catalog validation passes"},
			},
			VersionProvenance: FrameworkVersionProvenance{
				Source:        item.Source,
				AuthoredBy:    item.Provenance,
				ImportedBy:    "system:catalog-bootstrap",
				ImportedAt:    now,
				ContentDigest: digest,
			},
			RegisteredAt: now,
		}
	}

	for _, id := range lifecycle.order {
		versions := sortedFrameworkVersions(lifecycle.versions[id], false)
		var previous string
		for _, version := range versions {
			record := lifecycle.versions[id][version]
			record.Supersedes = previous
			record.Migration.FromVersion = previous
			if previous != "" {
				parent := lifecycle.versions[id][previous]
				record.VersionProvenance.ParentDigest = parent.VersionProvenance.ContentDigest
			}
			lifecycle.versions[id][version] = record
			if _, err := lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
				FrameworkID: id,
				Version:     version,
				Action:      FrameworkLifecycleRegistered,
				Actor:       "system:catalog-bootstrap",
				Reason:      "Imported immutable catalog version.",
				Migration:   record.Migration,
				Provenance:  record.VersionProvenance,
				OccurredAt:  now,
			}); err != nil {
				return nil, nil, err
			}
			if previous != "" {
				prior := lifecycle.versions[id][previous]
				retiredAt := now
				prior.LifecycleState = FrameworkVersionRetired
				prior.RetiredAt = &retiredAt
				lifecycle.versions[id][previous] = prior
			}
			activatedAt := now
			record = lifecycle.versions[id][version]
			record.LifecycleState = FrameworkVersionActive
			record.ActivatedAt = &activatedAt
			record.RetiredAt = nil
			lifecycle.versions[id][version] = record
			lifecycle.active[id] = version
			if _, err := lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
				FrameworkID:           id,
				Version:               version,
				Action:                FrameworkLifecycleActivated,
				PreviousActiveVersion: previous,
				ActiveVersion:         version,
				Actor:                 "system:catalog-bootstrap",
				Reason:                "Selected the highest semantic version deterministically during catalog bootstrap.",
				Migration:             record.Migration,
				Provenance:            record.VersionProvenance,
				OccurredAt:            now,
			}); err != nil {
				return nil, nil, err
			}
			previous = version
		}
	}
	return lifecycle, lifecycle.activeCatalogLocked(), nil
}

// NewServiceWithCatalog is primarily intended for catalog upgrades and
// deterministic replay. When multiple versions of an ID are supplied, the
// highest semantic version becomes active and prior versions become retired.
func NewServiceWithCatalog(repo Repository, catalog []Framework) (*Service, error) {
	return newServiceWithCatalog(repo, catalog, time.Now)
}

func newServiceWithCatalog(
	repo Repository,
	catalog []Framework,
	now func() time.Time,
) (*Service, error) {
	if now == nil {
		now = time.Now
	}
	lifecycle, active, err := newFrameworkCatalogLifecycle(catalog, now().UTC())
	if err != nil {
		return nil, err
	}
	if repo == nil {
		repo = NewMemoryRepository()
	}
	return &Service{
		catalog:   active,
		lifecycle: lifecycle,
		repo:      repo,
		now:       now,
	}, nil
}

// StageFrameworkVersion registers immutable content without changing ID-only
// lookups or selector behavior.
func (s *Service) StageFrameworkVersion(
	request StageFrameworkVersionRequest,
) (*FrameworkVersionRecord, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	request.Actor = compactLifecycleText(request.Actor, 255)
	if request.Actor == "" {
		return nil, fmt.Errorf("actor is required")
	}
	request.Framework = cloneFramework(request.Framework)
	id := normalizeFrameworkID(request.Framework.ID)
	version := strings.TrimSpace(request.Framework.Version)

	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	versions, ok := s.lifecycle.versions[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	if !catalogSemanticVersionPattern.MatchString(version) {
		return nil, fmt.Errorf("framework has invalid semantic version %q", version)
	}
	if _, exists := versions[version]; exists {
		return nil, fmt.Errorf("framework version %s@%s already exists", id, version)
	}
	activeVersion := s.lifecycle.active[id]
	active := versions[activeVersion]
	if !sameFrameworkIdentity(active.Framework, request.Framework) {
		return nil, fmt.Errorf("framework version identity must preserve id, name, and family")
	}
	request.Framework.ID = active.ID
	if compareSemanticVersions(version, activeVersion) <= 0 {
		return nil, fmt.Errorf(
			"staged version %s must be newer than active version %s",
			version,
			activeVersion,
		)
	}

	request.Supersedes = strings.TrimSpace(request.Supersedes)
	if request.Supersedes == "" {
		request.Supersedes = activeVersion
	}
	if request.Supersedes != activeVersion {
		return nil, fmt.Errorf(
			"staged version must supersede active version %s",
			activeVersion,
		)
	}
	migration, err := normalizeFrameworkMigration(request.Migration, activeVersion)
	if err != nil {
		return nil, err
	}
	candidateCatalog := s.lifecycle.allFrameworksLocked()
	candidateCatalog = append(candidateCatalog, request.Framework)
	if err := ValidateCatalog(candidateCatalog); err != nil {
		return nil, fmt.Errorf("invalid staged framework version: %w", err)
	}
	digest, err := frameworkContentDigest(request.Framework)
	if err != nil {
		return nil, err
	}
	provenance, err := normalizeFrameworkProvenance(
		request.Provenance,
		request.Framework,
		request.Actor,
		active.VersionProvenance.ContentDigest,
		digest,
		s.now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	now := normalizedLifecycleTime(s.now().UTC())
	record := FrameworkVersionRecord{
		Framework:         request.Framework,
		LifecycleState:    FrameworkVersionStaged,
		Supersedes:        activeVersion,
		Migration:         migration,
		VersionProvenance: provenance,
		RegisteredAt:      now,
	}
	versions[version] = record
	if _, err := s.lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
		FrameworkID:           id,
		Version:               version,
		Action:                FrameworkLifecycleRegistered,
		PreviousActiveVersion: activeVersion,
		ActiveVersion:         activeVersion,
		Actor:                 request.Actor,
		Reason:                migration.ChangeSummary,
		Migration:             migration,
		Provenance:            provenance,
		OccurredAt:            now,
	}); err != nil {
		delete(versions, version)
		return nil, err
	}
	result := cloneFrameworkVersionRecord(record)
	return &result, nil
}

func (s *Service) ActivateFrameworkVersion(
	id string,
	version string,
	request ActivateFrameworkVersionRequest,
) (*FrameworkVersionRecord, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	version = strings.TrimSpace(version)
	request.Actor = compactLifecycleText(request.Actor, 255)
	request.Reason = compactLifecycleText(request.Reason, 1024)
	if request.Actor == "" || request.Reason == "" {
		return nil, fmt.Errorf("actor and activation reason are required")
	}

	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	versions, ok := s.lifecycle.versions[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	target, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("framework version not found")
	}
	activeVersion := s.lifecycle.active[id]
	if request.ExpectedActiveVersion != activeVersion {
		return nil, fmt.Errorf(
			"active framework version changed: expected %s, current %s",
			request.ExpectedActiveVersion,
			activeVersion,
		)
	}
	if !strings.EqualFold(
		strings.TrimSpace(request.ExpectedTargetDigest),
		target.VersionProvenance.ContentDigest,
	) {
		return nil, fmt.Errorf("target framework digest does not match")
	}
	if target.LifecycleState == FrameworkVersionActive {
		result := cloneFrameworkVersionRecord(target)
		return &result, nil
	}
	if target.LifecycleState != FrameworkVersionStaged {
		return nil, fmt.Errorf("only a staged framework version may be activated")
	}
	if target.Migration.FromVersion != activeVersion || target.Supersedes != activeVersion {
		return nil, fmt.Errorf("staged migration no longer matches the active version")
	}

	now := normalizedLifecycleTime(s.now().UTC())
	active := versions[activeVersion]
	retiredAt := now
	active.LifecycleState = FrameworkVersionRetired
	active.RetiredAt = &retiredAt
	versions[activeVersion] = active
	activatedAt := now
	target.LifecycleState = FrameworkVersionActive
	target.ActivatedAt = &activatedAt
	target.RetiredAt = nil
	versions[version] = target
	s.lifecycle.active[id] = version
	s.catalog = s.lifecycle.activeCatalogLocked()
	if _, err := s.lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
		FrameworkID:           id,
		Version:               version,
		Action:                FrameworkLifecycleActivated,
		PreviousActiveVersion: activeVersion,
		ActiveVersion:         version,
		Actor:                 request.Actor,
		Reason:                request.Reason,
		Migration:             target.Migration,
		Provenance:            target.VersionProvenance,
		OccurredAt:            now,
	}); err != nil {
		return nil, err
	}
	result := cloneFrameworkVersionRecord(target)
	return &result, nil
}

func (s *Service) RollbackFrameworkVersion(
	id string,
	version string,
	request RollbackFrameworkVersionRequest,
) (*FrameworkVersionRecord, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	version = strings.TrimSpace(version)
	request.Actor = compactLifecycleText(request.Actor, 255)
	request.Reason = compactLifecycleText(request.Reason, 1024)
	if request.Actor == "" || request.Reason == "" {
		return nil, fmt.Errorf("actor and rollback reason are required")
	}

	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	versions, ok := s.lifecycle.versions[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	target, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("framework version not found")
	}
	activeVersion := s.lifecycle.active[id]
	if request.ExpectedActiveVersion != activeVersion {
		return nil, fmt.Errorf(
			"active framework version changed: expected %s, current %s",
			request.ExpectedActiveVersion,
			activeVersion,
		)
	}
	if !strings.EqualFold(
		strings.TrimSpace(request.ExpectedTargetDigest),
		target.VersionProvenance.ContentDigest,
	) {
		return nil, fmt.Errorf("target framework digest does not match")
	}
	if target.LifecycleState != FrameworkVersionRetired {
		return nil, fmt.Errorf("rollback target must be a retired framework version")
	}
	if !s.lifecycle.wasPreviouslyActiveLocked(id, version) {
		return nil, fmt.Errorf("rollback target was never active")
	}

	now := normalizedLifecycleTime(s.now().UTC())
	current := versions[activeVersion]
	retiredAt := now
	current.LifecycleState = FrameworkVersionRetired
	current.RetiredAt = &retiredAt
	versions[activeVersion] = current
	activatedAt := now
	target.LifecycleState = FrameworkVersionActive
	target.ActivatedAt = &activatedAt
	target.RetiredAt = nil
	versions[version] = target
	s.lifecycle.active[id] = version
	s.catalog = s.lifecycle.activeCatalogLocked()
	rollbackMigration := FrameworkMigrationMetadata{
		FromVersion:        activeVersion,
		Strategy:           frameworkMigrationManual,
		ChangeSummary:      request.Reason,
		CompatibilityNotes: "Rollback restores an immutable previously active framework contract.",
		MigrationSteps:     []string{"restore the prior framework contract as the active projection"},
		ValidationCriteria: append([]string(nil), target.Migration.ValidationCriteria...),
	}
	if len(rollbackMigration.ValidationCriteria) == 0 {
		rollbackMigration.ValidationCriteria = []string{"the restored framework passes catalog validation"}
	}
	if _, err := s.lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
		FrameworkID:           id,
		Version:               version,
		Action:                FrameworkLifecycleRolledBack,
		PreviousActiveVersion: activeVersion,
		ActiveVersion:         version,
		Actor:                 request.Actor,
		Reason:                request.Reason,
		Migration:             rollbackMigration,
		Provenance:            target.VersionProvenance,
		OccurredAt:            now,
	}); err != nil {
		return nil, err
	}
	result := cloneFrameworkVersionRecord(target)
	return &result, nil
}

// RetireFrameworkVersion removes an unactivated staged version from
// consideration without deleting its immutable content or history.
func (s *Service) RetireFrameworkVersion(
	id string,
	version string,
	request RetireFrameworkVersionRequest,
) (*FrameworkVersionRecord, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	version = strings.TrimSpace(version)
	request.Actor = compactLifecycleText(request.Actor, 255)
	request.Reason = compactLifecycleText(request.Reason, 1024)
	if request.Actor == "" || request.Reason == "" {
		return nil, fmt.Errorf("actor and retirement reason are required")
	}
	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	versions, ok := s.lifecycle.versions[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	target, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("framework version not found")
	}
	if target.LifecycleState != FrameworkVersionStaged {
		return nil, fmt.Errorf("only a staged framework version may be retired directly")
	}
	if !strings.EqualFold(
		strings.TrimSpace(request.ExpectedTargetDigest),
		target.VersionProvenance.ContentDigest,
	) {
		return nil, fmt.Errorf("target framework digest does not match")
	}
	now := normalizedLifecycleTime(s.now().UTC())
	retiredAt := now
	target.LifecycleState = FrameworkVersionRetired
	target.RetiredAt = &retiredAt
	versions[version] = target
	if _, err := s.lifecycle.appendEventLocked(id, FrameworkLifecycleEvent{
		FrameworkID:   id,
		Version:       version,
		Action:        FrameworkLifecycleRetired,
		ActiveVersion: s.lifecycle.active[id],
		Actor:         request.Actor,
		Reason:        request.Reason,
		Migration:     target.Migration,
		Provenance:    target.VersionProvenance,
		OccurredAt:    now,
	}); err != nil {
		return nil, err
	}
	result := cloneFrameworkVersionRecord(target)
	return &result, nil
}

func (s *Service) ActiveFrameworkVersion(id string) (*FrameworkVersionRecord, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	s.lifecycle.mu.RLock()
	defer s.lifecycle.mu.RUnlock()
	version, ok := s.lifecycle.active[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	record := cloneFrameworkVersionRecord(s.lifecycle.versions[id][version])
	return &record, nil
}

func (s *Service) FrameworkVersions(
	owner string,
	id string,
) ([]FrameworkVersionView, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	s.lifecycle.mu.RLock()
	versions, ok := s.lifecycle.versions[id]
	if !ok {
		s.lifecycle.mu.RUnlock()
		return nil, fmt.Errorf("framework not found")
	}
	records := make([]FrameworkVersionRecord, 0, len(versions))
	for _, version := range sortedFrameworkVersions(versions, true) {
		records = append(records, cloneFrameworkVersionRecord(versions[version]))
	}
	s.lifecycle.mu.RUnlock()
	preference, err := s.preferenceForFramework(owner, id)
	if err != nil {
		return nil, err
	}
	result := make([]FrameworkVersionView, 0, len(records))
	for _, record := range records {
		result = append(result, applyVersionPreference(record, preference))
	}
	return result, nil
}

func (s *Service) GetFrameworkVersion(
	owner string,
	id string,
	version string,
) (*FrameworkVersionView, error) {
	versions, err := s.FrameworkVersions(owner, id)
	if err != nil {
		return nil, err
	}
	version = strings.TrimSpace(version)
	for _, candidate := range versions {
		if candidate.Version == version {
			result := candidate
			return &result, nil
		}
	}
	return nil, fmt.Errorf("framework version not found")
}

func (s *Service) FrameworkLifecycleHistory(
	id string,
	limit int,
) ([]FrameworkLifecycleEvent, error) {
	if s == nil || s.lifecycle == nil {
		return nil, fmt.Errorf("framework registry lifecycle is unavailable")
	}
	id = normalizeFrameworkID(id)
	s.lifecycle.mu.RLock()
	defer s.lifecycle.mu.RUnlock()
	events, ok := s.lifecycle.history[id]
	if !ok {
		return nil, fmt.Errorf("framework not found")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	start := 0
	if len(events) > limit {
		start = len(events) - limit
	}
	result := make([]FrameworkLifecycleEvent, 0, len(events)-start)
	for index := len(events) - 1; index >= start; index-- {
		result = append(result, cloneFrameworkLifecycleEvent(events[index]))
	}
	return result, nil
}

func VerifyFrameworkLifecycleHistory(events []FrameworkLifecycleEvent) error {
	if len(events) == 0 {
		return nil
	}
	ordered := append([]FrameworkLifecycleEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Sequence < ordered[j].Sequence
	})
	expectedSequence := ordered[0].Sequence
	previous := ordered[0].PreviousEventDigest
	if expectedSequence == 1 && previous != "" {
		return fmt.Errorf("framework lifecycle root event has a previous digest")
	}
	for _, event := range ordered {
		if event.Sequence != expectedSequence {
			return fmt.Errorf(
				"framework lifecycle sequence gap: got %d, want %d",
				event.Sequence,
				expectedSequence,
			)
		}
		if event.PreviousEventDigest != previous {
			return fmt.Errorf("framework lifecycle digest chain is broken at sequence %d", event.Sequence)
		}
		digest, err := frameworkLifecycleEventDigest(event)
		if err != nil {
			return err
		}
		if !strings.EqualFold(digest, event.EventDigest) || event.ID != event.EventDigest {
			return fmt.Errorf("framework lifecycle event %d digest is invalid", event.Sequence)
		}
		previous = event.EventDigest
		expectedSequence++
	}
	return nil
}

func (s *Service) activeCatalogSnapshot() []Framework {
	if s == nil {
		return nil
	}
	if s.lifecycle == nil {
		result := make([]Framework, 0, len(s.catalog))
		for _, framework := range s.catalog {
			result = append(result, cloneFramework(framework))
		}
		return result
	}
	s.lifecycle.mu.RLock()
	defer s.lifecycle.mu.RUnlock()
	result := make([]Framework, 0, len(s.catalog))
	for _, framework := range s.catalog {
		result = append(result, cloneFramework(framework))
	}
	return result
}

func (s *Service) preferenceForFramework(owner string, id string) (Preference, error) {
	preferences, err := s.repo.ListPreferences(strings.TrimSpace(owner))
	if err != nil {
		return Preference{}, err
	}
	for _, preference := range preferences {
		if normalizeFrameworkID(preference.FrameworkID) == id {
			return preference, nil
		}
	}
	return Preference{FrameworkID: id, State: PreferenceDefault}, nil
}

func applyVersionPreference(
	record FrameworkVersionRecord,
	preference Preference,
) FrameworkVersionView {
	view := applyPreference(record.Framework, preference)
	return FrameworkVersionView{
		FrameworkVersionRecord: cloneFrameworkVersionRecord(record),
		Enabled:                view.Enabled && record.LifecycleState == FrameworkVersionActive,
		Pinned:                 view.Pinned,
		EffectiveAutonomyLevel: view.EffectiveAutonomyLevel,
		Adaptations:            append([]string(nil), view.Adaptations...),
		PreferenceUpdatedAt:    cloneTimePointer(view.PreferenceUpdatedAt),
	}
}

func (l *frameworkCatalogLifecycle) activeCatalogLocked() []Framework {
	result := make([]Framework, 0, len(l.order))
	for _, id := range l.order {
		version := l.active[id]
		result = append(result, cloneFramework(l.versions[id][version].Framework))
	}
	return result
}

func (l *frameworkCatalogLifecycle) allFrameworksLocked() []Framework {
	result := make([]Framework, 0)
	for _, id := range l.order {
		for _, version := range sortedFrameworkVersions(l.versions[id], false) {
			result = append(result, cloneFramework(l.versions[id][version].Framework))
		}
	}
	return result
}

func (l *frameworkCatalogLifecycle) appendEventLocked(
	id string,
	event FrameworkLifecycleEvent,
) (FrameworkLifecycleEvent, error) {
	events := l.history[id]
	event.Sequence = uint64(len(events) + 1)
	event.FrameworkID = normalizeFrameworkID(event.FrameworkID)
	event.OccurredAt = normalizedLifecycleTime(event.OccurredAt)
	if len(events) > 0 {
		event.PreviousEventDigest = events[len(events)-1].EventDigest
	}
	digest, err := frameworkLifecycleEventDigest(event)
	if err != nil {
		return FrameworkLifecycleEvent{}, err
	}
	event.ID = digest
	event.EventDigest = digest
	l.history[id] = append(events, cloneFrameworkLifecycleEvent(event))
	return cloneFrameworkLifecycleEvent(event), nil
}

func (l *frameworkCatalogLifecycle) wasPreviouslyActiveLocked(id, version string) bool {
	for _, event := range l.history[id] {
		if (event.Action == FrameworkLifecycleActivated ||
			event.Action == FrameworkLifecycleRolledBack) &&
			event.ActiveVersion == version {
			return true
		}
	}
	return false
}

func frameworkLifecycleEventDigest(event FrameworkLifecycleEvent) (string, error) {
	return canonicalSHA256(frameworkLifecycleEventFingerprint{
		Sequence:              event.Sequence,
		FrameworkID:           event.FrameworkID,
		Version:               event.Version,
		Action:                event.Action,
		PreviousActiveVersion: event.PreviousActiveVersion,
		ActiveVersion:         event.ActiveVersion,
		Actor:                 event.Actor,
		Reason:                event.Reason,
		Migration:             event.Migration,
		Provenance:            event.Provenance,
		OccurredAt:            event.OccurredAt,
		PreviousEventDigest:   event.PreviousEventDigest,
	})
}

func frameworkContentDigest(framework Framework) (string, error) {
	return canonicalSHA256(cloneFramework(framework))
}

func normalizeFrameworkMigration(
	migration FrameworkMigrationMetadata,
	activeVersion string,
) (FrameworkMigrationMetadata, error) {
	migration.FromVersion = strings.TrimSpace(migration.FromVersion)
	if migration.FromVersion == "" {
		migration.FromVersion = activeVersion
	}
	if migration.FromVersion != activeVersion {
		return FrameworkMigrationMetadata{}, fmt.Errorf(
			"migration must start from active version %s",
			activeVersion,
		)
	}
	migration.Strategy = strings.ToLower(strings.TrimSpace(migration.Strategy))
	switch migration.Strategy {
	case frameworkMigrationCompatible,
		frameworkMigrationTransform,
		frameworkMigrationManual,
		frameworkMigrationBreaking:
	default:
		return FrameworkMigrationMetadata{}, fmt.Errorf("unsupported framework migration strategy")
	}
	migration.ChangeSummary = compactLifecycleText(migration.ChangeSummary, 1024)
	migration.CompatibilityNotes = compactLifecycleText(migration.CompatibilityNotes, 2000)
	if migration.ChangeSummary == "" || migration.CompatibilityNotes == "" {
		return FrameworkMigrationMetadata{}, fmt.Errorf(
			"migration change summary and compatibility notes are required",
		)
	}
	migration.MigrationSteps = compactLifecycleStrings(migration.MigrationSteps, 50, 1000)
	migration.ValidationCriteria = compactLifecycleStrings(
		migration.ValidationCriteria,
		50,
		1000,
	)
	if len(migration.ValidationCriteria) == 0 {
		return FrameworkMigrationMetadata{}, fmt.Errorf(
			"migration validation criteria are required",
		)
	}
	if migration.Strategy != frameworkMigrationCompatible &&
		len(migration.MigrationSteps) == 0 {
		return FrameworkMigrationMetadata{}, fmt.Errorf(
			"non-compatible migration strategies require migration steps",
		)
	}
	return migration, nil
}

func normalizeFrameworkProvenance(
	provenance FrameworkVersionProvenance,
	framework Framework,
	actor string,
	parentDigest string,
	contentDigest string,
	now time.Time,
) (FrameworkVersionProvenance, error) {
	provenance.Source = compactLifecycleText(provenance.Source, 1000)
	if provenance.Source == "" {
		provenance.Source = compactLifecycleText(framework.Source, 1000)
	}
	provenance.Reference = compactLifecycleText(provenance.Reference, 2000)
	provenance.AuthoredBy = compactLifecycleText(provenance.AuthoredBy, 255)
	if provenance.AuthoredBy == "" {
		provenance.AuthoredBy = compactLifecycleText(framework.Provenance, 255)
	}
	provenance.ImportedBy = actor
	provenance.ImportedAt = normalizedLifecycleTime(now)
	if provenance.Source == "" || provenance.AuthoredBy == "" {
		return FrameworkVersionProvenance{}, fmt.Errorf(
			"framework provenance source and author are required",
		)
	}
	suppliedParentDigest := strings.ToLower(strings.TrimSpace(provenance.ParentDigest))
	if suppliedParentDigest != "" && suppliedParentDigest != parentDigest {
		return FrameworkVersionProvenance{}, fmt.Errorf("framework parent digest does not match")
	}
	provenance.ParentDigest = parentDigest
	provenance.ContentDigest = contentDigest
	return provenance, nil
}

func sameFrameworkIdentity(left, right Framework) bool {
	return normalizeFrameworkID(left.ID) == normalizeFrameworkID(right.ID) &&
		strings.EqualFold(strings.TrimSpace(left.Name), strings.TrimSpace(right.Name)) &&
		strings.EqualFold(strings.TrimSpace(left.Family), strings.TrimSpace(right.Family))
}

func sortedFrameworkVersions(
	versions map[string]FrameworkVersionRecord,
	descending bool,
) []string {
	result := make([]string, 0, len(versions))
	for version := range versions {
		result = append(result, version)
	}
	sort.SliceStable(result, func(i, j int) bool {
		comparison := compareSemanticVersions(result[i], result[j])
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
	return result
}

func compareSemanticVersions(left, right string) int {
	leftParts := strings.Split(strings.TrimSpace(left), ".")
	rightParts := strings.Split(strings.TrimSpace(right), ".")
	for index := 0; index < 3; index++ {
		leftPart := normalizedSemanticVersionPart(leftParts, index)
		rightPart := normalizedSemanticVersionPart(rightParts, index)
		if len(leftPart) < len(rightPart) {
			return -1
		}
		if len(leftPart) > len(rightPart) {
			return 1
		}
		if comparison := strings.Compare(leftPart, rightPart); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func normalizedSemanticVersionPart(parts []string, index int) string {
	if index >= len(parts) {
		return "0"
	}
	value := strings.TrimLeft(parts[index], "0")
	if value == "" {
		return "0"
	}
	return value
}

func normalizeFrameworkID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedLifecycleTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC().Round(0)
}

func compactLifecycleText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(safety.RedactSecrets(value))), " ")
	runes := []rune(value)
	if limit > 0 && len(runes) > limit {
		value = string(runes[:limit])
	}
	return value
}

func compactLifecycleStrings(values []string, maxItems, maxRunes int) []string {
	result := make([]string, 0, len(values))
	for _, value := range uniqueStrings(values) {
		value = compactLifecycleText(value, maxRunes)
		if value == "" {
			continue
		}
		result = append(result, value)
		if len(result) == maxItems {
			break
		}
	}
	return result
}

func cloneFrameworkVersionRecord(record FrameworkVersionRecord) FrameworkVersionRecord {
	cloned := record
	cloned.Framework = cloneFramework(record.Framework)
	cloned.Migration.MigrationSteps = append([]string(nil), record.Migration.MigrationSteps...)
	cloned.Migration.ValidationCriteria = append(
		[]string(nil),
		record.Migration.ValidationCriteria...,
	)
	cloned.ActivatedAt = cloneTimePointer(record.ActivatedAt)
	cloned.RetiredAt = cloneTimePointer(record.RetiredAt)
	return cloned
}

func cloneFrameworkLifecycleEvent(
	event FrameworkLifecycleEvent,
) FrameworkLifecycleEvent {
	cloned := event
	cloned.Migration.MigrationSteps = append([]string(nil), event.Migration.MigrationSteps...)
	cloned.Migration.ValidationCriteria = append(
		[]string(nil),
		event.Migration.ValidationCriteria...,
	)
	return cloned
}
