package source

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ModeManualImport       = "manual_import"
	ModeScheduledSync      = "scheduled_sync"
	ModeWebhookSync        = "webhook_sync"
	ModeFolderWatcher      = "folder_watcher"
	ModeHistoricalBackfill = "historical_backfill"
	ModeIncrementalSync    = "incremental_sync"
)

type CreateSourceRequest struct {
	ConnectorKey      string   `json:"connectorKey"`
	Name              string   `json:"name"`
	Category          string   `json:"category,omitempty"`
	Enabled           bool     `json:"enabled"`
	LocalOnly         bool     `json:"localOnly"`
	SyncFrequency     string   `json:"syncFrequency,omitempty"`
	SyncTarget        string   `json:"syncTarget,omitempty"`
	DefaultProjectKey string   `json:"defaultProjectKey,omitempty"`
	IngestionModes    []string `json:"ingestionModes,omitempty"`
	Permissions       []string `json:"permissions,omitempty"`
	ExcludePatterns   []string `json:"excludePatterns,omitempty"`
}

type UpdateSourceRequest struct {
	Name              string   `json:"name,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	LocalOnly         *bool    `json:"localOnly,omitempty"`
	SyncFrequency     string   `json:"syncFrequency,omitempty"`
	SyncTarget        *string  `json:"syncTarget,omitempty"`
	DefaultProjectKey *string  `json:"defaultProjectKey,omitempty"`
	Permissions       []string `json:"permissions,omitempty"`
	ExcludePatterns   []string `json:"excludePatterns,omitempty"`
}

type ImportItem struct {
	ExternalID string `json:"externalId"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	SourceURI  string `json:"sourceUri,omitempty"`
	ItemType   string `json:"itemType,omitempty"`
	ProjectKey string `json:"projectKey,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
}

type ImportRequest struct {
	Mode       string       `json:"mode,omitempty"`
	Items      []ImportItem `json:"items"`
	FolderPath string       `json:"folderPath,omitempty"`
	ProjectKey string       `json:"projectKey,omitempty"`
	Limit      int          `json:"limit,omitempty"`
	MaxBytes   int64        `json:"maxBytes,omitempty"`
}

type SyncResult struct {
	Job         models.SourceSyncJob      `json:"job"`
	Extractions []models.SourceExtraction `json:"extractions"`
	Message     string                    `json:"message"`
}

type SearchRequest struct {
	Query            string `json:"query"`
	ProjectKey       string `json:"projectKey,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	IncludeSensitive bool   `json:"includeSensitive,omitempty"`
}

type RankedExtraction struct {
	Extraction  models.SourceExtraction `json:"extraction"`
	Score       float64                 `json:"score"`
	Explanation string                  `json:"explanation"`
}

type SearchResult struct {
	Query       string             `json:"query"`
	ProjectKey  string             `json:"projectKey,omitempty"`
	UsedContext []RankedExtraction `json:"usedContext"`
	Explanation string             `json:"explanation"`
}

type ScheduledSyncRun struct {
	Checked   int      `json:"checked"`
	Due       int      `json:"due"`
	Completed int      `json:"completed"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	Messages  []string `json:"messages"`
}

type Service interface {
	Connectors() ([]models.SourceConnector, error)
	CreateSource(request CreateSourceRequest) (*models.ConnectedSource, error)
	UpdateSource(id uuid.UUID, request UpdateSourceRequest) (*models.ConnectedSource, error)
	Sources(includeDisabled bool) ([]models.ConnectedSource, error)
	Sync(sourceID uuid.UUID, request ImportRequest) (*SyncResult, error)
	RunDueScheduledSyncs(now time.Time) (*ScheduledSyncRun, error)
	Reindex(sourceID uuid.UUID) (*SyncResult, error)
	Pause(sourceID uuid.UUID, paused bool) (*models.ConnectedSource, error)
	Revoke(sourceID uuid.UUID) (*models.ConnectedSource, error)
	Search(request SearchRequest) (*SearchResult, error)
	Extractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error)
	UpdateExtraction(id uuid.UUID, request models.SourceExtraction) (*models.SourceExtraction, error)
	ArchiveExtraction(id uuid.UUID, archived bool) (*models.SourceExtraction, error)
	DeleteExtraction(id uuid.UUID) error
	AuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error)
}

type service struct {
	repo            Repository
	memoryService   memory.Service
	workflowService workflow.Service
}

var errLocalFolderLimitReached = fmt.Errorf("local folder scan limit reached")

func NewService(repo Repository, memoryService memory.Service) Service {
	return &service{repo: repo, memoryService: memoryService}
}

func NewServiceWithWorkflow(repo Repository, memoryService memory.Service, workflowService workflow.Service) Service {
	return &service{repo: repo, memoryService: memoryService, workflowService: workflowService}
}

func DefaultService() Service {
	return NewServiceWithWorkflow(DefaultRepository(), memory.DefaultService(), workflow.DefaultService())
}

func (s *service) Connectors() ([]models.SourceConnector, error) {
	if err := s.ensureConnectors(); err != nil {
		return nil, err
	}
	return s.repo.FindConnectors()
}

func (s *service) CreateSource(request CreateSourceRequest) (*models.ConnectedSource, error) {
	if err := s.ensureConnectors(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	connectorKey := strings.TrimSpace(request.ConnectorKey)
	if connectorKey == "" {
		return nil, fmt.Errorf("connectorKey is required")
	}
	category := firstNonEmpty(request.Category, categoryForConnector(connectorKey))
	if category == "" {
		return nil, fmt.Errorf("category is required")
	}
	source := &models.ConnectedSource{
		ConnectorKey:      connectorKey,
		Name:              name,
		Category:          category,
		Enabled:           request.Enabled,
		LocalOnly:         request.LocalOnly,
		SyncFrequency:     firstNonEmpty(request.SyncFrequency, "manual"),
		SyncTarget:        strings.TrimSpace(request.SyncTarget),
		DefaultProjectKey: strings.TrimSpace(request.DefaultProjectKey),
		IngestionModes:    joinValues(defaultModes(request.IngestionModes)),
		Permissions:       joinValues(minimalPermissions(category, request.Permissions)),
		ExcludePatterns:   joinValues(request.ExcludePatterns),
		Status:            "active",
	}
	if connectorKey == "local-folder" && source.SyncTarget == "" {
		source.SyncTarget = "."
	}
	if !request.Enabled {
		source.Status = "paused"
	}
	created, err := s.repo.CreateSource(source)
	if err == nil {
		s.audit(created.ID, "source.connected", "connected source registered with minimal permissions")
	}
	return created, err
}

func (s *service) UpdateSource(id uuid.UUID, request UpdateSourceRequest) (*models.ConnectedSource, error) {
	source, err := s.repo.FindSource(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Name) != "" {
		source.Name = strings.TrimSpace(request.Name)
	}
	if request.Enabled != nil {
		source.Enabled = *request.Enabled
		if source.Enabled {
			source.Status = "active"
		} else {
			source.Status = "paused"
		}
	}
	if request.LocalOnly != nil {
		source.LocalOnly = *request.LocalOnly
	}
	if request.SyncFrequency != "" {
		source.SyncFrequency = request.SyncFrequency
	}
	if request.SyncTarget != nil {
		source.SyncTarget = strings.TrimSpace(*request.SyncTarget)
	}
	if request.DefaultProjectKey != nil {
		source.DefaultProjectKey = strings.TrimSpace(*request.DefaultProjectKey)
	}
	if request.Permissions != nil {
		source.Permissions = joinValues(minimalPermissions(source.Category, request.Permissions))
	}
	if request.ExcludePatterns != nil {
		source.ExcludePatterns = joinValues(request.ExcludePatterns)
	}
	updated, err := s.repo.UpdateSource(source)
	if err == nil {
		s.audit(id, "source.updated", "source controls updated")
	}
	return updated, err
}

func (s *service) Sources(includeDisabled bool) ([]models.ConnectedSource, error) {
	return s.repo.FindSources(includeDisabled)
}

func (s *service) Sync(sourceID uuid.UUID, request ImportRequest) (*SyncResult, error) {
	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return nil, err
	}
	if !source.Enabled || source.Status == "paused" || source.Status == "revoked" {
		return nil, fmt.Errorf("source is not enabled for sync")
	}
	if source.ConnectorKey == "local-folder" {
		request.FolderPath = firstNonEmpty(request.FolderPath, source.SyncTarget, ".")
		request.ProjectKey = firstNonEmpty(request.ProjectKey, source.DefaultProjectKey)
		source.SyncTarget = request.FolderPath
		source.DefaultProjectKey = request.ProjectKey
	}
	mode := firstNonEmpty(request.Mode, ModeManualImport)
	started := time.Now().UTC()
	job, err := s.repo.CreateSyncJob(&models.SourceSyncJob{
		SourceID:     sourceID,
		Mode:         mode,
		Status:       "running",
		CursorBefore: source.Cursor,
		StartedAt:    started,
	})
	if err != nil {
		return nil, err
	}

	extractions := []models.SourceExtraction{}
	added := 0
	updated := 0
	items := request.Items
	if len(items) == 0 && source.ConnectorKey == "local-folder" {
		items, err = s.localFolderItems(source, request)
		if err != nil {
			now := time.Now().UTC()
			job.Status = "failed"
			job.Message = err.Error()
			job.CompletedAt = &now
			_, _ = s.repo.UpdateSyncJob(job)
			s.audit(sourceID, "source.sync_failed", err.Error())
			return nil, err
		}
	}
	for index, item := range items {
		if shouldExclude(source.ExcludePatterns, item.Title+" "+item.SourceURI) {
			continue
		}
		raw, wasAdded, errItem := s.upsertRawItem(source, item, index)
		if errItem != nil {
			continue
		}
		if wasAdded {
			added++
		} else {
			updated++
		}
		extraction, errExtract := s.extractAndStore(source, raw, item.Content)
		if errExtract == nil {
			extractions = append(extractions, *extraction)
			s.indexExtraction(extraction)
			s.storeUsefulMemory(source, extraction)
			s.createWorkflowFromExtraction(source, extraction)
		}
	}

	now := time.Now().UTC()
	source.LastSyncedAt = &now
	source.Cursor = fmt.Sprintf("%s:%d", now.Format(time.RFC3339), len(items))
	_, _ = s.repo.UpdateSource(source)
	job.Status = "completed"
	job.CursorAfter = source.Cursor
	job.ItemsSeen = len(items)
	job.ItemsAdded = added
	job.ItemsUpdated = updated
	job.Message = "sync completed with cached extraction and provenance links"
	job.CompletedAt = &now
	job, err = s.repo.UpdateSyncJob(job)
	if err == nil {
		s.audit(sourceID, "source.synced", job.Message)
	}
	return &SyncResult{Job: *job, Extractions: extractions, Message: job.Message}, err
}

func (s *service) RunDueScheduledSyncs(now time.Time) (*ScheduledSyncRun, error) {
	sources, err := s.repo.FindSources(false)
	if err != nil {
		return nil, err
	}
	run := &ScheduledSyncRun{Checked: len(sources)}
	for _, source := range sources {
		due, reason := scheduledSourceDue(source, now)
		if !due {
			run.Skipped++
			if reason != "" {
				run.Messages = append(run.Messages, fmt.Sprintf("%s skipped: %s", source.Name, reason))
			}
			continue
		}
		run.Due++
		result, errSync := s.Sync(source.ID, ImportRequest{
			Mode:       ModeScheduledSync,
			FolderPath: source.SyncTarget,
			ProjectKey: source.DefaultProjectKey,
		})
		if errSync != nil {
			run.Failed++
			run.Messages = append(run.Messages, fmt.Sprintf("%s failed: %s", source.Name, errSync.Error()))
			continue
		}
		run.Completed++
		run.Messages = append(run.Messages, fmt.Sprintf("%s synced: %d seen, %d added, %d updated", source.Name, result.Job.ItemsSeen, result.Job.ItemsAdded, result.Job.ItemsUpdated))
	}
	return run, nil
}

func (s *service) Reindex(sourceID uuid.UUID) (*SyncResult, error) {
	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindRawItems(sourceID)
	if err != nil {
		return nil, err
	}
	request := ImportRequest{Mode: ModeIncrementalSync}
	for _, item := range items {
		request.Items = append(request.Items, ImportItem{
			ExternalID: item.ExternalID,
			Title:      item.Title,
			Content:    item.Metadata,
			SourceURI:  item.SourceURI,
			ItemType:   item.ItemType,
			ProjectKey: item.ProjectKey,
			Metadata:   item.Metadata,
		})
	}
	result, err := s.Sync(source.ID, request)
	if err == nil {
		s.audit(sourceID, "source.reindexed", "source re-indexed from cached raw metadata")
	}
	return result, err
}

func (s *service) Pause(sourceID uuid.UUID, paused bool) (*models.ConnectedSource, error) {
	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return nil, err
	}
	source.Enabled = !paused
	if paused {
		source.Status = "paused"
	} else {
		source.Status = "active"
	}
	updated, err := s.repo.UpdateSource(source)
	if err == nil {
		s.audit(sourceID, "source.pause", fmt.Sprintf("source paused=%v", paused))
	}
	return updated, err
}

func (s *service) Revoke(sourceID uuid.UUID) (*models.ConnectedSource, error) {
	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	source.Enabled = false
	source.Status = "revoked"
	source.RevokedAt = &now
	updated, err := s.repo.UpdateSource(source)
	if err == nil {
		s.audit(sourceID, "source.revoked", "source access revoked")
	}
	return updated, err
}

func (s *service) Search(request SearchRequest) (*SearchResult, error) {
	limit := request.Limit
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	extractions, err := s.repo.FindExtractions(strings.TrimSpace(request.ProjectKey), false)
	if err != nil {
		return nil, err
	}
	ranked := []RankedExtraction{}
	for _, extraction := range extractions {
		if extraction.Sensitive && !request.IncludeSensitive {
			continue
		}
		score, explanation := scoreExtraction(extraction, request)
		if score <= 0.12 {
			continue
		}
		ranked = append(ranked, RankedExtraction{
			Extraction:  extraction,
			Score:       math.Round(score*1000) / 1000,
			Explanation: explanation,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return &SearchResult{
		Query:       request.Query,
		ProjectKey:  request.ProjectKey,
		UsedContext: ranked,
		Explanation: fmt.Sprintf("Retrieved %d relevant connected-source records from %d cached extractions; unrelated and sensitive records were not loaded.", len(ranked), len(extractions)),
	}, nil
}

func (s *service) Extractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	return s.repo.FindExtractions(projectKey, includeArchived)
}

func (s *service) UpdateExtraction(id uuid.UUID, request models.SourceExtraction) (*models.SourceExtraction, error) {
	extraction, err := s.repo.FindExtraction(id)
	if err != nil {
		return nil, err
	}
	if request.Text != "" {
		extraction.Text = request.Text
	}
	if request.Summary != "" {
		extraction.Summary = request.Summary
	}
	if request.ProjectKey != "" {
		extraction.ProjectKey = request.ProjectKey
	}
	extraction.Entities = request.Entities
	extraction.Dates = request.Dates
	extraction.Tasks = request.Tasks
	extraction.Decisions = request.Decisions
	extraction.FollowUps = request.FollowUps
	extraction.Sensitive = request.Sensitive
	extraction.Uncertain = request.Uncertain
	updated, err := s.repo.SaveExtraction(extraction)
	if err == nil {
		s.audit(extraction.SourceID, "extraction.corrected", "operator corrected extraction")
	}
	return updated, err
}

func (s *service) ArchiveExtraction(id uuid.UUID, archived bool) (*models.SourceExtraction, error) {
	extraction, err := s.repo.FindExtraction(id)
	if err != nil {
		return nil, err
	}
	extraction.Archived = archived
	updated, err := s.repo.SaveExtraction(extraction)
	if err == nil {
		s.audit(extraction.SourceID, "extraction.archived", fmt.Sprintf("archived=%v", archived))
	}
	return updated, err
}

func (s *service) DeleteExtraction(id uuid.UUID) error {
	extraction, err := s.repo.FindExtraction(id)
	if err == nil {
		s.audit(extraction.SourceID, "extraction.deleted", "operator deleted extraction")
	}
	return s.repo.DeleteExtraction(id)
}

func (s *service) AuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error) {
	return s.repo.FindAuditLogs(sourceID)
}

func (s *service) localFolderItems(source *models.ConnectedSource, request ImportRequest) ([]ImportItem, error) {
	root := firstNonEmpty(os.Getenv("CONNECTED_SOURCE_LOCAL_ROOT"), "/root/connected-sources")
	folder, err := resolveAllowedFolder(root, request.FolderPath)
	if err != nil {
		return nil, err
	}
	limit := firstPositiveInt(request.Limit, envInt("CONNECTED_SOURCE_FILE_LIMIT", 100))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	maxBytes := firstPositiveInt64(request.MaxBytes, envInt64("CONNECTED_SOURCE_MAX_BYTES", 1024*1024))
	if maxBytes <= 0 || maxBytes > 10*1024*1024 {
		maxBytes = 1024 * 1024
	}
	includeAfter := time.Time{}
	if request.Mode != ModeHistoricalBackfill && source.LastSyncedAt != nil {
		includeAfter = *source.LastSyncedAt
	}
	items := []ImportItem{}
	err = filepath.WalkDir(folder, func(path string, entry os.DirEntry, errWalk error) error {
		if errWalk != nil {
			return nil
		}
		if len(items) >= limit {
			return errLocalFolderLimitReached
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != folder && shouldExclude(source.ExcludePatterns, name) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldExclude(source.ExcludePatterns, path) || !isReadableLocalFile(path) {
			return nil
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return nil
		}
		if !includeAfter.IsZero() && !info.ModTime().After(includeAfter) {
			return nil
		}
		content, errRead := readLocalTextFile(path, maxBytes)
		if errRead != nil || strings.TrimSpace(content) == "" {
			return nil
		}
		rel, _ := filepath.Rel(folder, path)
		items = append(items, ImportItem{
			ExternalID: "file:" + filepath.ToSlash(rel),
			Title:      filepath.ToSlash(rel),
			Content:    content,
			SourceURI:  "file://" + filepath.ToSlash(path),
			ItemType:   localFileContentType(path),
			ProjectKey: request.ProjectKey,
			Metadata:   fmt.Sprintf("size=%d;modified=%s", info.Size(), info.ModTime().UTC().Format(time.RFC3339)),
		})
		return nil
	})
	if err != nil && err != errLocalFolderLimitReached {
		return nil, err
	}
	s.audit(source.ID, "source.local_folder_scanned", fmt.Sprintf("scanned %d files from %s", len(items), folder))
	return items, nil
}

func (s *service) ensureConnectors() error {
	for _, connector := range defaultConnectors() {
		c := connector
		if _, err := s.repo.SaveConnector(&c); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) upsertRawItem(source *models.ConnectedSource, item ImportItem, index int) (*models.SourceRawItem, bool, error) {
	externalID := firstNonEmpty(item.ExternalID, fmt.Sprintf("manual-%d-%s", index, hashText(item.Title+item.Content)))
	existing, err := s.repo.FindRawItem(source.ID, externalID)
	added := false
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	if err == gorm.ErrRecordNotFound {
		existing = &models.SourceRawItem{
			SourceID:   source.ID,
			ExternalID: externalID,
			FetchedAt:  time.Now().UTC(),
		}
		added = true
	}
	existing.ProjectKey = item.ProjectKey
	existing.ItemType = firstNonEmpty(item.ItemType, source.Category)
	existing.Title = item.Title
	existing.SourceURI = item.SourceURI
	existing.Metadata = item.Content
	existing.ContentHash = hashText(item.Title + "|" + item.Content)
	raw, err := s.repo.SaveRawItem(existing)
	return raw, added, err
}

func (s *service) extractAndStore(source *models.ConnectedSource, raw *models.SourceRawItem, text string) (*models.SourceExtraction, error) {
	existing, err := s.repo.FindExtractionByRawItem(raw.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		existing = &models.SourceExtraction{SourceID: source.ID, RawItemID: raw.ID}
	}
	now := time.Now().UTC()
	clean := normalizeSpaces(text)
	existing.ProjectKey = raw.ProjectKey
	existing.ContentType = raw.ItemType
	existing.Text = clean
	existing.Summary = compact(clean, 420)
	existing.Entities = joinValues(extractEntities(clean))
	existing.Dates = joinValues(extractDates(clean))
	existing.Tasks = joinValues(extractTasks(clean))
	existing.Decisions = joinValues(extractDecisions(clean))
	existing.FollowUps = joinValues(extractFollowUps(clean))
	existing.SourceURI = raw.SourceURI
	existing.SourceLabel = raw.Title
	existing.ContentHash = raw.ContentHash
	existing.Sensitive = containsAny(strings.ToLower(clean), "password", "secret", "token", "bank", "invoice", "contract", "legal", "medical")
	existing.Uncertain = len(clean) < 40 || containsAny(strings.ToLower(clean), "maybe", "unclear", "unknown")
	existing.LastIndexedAt = &now
	return s.repo.SaveExtraction(existing)
}

func (s *service) storeUsefulMemory(source *models.ConnectedSource, extraction *models.SourceExtraction) {
	if extraction.Sensitive || extraction.Uncertain || extraction.Summary == "" {
		return
	}
	_, _ = s.memoryService.Create(memory.CreateRequest{
		ProjectKey:  extraction.ProjectKey,
		Kind:        "source",
		Content:     extraction.Summary,
		Summary:     extraction.Summary,
		Tags:        []string{"connected-source", source.Category, source.ConnectorKey},
		Confidence:  0.68,
		SourceURI:   extraction.SourceURI,
		SourceLabel: extraction.SourceLabel,
	})
}

func (s *service) createWorkflowFromExtraction(source *models.ConnectedSource, extraction *models.SourceExtraction) {
	if s.workflowService == nil {
		return
	}
	taskSignal := firstNonEmpty(extraction.Tasks, extraction.FollowUps)
	if taskSignal == "" {
		return
	}
	input := strings.Join([]string{
		firstNonEmpty(extraction.Summary, extraction.SourceLabel),
		"Tasks: " + extraction.Tasks,
		"Follow-ups: " + extraction.FollowUps,
		"Dates: " + extraction.Dates,
	}, "\n")
	_, err := s.workflowService.Intake(workflow.IntakeRequest{
		Input:       input,
		ProjectKey:  extraction.ProjectKey,
		SourceType:  source.Category,
		SourceURI:   firstNonEmpty(extraction.SourceURI, "source-extraction://"+extraction.ID.String()),
		SourceLabel: extraction.SourceLabel,
		Trigger:     "source.extraction",
		Actor:       "source-worker",
	})
	if err != nil {
		s.audit(source.ID, "workflow.intake_failed", err.Error())
		return
	}
	s.audit(source.ID, "workflow.intake_created", "actionable extraction sent to workflow engine")
}

func (s *service) indexExtraction(extraction *models.SourceExtraction) {
	keywords := strings.Join(mapKeys(tokenSet(extraction.Text+" "+extraction.Summary+" "+extraction.Entities+" "+extraction.Tasks)), ",")
	_, _ = s.repo.SaveIndexEntry(&models.SourceIndexEntry{
		SourceID:     extraction.SourceID,
		ExtractionID: extraction.ID,
		ProjectKey:   extraction.ProjectKey,
		IndexType:    "keyword",
		Keywords:     keywords,
	})
	_, _ = s.repo.SaveIndexEntry(&models.SourceIndexEntry{
		SourceID:     extraction.SourceID,
		ExtractionID: extraction.ID,
		ProjectKey:   extraction.ProjectKey,
		IndexType:    "vector_ref",
		VectorRef:    "local-vector-pending:" + extraction.ID.String(),
	})
}

func (s *service) audit(sourceID uuid.UUID, action, message string) {
	_, _ = s.repo.SaveAuditLog(&models.SourceAuditLog{
		SourceID: sourceID,
		Action:   action,
		Message:  message,
	})
}

func defaultConnectors() []models.SourceConnector {
	modes := joinValues([]string{ModeManualImport, ModeScheduledSync, ModeWebhookSync, ModeHistoricalBackfill, ModeIncrementalSync})
	return []models.SourceConnector{
		{ConnectorKey: "email", Name: "Email accounts", Category: "email", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true},
		{ConnectorKey: "calendar", Name: "Calendars", Category: "calendar", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true},
		{ConnectorKey: "cloud-documents", Name: "Cloud drives and documents", Category: "cloud_document", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true},
		{ConnectorKey: "project-board", Name: "Trello and project boards", Category: "project_board", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true},
		{ConnectorKey: "github", Name: "GitHub repositories and issues", Category: "github", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true},
		{ConnectorKey: "local-folder", Name: "Selected local folders", Category: "local_folder", SupportedModes: joinValues([]string{ModeManualImport, ModeScheduledSync, ModeFolderWatcher, ModeIncrementalSync}), RequiredScopes: "selected-folder-read", LocalOnlyCapable: true, Enabled: true},
	}
}

func categoryForConnector(connectorKey string) string {
	for _, connector := range defaultConnectors() {
		if connector.ConnectorKey == connectorKey {
			return connector.Category
		}
	}
	return ""
}

func defaultModes(values []string) []string {
	if len(values) > 0 {
		return values
	}
	return []string{ModeManualImport, ModeIncrementalSync}
}

func minimalPermissions(category string, requested []string) []string {
	if len(requested) == 0 {
		return []string{"metadata:read", category + ":read"}
	}
	allowed := map[string]bool{"metadata:read": true, category + ":read": true, "selected-folder-read": true}
	result := []string{}
	for _, value := range requested {
		value = strings.TrimSpace(value)
		if allowed[value] {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return []string{"metadata:read"}
	}
	return result
}

func scoreExtraction(extraction models.SourceExtraction, request SearchRequest) (float64, string) {
	queryTokens := tokenSet(request.Query)
	textTokens := tokenSet(extraction.Text + " " + extraction.Summary + " " + extraction.Entities + " " + extraction.Tasks + " " + extraction.Decisions)
	relevance := overlapScore(queryTokens, textTokens)
	projectMatch := 0.0
	if request.ProjectKey != "" && request.ProjectKey == extraction.ProjectKey {
		projectMatch = 0.2
	}
	recency := recencyScore(extraction.UpdatedAt)
	provenance := 0.1
	if extraction.SourceURI == "" {
		provenance = 0
	}
	score := relevance*0.55 + projectMatch + recency*0.15 + provenance
	parts := []string{fmt.Sprintf("relevance %.2f", relevance), fmt.Sprintf("recency %.2f", recency)}
	if projectMatch > 0 {
		parts = append(parts, "same project")
	}
	if provenance > 0 {
		parts = append(parts, "source linked")
	}
	return score, strings.Join(parts, ", ")
}

func scheduledSourceDue(source models.ConnectedSource, now time.Time) (bool, string) {
	if source.ConnectorKey != "local-folder" {
		return false, "scheduled adapter is not implemented for connector " + source.ConnectorKey
	}
	interval, ok := parseSyncFrequency(source.SyncFrequency)
	if !ok {
		return false, "sync frequency is manual or unsupported"
	}
	if source.LastSyncedAt == nil {
		return true, ""
	}
	if now.Sub(*source.LastSyncedAt) >= interval {
		return true, ""
	}
	return false, "not due yet"
}

func parseSyncFrequency(value string) (time.Duration, bool) {
	clean := strings.TrimSpace(strings.ToLower(value))
	switch clean {
	case "", "manual", "off", "disabled", "none":
		return 0, false
	case "hourly":
		return time.Hour, true
	case "daily":
		return 24 * time.Hour, true
	case "weekly":
		return 7 * 24 * time.Hour, true
	}
	duration, err := time.ParseDuration(clean)
	if err != nil || duration < time.Minute {
		return 0, false
	}
	return duration, true
}

func extractEntities(text string) []string {
	result := []string{}
	for _, word := range strings.Fields(text) {
		word = strings.Trim(word, ".,;:()[]")
		if len(word) > 2 && word[:1] == strings.ToUpper(word[:1]) {
			result = append(result, word)
		}
	}
	return uniqueStrings(limitValues(result, 20))
}

func extractDates(text string) []string {
	result := []string{}
	for _, word := range strings.Fields(text) {
		clean := strings.Trim(word, ".,;:()[]")
		if strings.Contains(clean, "202") || strings.Contains(clean, "deadline") || strings.Contains(clean, "tomorrow") || strings.Contains(clean, "today") {
			result = append(result, clean)
		}
	}
	return uniqueStrings(limitValues(result, 20))
}

func extractTasks(text string) []string {
	return extractSentences(text, "todo", "must", "should", "need to", "action", "task")
}

func extractDecisions(text string) []string {
	return extractSentences(text, "decided", "decision", "approved", "rejected", "agreed")
}

func extractFollowUps(text string) []string {
	return extractSentences(text, "follow up", "waiting", "open loop", "remind", "next")
}

func extractSentences(text string, needles ...string) []string {
	result := []string{}
	for _, sentence := range strings.Split(strings.ReplaceAll(text, "\n", ". "), ".") {
		lower := strings.ToLower(sentence)
		if containsAny(lower, needles...) {
			result = append(result, compact(sentence, 220))
		}
	}
	return uniqueStrings(limitValues(result, 12))
}

func shouldExclude(patterns, value string) bool {
	value = strings.ToLower(value)
	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(strings.ToLower(pattern))
		if pattern != "" && strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func hashText(value string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.ToLower(normalizeSpaces(value))))
	return fmt.Sprintf("%x", hash.Sum64())
}

func normalizeSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func compact(value string, limit int) string {
	value = normalizeSpaces(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit-3] + "..."
}

func joinValues(values []string) string {
	return strings.Join(uniqueStrings(values), ",")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func limitValues(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func tokenSet(value string) map[string]bool {
	set := map[string]bool{}
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ", "/", " ", "\\", " ", "\n", " ", "\t", " ", "(", " ", ")", " ")
	for _, token := range strings.Fields(strings.ToLower(replacer.Replace(value))) {
		if len(token) >= 3 {
			set[token] = true
		}
	}
	return set
}

func mapKeys(values map[string]bool) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 80 {
		return keys[:80]
	}
	return keys
}

func overlapScore(queryTokens, textTokens map[string]bool) float64 {
	if len(queryTokens) == 0 {
		return 0.2
	}
	matches := 0
	for token := range queryTokens {
		if textTokens[token] {
			matches++
		}
	}
	return float64(matches) / float64(len(queryTokens))
}

func recencyScore(value time.Time) float64 {
	days := time.Since(value).Hours() / 24
	if days <= 1 {
		return 1
	}
	if days >= 90 {
		return 0.05
	}
	return 1 - (days / 100)
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed int64
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func resolveAllowedFolder(root, requested string) (string, error) {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "."
	}
	if strings.ContainsAny(requested, "\r\n\x00") {
		return "", fmt.Errorf("folder path contains invalid characters")
	}
	var folderAbs string
	if filepath.IsAbs(requested) {
		folderAbs, err = filepath.Abs(filepath.Clean(requested))
	} else {
		folderAbs, err = filepath.Abs(filepath.Join(rootAbs, filepath.Clean(requested)))
	}
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, folderAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("folder path must stay inside allowlisted root %s", rootAbs)
	}
	info, err := os.Stat(folderAbs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("folder path is not a directory")
	}
	return folderAbs, nil
}

func readLocalTextFile(path string, maxBytes int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > maxBytes {
		content = content[:maxBytes]
	}
	if strings.Contains(string(content), "\x00") {
		return "", fmt.Errorf("binary file skipped")
	}
	return string(content), nil
}

func isReadableLocalFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".yaml", ".yml", ".log":
		return true
	default:
		return false
	}
}

func localFileContentType(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		return "local_file"
	}
	return "local_file_" + ext
}
