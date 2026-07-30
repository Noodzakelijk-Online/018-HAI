package braincatalog

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// UpstreamReviewHistoryRepository keeps upstream drift evidence separate from
// the static catalog. A review cannot alter a catalog disposition by itself.
type UpstreamReviewHistoryRepository interface {
	RecordUpstreamReview(*models.BrainCatalogUpstreamReview) (*models.BrainCatalogUpstreamReview, error)
	FindLatestUpstreamReview(catalogEntryID string) (*models.BrainCatalogUpstreamReview, error)
	FindRecentUpstreamReviews(limit int) ([]models.BrainCatalogUpstreamReview, error)
}

// CollectionReviewHistoryRepository retains only the bounded source-index
// evidence needed to distinguish a fresh OSS Insight snapshot from source
// drift. It is separate from fixed-upstream metadata history.
type CollectionReviewHistoryRepository interface {
	RecordCollectionReview(*models.BrainCatalogCollectionReview) (*models.BrainCatalogCollectionReview, error)
	FindLatestCollectionReview() (*models.BrainCatalogCollectionReview, error)
	FindRecentCollectionReviews(limit int) ([]models.BrainCatalogCollectionReview, error)
}

// RepositoryDiscoveryReviewHistoryRepository preserves only the compact,
// read-only result of a daily OSS Insight repository gap review.
type RepositoryDiscoveryReviewHistoryRepository interface {
	RecordRepositoryDiscoveryReview(*models.BrainCatalogRepositoryDiscoveryReview) (*models.BrainCatalogRepositoryDiscoveryReview, error)
	FindLatestRepositoryDiscoveryReview() (*models.BrainCatalogRepositoryDiscoveryReview, error)
	FindRecentRepositoryDiscoveryReviews(limit int) ([]models.BrainCatalogRepositoryDiscoveryReview, error)
}

type gormUpstreamReviewHistoryRepository struct {
	db *gorm.DB
}

type gormCollectionReviewHistoryRepository struct {
	db *gorm.DB
}

type gormRepositoryDiscoveryReviewHistoryRepository struct {
	db *gorm.DB
}

func NewGormUpstreamReviewHistoryRepository(db *gorm.DB) UpstreamReviewHistoryRepository {
	return &gormUpstreamReviewHistoryRepository{db: db}
}

func NewGormCollectionReviewHistoryRepository(db *gorm.DB) CollectionReviewHistoryRepository {
	return &gormCollectionReviewHistoryRepository{db: db}
}

func NewGormRepositoryDiscoveryReviewHistoryRepository(db *gorm.DB) RepositoryDiscoveryReviewHistoryRepository {
	return &gormRepositoryDiscoveryReviewHistoryRepository{db: db}
}

func DefaultUpstreamReviewHistoryRepository() (UpstreamReviewHistoryRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize brain catalog upstream review history: %w", err)
	}
	return NewGormUpstreamReviewHistoryRepository(db), nil
}

func DefaultCollectionReviewHistoryRepository() (CollectionReviewHistoryRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize brain catalog collection review history: %w", err)
	}
	return NewGormCollectionReviewHistoryRepository(db), nil
}

func DefaultRepositoryDiscoveryReviewHistoryRepository() (RepositoryDiscoveryReviewHistoryRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize brain catalog repository discovery review history: %w", err)
	}
	return NewGormRepositoryDiscoveryReviewHistoryRepository(db), nil
}

func (r *gormUpstreamReviewHistoryRepository) RecordUpstreamReview(record *models.BrainCatalogUpstreamReview) (*models.BrainCatalogUpstreamReview, error) {
	if record == nil || strings.TrimSpace(record.CatalogEntryID) == "" || strings.TrimSpace(record.UpstreamURL) == "" {
		return nil, fmt.Errorf("catalog entry id and upstream URL are required")
	}
	if record.CheckedAt.IsZero() {
		record.CheckedAt = time.Now().UTC()
	}
	record.CheckedAt = record.CheckedAt.UTC()
	if err := r.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *gormUpstreamReviewHistoryRepository) FindLatestUpstreamReview(catalogEntryID string) (*models.BrainCatalogUpstreamReview, error) {
	catalogEntryID = strings.TrimSpace(catalogEntryID)
	if catalogEntryID == "" {
		return nil, fmt.Errorf("catalog entry id is required")
	}
	var record models.BrainCatalogUpstreamReview
	result := r.db.Where("catalog_entry_id = ?", catalogEntryID).Order("checked_at DESC").Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func (r *gormUpstreamReviewHistoryRepository) FindRecentUpstreamReviews(limit int) ([]models.BrainCatalogUpstreamReview, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []models.BrainCatalogUpstreamReview
	if err := r.db.Order("checked_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *gormCollectionReviewHistoryRepository) RecordCollectionReview(record *models.BrainCatalogCollectionReview) (*models.BrainCatalogCollectionReview, error) {
	if record == nil || strings.TrimSpace(record.SourceURL) == "" {
		return nil, fmt.Errorf("collection review source URL is required")
	}
	if record.CheckedAt.IsZero() {
		record.CheckedAt = time.Now().UTC()
	}
	record.CheckedAt = record.CheckedAt.UTC()
	if err := r.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *gormCollectionReviewHistoryRepository) FindLatestCollectionReview() (*models.BrainCatalogCollectionReview, error) {
	var record models.BrainCatalogCollectionReview
	result := r.db.Order("checked_at DESC").Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func (r *gormCollectionReviewHistoryRepository) FindRecentCollectionReviews(limit int) ([]models.BrainCatalogCollectionReview, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []models.BrainCatalogCollectionReview
	if err := r.db.Order("checked_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *gormRepositoryDiscoveryReviewHistoryRepository) RecordRepositoryDiscoveryReview(record *models.BrainCatalogRepositoryDiscoveryReview) (*models.BrainCatalogRepositoryDiscoveryReview, error) {
	if record == nil || strings.TrimSpace(record.SourceURL) == "" || strings.TrimSpace(record.Scope) == "" {
		return nil, fmt.Errorf("repository discovery review source URL and scope are required")
	}
	if record.CheckedAt.IsZero() {
		record.CheckedAt = time.Now().UTC()
	}
	record.CheckedAt = record.CheckedAt.UTC()
	if err := r.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *gormRepositoryDiscoveryReviewHistoryRepository) FindLatestRepositoryDiscoveryReview() (*models.BrainCatalogRepositoryDiscoveryReview, error) {
	var record models.BrainCatalogRepositoryDiscoveryReview
	result := r.db.Order("checked_at DESC").Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func (r *gormRepositoryDiscoveryReviewHistoryRepository) FindRecentRepositoryDiscoveryReviews(limit int) ([]models.BrainCatalogRepositoryDiscoveryReview, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []models.BrainCatalogRepositoryDiscoveryReview
	if err := r.db.Order("checked_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func upstreamReviewRecord(entry Entry, review UpstreamReview) *models.BrainCatalogUpstreamReview {
	gates, _ := json.Marshal(review.RequiredGates)
	checkedAt, err := time.Parse(time.RFC3339, review.CheckedAt)
	if err != nil {
		checkedAt = time.Now().UTC()
	}
	return &models.BrainCatalogUpstreamReview{
		CatalogEntryID: entry.ID, Name: review.Name, UpstreamURL: review.UpstreamURL,
		ResolvedRepository: review.ResolvedRepository, ResolvedUpstreamURL: review.ResolvedUpstreamURL,
		RepositoryMoved: review.RepositoryMoved, Available: review.Available, Archived: review.Archived,
		License: review.License, DefaultBranch: review.DefaultBranch, PushedAt: review.PushedAt,
		Message: review.Message, Disposition: string(review.Disposition), Readiness: review.Readiness,
		ReadinessReason: review.ReadinessReason, RequiredGatesJSON: string(gates), CheckedAt: checkedAt.UTC(),
	}
}

func upstreamReviewFromRecord(record models.BrainCatalogUpstreamReview) UpstreamReview {
	var gates []string
	_ = json.Unmarshal([]byte(record.RequiredGatesJSON), &gates)
	return UpstreamReview{
		ID: record.CatalogEntryID, Name: record.Name, UpstreamURL: record.UpstreamURL,
		ResolvedRepository: record.ResolvedRepository, ResolvedUpstreamURL: record.ResolvedUpstreamURL,
		RepositoryMoved: record.RepositoryMoved, CheckedAt: record.CheckedAt.UTC().Format(time.RFC3339),
		Available: record.Available, Archived: record.Archived, License: record.License,
		DefaultBranch: record.DefaultBranch, PushedAt: record.PushedAt, Message: record.Message,
		Disposition: Status(record.Disposition), Readiness: record.Readiness, ReadinessReason: record.ReadinessReason,
		RequiredGates: gates,
	}
}

func collectionReviewRecord(review OSSInsightCollectionReview) *models.BrainCatalogCollectionReview {
	newCollections, _ := json.Marshal(review.NewCollections)
	missingExpected, _ := json.Marshal(review.MissingExpected)
	checkedAt, err := time.Parse(time.RFC3339, review.CheckedAt)
	if err != nil {
		checkedAt = time.Now().UTC()
	}
	return &models.BrainCatalogCollectionReview{
		SourceURL: review.SourceURL, Available: review.Available, ExpectedTotal: review.ExpectedTotal,
		CurrentTotal: review.CurrentTotal, NewCollectionsJSON: string(newCollections), MissingExpectedJSON: string(missingExpected),
		Message: review.Message, CheckedAt: checkedAt.UTC(),
	}
}

func collectionReviewFromRecord(record models.BrainCatalogCollectionReview) OSSInsightCollectionReview {
	var newCollections, missingExpected []string
	_ = json.Unmarshal([]byte(record.NewCollectionsJSON), &newCollections)
	_ = json.Unmarshal([]byte(record.MissingExpectedJSON), &missingExpected)
	return OSSInsightCollectionReview{
		CheckedAt: record.CheckedAt.UTC().Format(time.RFC3339), SourceURL: record.SourceURL, Available: record.Available,
		ExpectedTotal: record.ExpectedTotal, CurrentTotal: record.CurrentTotal, NewCollections: newCollections,
		MissingExpected: missingExpected, Message: record.Message,
	}
}

const maxPersistedRepositoryDiscoveryCandidates = 30

// OSSInsightRepositoryDiscoveryMaintenanceReview is intentionally smaller
// than an interactive discovery report. It provides durable evidence without
// retaining an upstream's full response or treating a candidate as adopted.
type OSSInsightRepositoryDiscoveryMaintenanceReview struct {
	CheckedAt               string                   `json:"checkedAt"`
	SourceURL               string                   `json:"sourceUrl"`
	Scope                   OSSInsightDiscoveryScope `json:"scope"`
	Available               bool                     `json:"available"`
	CollectionsScreened     int                      `json:"collectionsScreened"`
	EligibleCollections     int                      `json:"eligibleCollections"`
	CollectionsChecked      int                      `json:"collectionsChecked"`
	RepositoriesChecked     int                      `json:"repositoriesChecked"`
	KnownProfileHits        int                      `json:"knownProfileHits"`
	UnreviewedDiscoveries   int                      `json:"unreviewedDiscoveries"`
	MissingCollections      []string                 `json:"missingCollections,omitempty"`
	UnavailableCollections  []string                 `json:"unavailableCollections,omitempty"`
	CandidateRepositories   []string                 `json:"candidateRepositories,omitempty"`
	CandidatesTruncated     bool                     `json:"candidatesTruncated"`
	Message                 string                   `json:"message"`
}

func repositoryDiscoveryReviewRecord(report OSSInsightRepositoryDiscoveryReport) *models.BrainCatalogRepositoryDiscoveryReview {
	missing, _ := json.Marshal(report.MissingCollections)
	unavailable, _ := json.Marshal(report.UnavailableCollections)
	candidates := repositoryDiscoveryCandidateNames(report.Discoveries)
	candidateNames, _ := json.Marshal(candidates.names)
	checkedAt, err := time.Parse(time.RFC3339, report.CheckedAt)
	if err != nil {
		checkedAt = time.Now().UTC()
	}
	return &models.BrainCatalogRepositoryDiscoveryReview{
		SourceURL: report.SourceURL, Scope: string(report.Scope), Available: report.Available,
		CollectionsScreened: report.CollectionsScreened, EligibleCollections: report.EligibleCollections,
		CollectionsChecked: report.CollectionsChecked, RepositoriesChecked: report.RepositoriesChecked,
		KnownProfileHits: report.KnownProfileHits, UnreviewedDiscoveries: len(report.Discoveries),
		MissingCollectionsJSON: string(missing), UnavailableCollectionsJSON: string(unavailable),
		CandidateRepositoriesJSON: string(candidateNames), CandidatesTruncated: candidates.truncated || report.DiscoveriesTruncated,
		Message: report.Message, CheckedAt: checkedAt.UTC(),
	}
}

func repositoryDiscoveryReviewFromRecord(record models.BrainCatalogRepositoryDiscoveryReview) OSSInsightRepositoryDiscoveryMaintenanceReview {
	var missing, unavailable, candidates []string
	_ = json.Unmarshal([]byte(record.MissingCollectionsJSON), &missing)
	_ = json.Unmarshal([]byte(record.UnavailableCollectionsJSON), &unavailable)
	_ = json.Unmarshal([]byte(record.CandidateRepositoriesJSON), &candidates)
	return OSSInsightRepositoryDiscoveryMaintenanceReview{
		CheckedAt: record.CheckedAt.UTC().Format(time.RFC3339), SourceURL: record.SourceURL,
		Scope: OSSInsightDiscoveryScope(record.Scope), Available: record.Available,
		CollectionsScreened: record.CollectionsScreened, EligibleCollections: record.EligibleCollections,
		CollectionsChecked: record.CollectionsChecked, RepositoriesChecked: record.RepositoriesChecked,
		KnownProfileHits: record.KnownProfileHits, UnreviewedDiscoveries: record.UnreviewedDiscoveries,
		MissingCollections: missing, UnavailableCollections: unavailable, CandidateRepositories: candidates,
		CandidatesTruncated: record.CandidatesTruncated, Message: record.Message,
	}
}

type repositoryDiscoveryCandidateList struct {
	names     []string
	truncated bool
}

func repositoryDiscoveryCandidateNames(discoveries []OSSInsightRepositoryDiscovery) repositoryDiscoveryCandidateList {
	capacity := len(discoveries)
	if capacity > maxPersistedRepositoryDiscoveryCandidates {
		capacity = maxPersistedRepositoryDiscoveryCandidates
	}
	result := repositoryDiscoveryCandidateList{names: make([]string, 0, capacity)}
	for _, discovery := range discoveries {
		name := strings.TrimSpace(discovery.Repository)
		if name == "" {
			continue
		}
		if len(result.names) >= maxPersistedRepositoryDiscoveryCandidates {
			result.truncated = true
			break
		}
		result.names = append(result.names, name)
	}
	return result
}
