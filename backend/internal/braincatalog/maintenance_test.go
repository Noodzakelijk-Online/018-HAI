package braincatalog

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"testing"
	"time"
)

type fakeCatalogReviewHistory struct {
	latest  map[string]models.BrainCatalogUpstreamReview
	recorded []models.BrainCatalogUpstreamReview
}

func (r *fakeCatalogReviewHistory) RecordUpstreamReview(record *models.BrainCatalogUpstreamReview) (*models.BrainCatalogUpstreamReview, error) {
	if record == nil {
		return nil, fmt.Errorf("record required")
	}
	if r.latest == nil {
		r.latest = map[string]models.BrainCatalogUpstreamReview{}
	}
	r.latest[record.CatalogEntryID] = *record
	r.recorded = append(r.recorded, *record)
	return record, nil
}

func (r *fakeCatalogReviewHistory) FindLatestUpstreamReview(id string) (*models.BrainCatalogUpstreamReview, error) {
	record, ok := r.latest[id]
	if !ok {
		return nil, nil
	}
	return &record, nil
}

func (r *fakeCatalogReviewHistory) FindRecentUpstreamReviews(limit int) ([]models.BrainCatalogUpstreamReview, error) {
	if limit > len(r.recorded) {
		limit = len(r.recorded)
	}
	return append([]models.BrainCatalogUpstreamReview(nil), r.recorded[:limit]...), nil
}

type fakeCatalogReviewer struct {
	calls int
	fail  bool
}

func (r *fakeCatalogReviewer) Review(entry Entry) (UpstreamReview, error) {
	r.calls++
	if r.fail {
		return UpstreamReview{}, fmt.Errorf("temporary GitHub failure")
	}
	review := UpstreamReview{ID: entry.ID, Name: entry.Name, UpstreamURL: entry.UpstreamURL, Available: true, License: "MIT", Disposition: entry.Status, CheckedAt: "2026-07-21T13:00:00Z"}
	applyReadinessAssessment(entry, &review)
	return review, nil
}

type fakeCollectionReviewHistory struct {
	latest   *models.BrainCatalogCollectionReview
	recorded []models.BrainCatalogCollectionReview
}

func (r *fakeCollectionReviewHistory) RecordCollectionReview(record *models.BrainCatalogCollectionReview) (*models.BrainCatalogCollectionReview, error) {
	if record == nil {
		return nil, fmt.Errorf("record required")
	}
	copy := *record
	r.latest = &copy
	r.recorded = append(r.recorded, copy)
	return record, nil
}

func (r *fakeCollectionReviewHistory) FindLatestCollectionReview() (*models.BrainCatalogCollectionReview, error) {
	if r.latest == nil {
		return nil, nil
	}
	copy := *r.latest
	return &copy, nil
}

func (r *fakeCollectionReviewHistory) FindRecentCollectionReviews(limit int) ([]models.BrainCatalogCollectionReview, error) {
	if limit > len(r.recorded) {
		limit = len(r.recorded)
	}
	return append([]models.BrainCatalogCollectionReview(nil), r.recorded[:limit]...), nil
}

type fakeScheduledCollectionReviewer struct {
	calls int
	fail  bool
}

type fakeRepositoryDiscoveryReviewHistory struct {
	latest   *models.BrainCatalogRepositoryDiscoveryReview
	recorded []models.BrainCatalogRepositoryDiscoveryReview
}

func (r *fakeRepositoryDiscoveryReviewHistory) RecordRepositoryDiscoveryReview(record *models.BrainCatalogRepositoryDiscoveryReview) (*models.BrainCatalogRepositoryDiscoveryReview, error) {
	if record == nil {
		return nil, fmt.Errorf("record required")
	}
	copy := *record
	r.latest = &copy
	r.recorded = append(r.recorded, copy)
	return record, nil
}

func (r *fakeRepositoryDiscoveryReviewHistory) FindLatestRepositoryDiscoveryReview() (*models.BrainCatalogRepositoryDiscoveryReview, error) {
	if r.latest == nil {
		return nil, nil
	}
	copy := *r.latest
	return &copy, nil
}

func (r *fakeRepositoryDiscoveryReviewHistory) FindRecentRepositoryDiscoveryReviews(limit int) ([]models.BrainCatalogRepositoryDiscoveryReview, error) {
	if limit > len(r.recorded) {
		limit = len(r.recorded)
	}
	return append([]models.BrainCatalogRepositoryDiscoveryReview(nil), r.recorded[:limit]...), nil
}

type fakeScheduledRepositoryScout struct {
	calls int
	fail  bool
}

func (s *fakeScheduledRepositoryScout) DiscoverRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	return s.DiscoverReviewableRepositories()
}

func (s *fakeScheduledRepositoryScout) DiscoverReviewableRepositories() (OSSInsightRepositoryDiscoveryReport, error) {
	s.calls++
	if s.fail {
		return OSSInsightRepositoryDiscoveryReport{}, fmt.Errorf("temporary OSS Insight failure")
	}
	return OSSInsightRepositoryDiscoveryReport{
		CheckedAt: "2026-07-21T13:00:00Z", SourceURL: ossInsightCollectionsURL,
		Scope: OSSInsightReviewableScope, Available: true, CollectionsScreened: 138,
		EligibleCollections: 36, CollectionsChecked: 36, RepositoriesChecked: 116,
		KnownProfileHits: 112, Discoveries: []OSSInsightRepositoryDiscovery{
			{Repository: "owner/review-me"}, {Repository: "owner/second-candidate"},
		},
		Message: "review only",
	}, nil
}

func (s *fakeScheduledRepositoryScout) DiscoverRepositoriesFor(_ OSSInsightDiscoveryScope) (OSSInsightRepositoryDiscoveryReport, error) {
	return s.DiscoverReviewableRepositories()
}

func (r *fakeScheduledCollectionReviewer) ReviewCollections() (OSSInsightCollectionReview, error) {
	r.calls++
	if r.fail {
		return OSSInsightCollectionReview{}, fmt.Errorf("temporary OSS Insight failure")
	}
	return OSSInsightCollectionReview{
		CheckedAt: "2026-07-21T13:00:00Z", SourceURL: ossInsightCollectionsURL, Available: true,
		ExpectedTotal: 138, CurrentTotal: 138, Message: "source snapshot matches",
	}, nil
}

func TestCatalogMaintenanceIsBoundedAndReusesDailyEvidence(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "true")
	t.Setenv("HAI_CATALOG_REVALIDATION_BATCH_SIZE", "2")
	now := time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC)
	history := &fakeCatalogReviewHistory{}
	reviewer := &fakeCatalogReviewer{}
	service := NewCatalogMaintenanceService(reviewer, history)
	service.now = func() time.Time { return now }
	service.entries = func() []Entry {
		return []Entry{
			{ID: "one", Name: "One", UpstreamURL: "https://github.com/owner/one", Status: StatusCandidate},
			{ID: "two", Name: "Two", UpstreamURL: "https://github.com/owner/two", Status: StatusCandidate},
			{ID: "three", Name: "Three", UpstreamURL: "https://github.com/owner/three", Status: StatusCandidate},
		}
	}

	first := service.RunDueRevalidations()
	if !first.Enabled || first.Eligible != 3 || first.Checked != 2 || first.Reused != 0 || first.Failed != 0 || reviewer.calls != 2 {
		t.Fatalf("first bounded run = %#v, calls=%d", first, reviewer.calls)
	}
	if len(history.recorded) != 2 || history.recorded[0].CatalogEntryID != "one" || history.recorded[0].Disposition != string(StatusCandidate) {
		t.Fatalf("unexpected durable evidence: %#v", history.recorded)
	}

	second := service.RunDueRevalidations()
	if second.Checked != 1 || second.Reused != 2 || reviewer.calls != 3 {
		t.Fatalf("second run should check the next stale entry: %#v, calls=%d", second, reviewer.calls)
	}

	third := service.RunDueRevalidations()
	if third.Checked != 0 || third.Reused != 3 || reviewer.calls != 3 {
		t.Fatalf("daily evidence should suppress provider calls: %#v, calls=%d", third, reviewer.calls)
	}
}

func TestCatalogMaintenanceFailureDoesNotChangeCatalogDisposition(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "true")
	history := &fakeCatalogReviewHistory{}
	service := NewCatalogMaintenanceService(&fakeCatalogReviewer{fail: true}, history)
	service.now = func() time.Time { return time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC) }
	service.entries = func() []Entry {
		return []Entry{{ID: "candidate", Name: "Candidate", UpstreamURL: "https://github.com/owner/candidate", Status: StatusCandidate}}
	}

	run := service.RunDueRevalidations()
	if run.Checked != 1 || run.Failed != 1 || len(history.recorded) != 1 {
		t.Fatalf("failure run = %#v, records=%#v", run, history.recorded)
	}
	record := history.recorded[0]
	if record.Disposition != string(StatusCandidate) || record.Available || record.Message == "" {
		t.Fatalf("failure evidence changed catalog disposition: %#v", record)
	}
}

func TestCatalogMaintenanceDisabledDoesNotCallUpstream(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "false")
	reviewer := &fakeCatalogReviewer{}
	service := NewCatalogMaintenanceService(reviewer, &fakeCatalogReviewHistory{})
	service.entries = func() []Entry {
		return []Entry{{ID: "candidate", Name: "Candidate", UpstreamURL: "https://github.com/owner/candidate", Status: StatusCandidate}}
	}
	if run := service.RunDueRevalidations(); run.Enabled || run.Checked != 0 || reviewer.calls != 0 {
		t.Fatalf("disabled catalog maintenance performed external work: %#v calls=%d", run, reviewer.calls)
	}
}

func TestCatalogCollectionMaintenancePersistsAndReusesDailySourceEvidence(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "true")
	t.Setenv("HAI_CATALOG_COLLECTION_REVALIDATION_ENABLED", "true")
	now := time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC)
	collectionHistory := &fakeCollectionReviewHistory{}
	collectionReviewer := &fakeScheduledCollectionReviewer{}
	service := NewCatalogMaintenanceService(&fakeCatalogReviewer{}, &fakeCatalogReviewHistory{}).
		WithCollectionMaintenance(collectionReviewer, collectionHistory)
	service.now = func() time.Time { return now }
	service.entries = func() []Entry { return nil }

	first := service.RunDueCollectionRevalidation()
	if !first.Enabled || first.Reused || first.Failed || !first.Review.Available || collectionReviewer.calls != 1 || len(collectionHistory.recorded) != 1 {
		t.Fatalf("first collection maintenance = %#v calls=%d records=%#v", first, collectionReviewer.calls, collectionHistory.recorded)
	}
	if collectionHistory.recorded[0].SourceURL != ossInsightCollectionsURL || collectionHistory.recorded[0].CurrentTotal != 138 {
		t.Fatalf("unexpected persisted collection evidence: %#v", collectionHistory.recorded[0])
	}

	second := service.RunDueCollectionRevalidation()
	if !second.Enabled || !second.Reused || second.Failed || collectionReviewer.calls != 1 || second.Review.CurrentTotal != 138 {
		t.Fatalf("daily collection evidence was not reused: %#v calls=%d", second, collectionReviewer.calls)
	}
}

func TestCatalogCollectionMaintenanceStaysDisabledWithoutBothOptIns(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "false")
	t.Setenv("HAI_CATALOG_COLLECTION_REVALIDATION_ENABLED", "true")
	collectionReviewer := &fakeScheduledCollectionReviewer{}
	service := NewCatalogMaintenanceService(&fakeCatalogReviewer{}, &fakeCatalogReviewHistory{}).
		WithCollectionMaintenance(collectionReviewer, &fakeCollectionReviewHistory{})
	if run := service.RunDueCollectionRevalidation(); run.Enabled || collectionReviewer.calls != 0 {
		t.Fatalf("disabled collection maintenance performed external work: %#v calls=%d", run, collectionReviewer.calls)
	}
}

func TestCatalogRepositoryDiscoveryMaintenancePersistsAndReusesDailyEvidence(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "true")
	t.Setenv("HAI_CATALOG_REPOSITORY_DISCOVERY_REVALIDATION_ENABLED", "true")
	now := time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC)
	history := &fakeRepositoryDiscoveryReviewHistory{}
	scout := &fakeScheduledRepositoryScout{}
	service := NewCatalogMaintenanceService(&fakeCatalogReviewer{}, &fakeCatalogReviewHistory{}).
		WithRepositoryDiscoveryMaintenance(scout, history)
	service.now = func() time.Time { return now }

	first := service.RunDueRepositoryDiscoveryRevalidation()
	if !first.Enabled || first.Reused || first.Failed || !first.Review.Available || scout.calls != 1 || len(history.recorded) != 1 {
		t.Fatalf("first repository discovery maintenance = %#v calls=%d records=%#v", first, scout.calls, history.recorded)
	}
	if first.Review.UnreviewedDiscoveries != 2 || len(first.Review.CandidateRepositories) != 2 || first.Review.CandidateRepositories[0] != "owner/review-me" {
		t.Fatalf("unexpected persisted repository discovery evidence: %#v", first.Review)
	}

	second := service.RunDueRepositoryDiscoveryRevalidation()
	if !second.Enabled || !second.Reused || second.Failed || scout.calls != 1 || second.Review.RepositoriesChecked != 116 {
		t.Fatalf("daily repository discovery evidence was not reused: %#v calls=%d", second, scout.calls)
	}
}

func TestCatalogRepositoryDiscoveryMaintenanceStaysDisabledWithoutBothOptIns(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_ENABLED", "false")
	t.Setenv("HAI_CATALOG_REPOSITORY_DISCOVERY_REVALIDATION_ENABLED", "true")
	scout := &fakeScheduledRepositoryScout{}
	service := NewCatalogMaintenanceService(&fakeCatalogReviewer{}, &fakeCatalogReviewHistory{}).
		WithRepositoryDiscoveryMaintenance(scout, &fakeRepositoryDiscoveryReviewHistory{})
	if run := service.RunDueRepositoryDiscoveryRevalidation(); run.Enabled || scout.calls != 0 {
		t.Fatalf("disabled repository discovery maintenance performed external work: %#v calls=%d", run, scout.calls)
	}
}

func TestCatalogRevalidationIntervalCannotExceedOneDay(t *testing.T) {
	t.Setenv("HAI_CATALOG_REVALIDATION_INTERVAL_HOURS", "999")
	if got := catalogRevalidationInterval(); got != 24*time.Hour {
		t.Fatalf("interval=%s, want 24h", got)
	}
	t.Setenv("HAI_CATALOG_REVALIDATION_INTERVAL_HOURS", "0")
	if got := catalogRevalidationInterval(); got != time.Hour {
		t.Fatalf("interval=%s, want 1h", got)
	}
}
