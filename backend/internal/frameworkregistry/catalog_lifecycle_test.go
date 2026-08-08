package frameworkregistry

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCatalogBootstrapSelectsHighestVersionAndPreservesIDOnlyAPI(t *testing.T) {
	catalog := BuiltinCatalog()
	versionOne := cloneFramework(catalog[0])
	versionTwo := nextFrameworkVersion(versionOne, "2.0.0")
	catalog = append(catalog, versionTwo)

	service, err := NewServiceWithCatalog(NewMemoryRepository(), catalog)
	if err != nil {
		t.Fatalf("NewServiceWithCatalog: %v", err)
	}
	views, err := service.List("alice")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 55 {
		t.Fatalf("ID-only List returned %d records, want 55", len(views))
	}
	active, err := service.Get("alice", versionOne.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if active.Version != "2.0.0" {
		t.Fatalf("ID-only Get selected %s, want deterministic highest version 2.0.0", active.Version)
	}
	versions, err := service.FrameworkVersions("alice", versionOne.ID)
	if err != nil {
		t.Fatalf("FrameworkVersions: %v", err)
	}
	if len(versions) != 2 ||
		versions[0].Version != "2.0.0" ||
		versions[0].LifecycleState != FrameworkVersionActive ||
		versions[1].Version != "1.0.0" ||
		versions[1].LifecycleState != FrameworkVersionRetired {
		t.Fatalf("version projection = %#v", versions)
	}
	history, err := service.FrameworkLifecycleHistory(versionOne.ID, 100)
	if err != nil {
		t.Fatalf("FrameworkLifecycleHistory: %v", err)
	}
	if err := VerifyFrameworkLifecycleHistory(history); err != nil {
		t.Fatalf("bootstrap history verification: %v", err)
	}
}

func TestStageActivateAndRollbackPreserveImmutableVersionHistory(t *testing.T) {
	now := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	service, err := newServiceWithCatalog(NewMemoryRepository(), BuiltinCatalog(), clock)
	if err != nil {
		t.Fatalf("newServiceWithCatalog: %v", err)
	}
	before, err := service.Get("alice", "human-sovereignty")
	if err != nil {
		t.Fatalf("Get initial: %v", err)
	}
	lowered := 1
	if _, err := service.UpdatePreference("alice", before.ID, PreferencePatch{
		MaximumAutonomyLevel: &lowered,
	}); err != nil {
		t.Fatalf("UpdatePreference: %v", err)
	}

	versionTwo := nextFrameworkVersion(before.Framework, "2.0.0")
	staged, err := service.StageFrameworkVersion(StageFrameworkVersionRequest{
		Framework: versionTwo,
		Actor:     "alice",
		Migration: testFrameworkMigration("1.0.0"),
		Provenance: FrameworkVersionProvenance{
			Source:     "owner-reviewed architecture update",
			Reference:  "review/frameworks/human-sovereignty-v2",
			AuthoredBy: "architecture working group",
		},
	})
	if err != nil {
		t.Fatalf("StageFrameworkVersion: %v", err)
	}
	if staged.LifecycleState != FrameworkVersionStaged ||
		staged.VersionProvenance.ContentDigest == "" ||
		staged.VersionProvenance.ParentDigest == "" {
		t.Fatalf("staged record = %#v", staged)
	}
	staged.Framework.Purpose = "caller mutation"
	staged.Migration.ValidationCriteria[0] = "caller mutation"

	stillActive, err := service.Get("alice", before.ID)
	if err != nil {
		t.Fatalf("Get after stage: %v", err)
	}
	if stillActive.Version != "1.0.0" {
		t.Fatalf("staging changed ID-only lookup to %s", stillActive.Version)
	}
	internalStaged, err := service.GetFrameworkVersion("alice", before.ID, "2.0.0")
	if err != nil {
		t.Fatalf("GetFrameworkVersion staged: %v", err)
	}
	if internalStaged.Purpose == "caller mutation" ||
		internalStaged.Migration.ValidationCriteria[0] == "caller mutation" {
		t.Fatal("returned staged record shared mutable state with registry")
	}
	if internalStaged.Enabled {
		t.Fatal("staged version must not become effectively enabled")
	}

	if _, err := service.ActivateFrameworkVersion(
		before.ID,
		"2.0.0",
		ActivateFrameworkVersionRequest{
			Actor:                 "alice",
			Reason:                "Reviewed migration and validation evidence.",
			ExpectedActiveVersion: "0.9.0",
			ExpectedTargetDigest:  internalStaged.VersionProvenance.ContentDigest,
		},
	); err == nil || !strings.Contains(err.Error(), "active framework version changed") {
		t.Fatalf("stale activation precondition error = %v", err)
	}

	activated, err := service.ActivateFrameworkVersion(
		before.ID,
		"2.0.0",
		ActivateFrameworkVersionRequest{
			Actor:                 "alice",
			Reason:                "Reviewed migration and validation evidence.",
			ExpectedActiveVersion: "1.0.0",
			ExpectedTargetDigest:  internalStaged.VersionProvenance.ContentDigest,
		},
	)
	if err != nil {
		t.Fatalf("ActivateFrameworkVersion: %v", err)
	}
	if activated.LifecycleState != FrameworkVersionActive {
		t.Fatalf("activated lifecycle state = %q", activated.LifecycleState)
	}
	activeView, err := service.Get("alice", before.ID)
	if err != nil {
		t.Fatalf("Get after activation: %v", err)
	}
	if activeView.Version != "2.0.0" || activeView.EffectiveAutonomyLevel != lowered {
		t.Fatalf("ID-only view did not follow activation and preference: %#v", activeView)
	}
	versionOne, err := service.GetFrameworkVersion("alice", before.ID, "1.0.0")
	if err != nil {
		t.Fatalf("GetFrameworkVersion retired v1: %v", err)
	}
	if versionOne.LifecycleState != FrameworkVersionRetired || versionOne.Enabled {
		t.Fatalf("retired v1 view = %#v", versionOne)
	}

	historyBeforeRollback, err := service.FrameworkLifecycleHistory(before.ID, 100)
	if err != nil {
		t.Fatalf("history before rollback: %v", err)
	}
	rolledBack, err := service.RollbackFrameworkVersion(
		before.ID,
		"1.0.0",
		RollbackFrameworkVersionRequest{
			Actor:                 "alice",
			Reason:                "Validation identified a regression in version two.",
			ExpectedActiveVersion: "2.0.0",
			ExpectedTargetDigest:  versionOne.VersionProvenance.ContentDigest,
		},
	)
	if err != nil {
		t.Fatalf("RollbackFrameworkVersion: %v", err)
	}
	if rolledBack.LifecycleState != FrameworkVersionActive {
		t.Fatalf("rollback target state = %q", rolledBack.LifecycleState)
	}
	afterRollback, err := service.Get("alice", before.ID)
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	if afterRollback.Version != "1.0.0" {
		t.Fatalf("ID-only Get after rollback = %s", afterRollback.Version)
	}
	historyAfterRollback, err := service.FrameworkLifecycleHistory(before.ID, 100)
	if err != nil {
		t.Fatalf("history after rollback: %v", err)
	}
	if len(historyAfterRollback) != len(historyBeforeRollback)+1 {
		t.Fatalf(
			"history length after rollback = %d, want %d",
			len(historyAfterRollback),
			len(historyBeforeRollback)+1,
		)
	}
	if !reflect.DeepEqual(historyAfterRollback[1:], historyBeforeRollback) {
		t.Fatal("rollback modified prior lifecycle history")
	}
	if historyAfterRollback[0].Action != FrameworkLifecycleRolledBack {
		t.Fatalf("latest lifecycle action = %q", historyAfterRollback[0].Action)
	}
	if err := VerifyFrameworkLifecycleHistory(historyAfterRollback); err != nil {
		t.Fatalf("history verification after rollback: %v", err)
	}
}

func TestRetireStagedVersionAndRejectInvalidMigrationMetadata(t *testing.T) {
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	active, err := service.ActiveFrameworkVersion("human-sovereignty")
	if err != nil {
		t.Fatalf("ActiveFrameworkVersion: %v", err)
	}
	versionTwo := nextFrameworkVersion(active.Framework, "2.0.0")
	if _, err := service.StageFrameworkVersion(StageFrameworkVersionRequest{
		Framework: versionTwo,
		Actor:     "alice",
		Migration: FrameworkMigrationMetadata{
			FromVersion:        "1.0.0",
			Strategy:           frameworkMigrationManual,
			ChangeSummary:      "Missing migration steps.",
			CompatibilityNotes: "Manual review is required.",
			ValidationCriteria: []string{"review succeeds"},
		},
		Provenance: FrameworkVersionProvenance{
			Source:     "test",
			AuthoredBy: "alice",
		},
	}); err == nil || !strings.Contains(err.Error(), "require migration steps") {
		t.Fatalf("invalid migration error = %v", err)
	}

	staged, err := service.StageFrameworkVersion(StageFrameworkVersionRequest{
		Framework: versionTwo,
		Actor:     "alice",
		Migration: testFrameworkMigration("1.0.0"),
		Provenance: FrameworkVersionProvenance{
			Source:     "owner-reviewed architecture update",
			AuthoredBy: "alice",
		},
	})
	if err != nil {
		t.Fatalf("StageFrameworkVersion: %v", err)
	}
	retired, err := service.RetireFrameworkVersion(
		active.ID,
		"2.0.0",
		RetireFrameworkVersionRequest{
			Actor:                "alice",
			Reason:               "Superseded before activation.",
			ExpectedTargetDigest: staged.VersionProvenance.ContentDigest,
		},
	)
	if err != nil {
		t.Fatalf("RetireFrameworkVersion: %v", err)
	}
	if retired.LifecycleState != FrameworkVersionRetired {
		t.Fatalf("retired lifecycle state = %q", retired.LifecycleState)
	}
	if _, err := service.RollbackFrameworkVersion(
		active.ID,
		"2.0.0",
		RollbackFrameworkVersionRequest{
			Actor:                 "alice",
			Reason:                "This version was never active.",
			ExpectedActiveVersion: "1.0.0",
			ExpectedTargetDigest:  staged.VersionProvenance.ContentDigest,
		},
	); err == nil || !strings.Contains(err.Error(), "never active") {
		t.Fatalf("never-active rollback error = %v", err)
	}
}

func TestFrameworkLifecycleHistoryDetectsTampering(t *testing.T) {
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	history, err := service.FrameworkLifecycleHistory("human-sovereignty", 100)
	if err != nil {
		t.Fatalf("FrameworkLifecycleHistory: %v", err)
	}
	if err := VerifyFrameworkLifecycleHistory(history); err != nil {
		t.Fatalf("valid history rejected: %v", err)
	}
	bounded, err := service.FrameworkLifecycleHistory("human-sovereignty", 1)
	if err != nil {
		t.Fatalf("bounded FrameworkLifecycleHistory: %v", err)
	}
	if err := VerifyFrameworkLifecycleHistory(bounded); err != nil {
		t.Fatalf("valid bounded history rejected: %v", err)
	}
	history[0].Reason = "tampered"
	if err := VerifyFrameworkLifecycleHistory(history); err == nil {
		t.Fatal("tampered history was accepted")
	}
}

func nextFrameworkVersion(current Framework, version string) Framework {
	next := cloneFramework(current)
	next.Version = version
	next.Purpose += " This reviewed version exercises lifecycle migration."
	return next
}

func testFrameworkMigration(fromVersion string) FrameworkMigrationMetadata {
	return FrameworkMigrationMetadata{
		FromVersion:        fromVersion,
		Strategy:           frameworkMigrationCompatible,
		ChangeSummary:      "Clarify lifecycle behavior without raising authority.",
		CompatibilityNotes: "Existing ID-only callers continue to receive one active framework.",
		MigrationSteps:     []string{"review the new immutable contract"},
		ValidationCriteria: []string{"ID-only lookup resolves the activated version"},
	}
}
