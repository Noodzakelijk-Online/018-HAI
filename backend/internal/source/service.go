package source

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/safety"
	"automation-hub-backend/internal/workflow"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	Errors      []string                  `json:"errors,omitempty"`
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
	SyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error)
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
	pursuitLinker   pursuitAutoLinker
	syncMu          sync.Mutex
	activeSyncs     map[uuid.UUID]bool
}

type pursuitAutoLinker interface {
	AutoLinkWorkflow(request pursuit.AutoLinkWorkflowRequest) (*pursuit.AutoLinkResult, error)
	AutoLinkMemory(request pursuit.AutoLinkMemoryRequest) (*pursuit.AutoLinkResult, error)
}

var errLocalFolderLimitReached = fmt.Errorf("local folder scan limit reached")
var ErrSyncInProgress = errors.New("source sync is already in progress")

const maxSyncErrorDetails = 20
const defaultHTTPFeedAllowedHosts = "localhost,127.0.0.1,::1,host.docker.internal,api.github.com"
const defaultWhatsAppChunkMessages = 40

var whatsAppMessageLine = regexp.MustCompile(`^\[?(\d{1,2}[/-]\d{1,2}[/-]\d{2,4}),?\s+(\d{1,2}:\d{2}(?::\d{2})?(?:\s?[APap]\.?[Mm]\.?)?)\]?\s*[-\x{2013}]\s*([^:]{1,120}):\s*(.*)$`)

func NewService(repo Repository, memoryService memory.Service) Service {
	return &service{repo: repo, memoryService: memoryService, activeSyncs: map[uuid.UUID]bool{}}
}

func NewServiceWithWorkflow(repo Repository, memoryService memory.Service, workflowService workflow.Service) Service {
	return NewServiceWithWorkflowAndPursuitLinker(repo, memoryService, workflowService, nil)
}

func NewServiceWithWorkflowAndPursuitLinker(repo Repository, memoryService memory.Service, workflowService workflow.Service, pursuitLinker pursuitAutoLinker) Service {
	return &service{repo: repo, memoryService: memoryService, workflowService: workflowService, pursuitLinker: pursuitLinker, activeSyncs: map[uuid.UUID]bool{}}
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
	connector, err := s.connectorByKey(connectorKey)
	if err != nil {
		return nil, err
	}
	if !connector.Enabled || !adapterIsUsable(connector.AdapterStatus) {
		return nil, fmt.Errorf("connector %s is registered but its real adapter is not implemented yet", connectorKey)
	}
	category := firstNonEmpty(request.Category, connector.Category)
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
	if sourceUsesLocalFolder(connectorKey) && source.SyncTarget == "" {
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

func (s *service) SyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error) {
	return s.repo.FindSyncJobs(sourceID)
}

func (s *service) Sync(sourceID uuid.UUID, request ImportRequest) (*SyncResult, error) {
	if !s.beginSync(sourceID) {
		return nil, ErrSyncInProgress
	}
	defer s.endSync(sourceID)

	source, err := s.repo.FindSource(sourceID)
	if err != nil {
		return nil, err
	}
	if !source.Enabled || source.Status == "paused" || source.Status == "revoked" {
		return nil, fmt.Errorf("source is not enabled for sync")
	}
	if !sourceHasNativeAdapter(source.ConnectorKey) && len(request.Items) == 0 {
		return nil, fmt.Errorf("connector %s has no real sync adapter yet; provide explicit manual import items or use local-folder", source.ConnectorKey)
	}
	if sourceUsesLocalFolder(source.ConnectorKey) {
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
	failed := 0
	itemErrors := []string{}
	recordFailure := func(item ImportItem, stage string, err error) {
		failed++
		if len(itemErrors) < maxSyncErrorDetails {
			itemErrors = append(itemErrors, itemFailure(item, stage, err))
		}
	}
	items := request.Items
	adapterCursor := ""
	if len(items) == 0 && sourceUsesLocalFolder(source.ConnectorKey) {
		items, err = s.localFolderItems(source, request)
		items = filterConnectorLocalItems(items, source.ConnectorKey)
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
	if len(items) == 0 && source.ConnectorKey == "json-feed" {
		items, adapterCursor, err = fetchJSONFeed(source)
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
	if len(items) == 0 && source.ConnectorKey == "github" {
		items, adapterCursor, err = fetchGitHubSource(source)
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
	if source.ConnectorKey == "whatsapp-export" {
		items, err = s.whatsAppExportItems(source, request)
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
	if len(items) == 0 && source.ConnectorKey == "odoo-herp" {
		items = odooHERPItems(source, request)
		s.audit(sourceID, "source.odoo_herp_modeled", fmt.Sprintf("modeled %d Odoo/HERP app domain(s) into governed source records", len(items)))
	}
	for index, item := range items {
		if shouldExclude(source.ExcludePatterns, item.Title+" "+item.SourceURI) {
			continue
		}
		raw, wasAdded, errItem := s.upsertRawItem(source, item, index)
		if errItem != nil {
			recordFailure(item, "raw item persistence failed", errItem)
			continue
		}
		if wasAdded {
			added++
		} else {
			updated++
		}
		extraction, errExtract := s.extractAndStore(source, raw, item.Content)
		if errExtract != nil {
			recordFailure(item, "extraction failed", errExtract)
			continue
		}
		if errIndex := s.indexExtraction(extraction); errIndex != nil {
			recordFailure(item, "index update failed", errIndex)
			continue
		}
		if errWorkflow := s.createWorkflowFromExtraction(source, extraction); errWorkflow != nil {
			recordFailure(item, "workflow intake failed", errWorkflow)
			continue
		}
		extractions = append(extractions, *extraction)
		s.storeUsefulMemory(source, extraction)
	}

	now := time.Now().UTC()
	job.ItemsSeen = len(items)
	job.ItemsAdded = added
	job.ItemsUpdated = updated
	job.ItemsFailed = failed
	job.CursorAfter = source.Cursor
	if failed == 0 {
		source.LastSyncedAt = &now
		source.Cursor = firstNonEmpty(adapterCursor, fmt.Sprintf("%s:%d", now.Format(time.RFC3339), len(items)))
		job.Status = "completed"
		job.CursorAfter = source.Cursor
		job.Message = "sync completed with cached extraction and provenance links"
	} else if len(extractions) > 0 {
		job.Status = "partial_failure"
		job.Message = fmt.Sprintf("sync partially completed; %d item(s) failed and the cursor was retained for retry", failed)
	} else {
		job.Status = "failed"
		job.Message = fmt.Sprintf("sync failed; %d item(s) failed and the cursor was retained for retry", failed)
	}
	if _, errSource := s.repo.UpdateSource(source); errSource != nil {
		failed++
		job.ItemsFailed = failed
		job.Status = "failed"
		job.CursorAfter = job.CursorBefore
		job.Message = "sync result could not update source state; cursor was not confirmed"
		if len(itemErrors) < maxSyncErrorDetails {
			itemErrors = append(itemErrors, "source state update failed: "+compact(errSource.Error(), 220))
		}
	}
	job.CompletedAt = &now
	job, err = s.repo.UpdateSyncJob(job)
	if err == nil {
		action := "source.synced"
		if job.Status != "completed" {
			action = "source.sync_partial_failure"
		}
		s.audit(sourceID, action, job.Message)
	}
	return &SyncResult{Job: *job, Extractions: extractions, Message: job.Message, Errors: itemErrors}, err
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
			if errors.Is(errSync, ErrSyncInProgress) {
				run.Skipped++
				run.Messages = append(run.Messages, fmt.Sprintf("%s skipped: sync already in progress", source.Name))
				continue
			}
			run.Failed++
			run.Messages = append(run.Messages, fmt.Sprintf("%s failed: %s", source.Name, errSync.Error()))
			s.createSyncFailureWorkflow(source, errSync.Error())
			continue
		}
		if result.Job.Status != "completed" {
			run.Failed++
			run.Messages = append(run.Messages, fmt.Sprintf("%s %s: %d seen, %d failed", source.Name, result.Job.Status, result.Job.ItemsSeen, result.Job.ItemsFailed))
			s.createSyncFailureWorkflow(source, result.Job.Message)
			continue
		}
		run.Completed++
		run.Messages = append(run.Messages, fmt.Sprintf("%s synced: %d seen, %d added, %d updated", source.Name, result.Job.ItemsSeen, result.Job.ItemsAdded, result.Job.ItemsUpdated))
	}
	return run, nil
}

func (s *service) createSyncFailureWorkflow(source models.ConnectedSource, reason string) {
	if s.workflowService == nil {
		return
	}
	record, err := s.workflowService.Intake(workflow.IntakeRequest{
		Input: strings.Join([]string{
			"Connected source sync failed for " + source.Name + ".",
			"Connector: " + source.ConnectorKey + ".",
			"Failure: " + compact(safety.RedactSecrets(reason), 320),
			"Required action: inspect connector health, permissions, target availability, and retry policy before resuming autonomous ingestion.",
		}, "\n"),
		ProjectKey:     source.DefaultProjectKey,
		SourceType:     "source_sync",
		SourceID:       source.ID.String(),
		SourceURI:      safety.RedactURL(source.SyncTarget),
		SourceLabel:    source.Name,
		ContentType:    "operational_failure",
		Trigger:        "scheduled_source_sync_failed",
		Actor:          "source-scheduler",
		RequiresReview: true,
		ReviewReason:   "background source ingestion failed and requires operator review",
	})
	if err != nil {
		s.audit(source.ID, "source.failure_workflow_failed", compact(err.Error(), 260))
		return
	}
	if record != nil {
		s.autoLinkPursuitWorkflow(&source, nil, record, "Connected source sync failed for "+source.Name+". "+reason)
	}
	s.audit(source.ID, "source.failure_workflow_created", "scheduled sync failure routed to workflow review")
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
			Content:    firstNonEmpty(item.Content, item.Metadata),
			SourceURI:  item.SourceURI,
			ItemType:   item.ItemType,
			ProjectKey: item.ProjectKey,
			Metadata:   item.Metadata,
		})
	}
	result, err := s.Sync(source.ID, request)
	if err == nil {
		s.audit(sourceID, "source.reindexed", "source re-indexed from cached raw content")
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
	before := *extraction
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
	if firstNonEmpty(extraction.Tasks, extraction.FollowUps) == "" {
		if err := s.retractWorkflowForExtraction(extraction, "corrected extraction no longer contains an actionable task or follow-up"); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.SaveExtraction(extraction)
	if err == nil && updated != nil {
		s.audit(extraction.SourceID, "extraction.corrected", "operator corrected extraction")
		if errIndex := s.indexExtraction(updated); errIndex != nil {
			return updated, fmt.Errorf("extraction corrected but index update failed: %w", errIndex)
		}
		if errWorkflow := s.reconcileWorkflowFromExtraction(updated); errWorkflow != nil {
			return updated, fmt.Errorf("extraction corrected but workflow reconciliation failed: %w", errWorkflow)
		}
		s.rememberExtractionCorrection(&before, updated)
	}
	return updated, err
}

func (s *service) ArchiveExtraction(id uuid.UUID, archived bool) (*models.SourceExtraction, error) {
	extraction, err := s.repo.FindExtraction(id)
	if err != nil {
		return nil, err
	}
	if archived {
		if err := s.retractWorkflowForExtraction(extraction, "source extraction was archived by the operator"); err != nil {
			return nil, err
		}
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
	if err != nil {
		return err
	}
	if err := s.retractWorkflowForExtraction(extraction, "source extraction was deleted by the operator"); err != nil {
		return err
	}
	s.audit(extraction.SourceID, "extraction.deleted", "operator deleted extraction")
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
		if entry.Type()&os.ModeSymlink != 0 {
			s.audit(source.ID, "source.local_folder_symlink_skipped", fmt.Sprintf("skipped symlink %s", path))
			return nil
		}
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
	s.audit(source.ID, "source.local_folder_scanned", fmt.Sprintf("scanned %d files from selected local-folder source", len(items)))
	return items, nil
}

func (s *service) whatsAppExportItems(source *models.ConnectedSource, request ImportRequest) ([]ImportItem, error) {
	projectKey := firstNonEmpty(request.ProjectKey, source.DefaultProjectKey)
	if len(request.Items) > 0 {
		return expandWhatsAppImportItems(request.Items, projectKey, source.Name, firstPositiveInt(request.Limit, envInt("WHATSAPP_EXPORT_CHUNK_MESSAGES", defaultWhatsAppChunkMessages))), nil
	}

	root := firstNonEmpty(os.Getenv("CONNECTED_SOURCE_LOCAL_ROOT"), "/root/connected-sources")
	folder, err := resolveAllowedFolder(root, firstNonEmpty(request.FolderPath, source.SyncTarget, "."))
	if err != nil {
		return nil, err
	}
	limit := firstPositiveInt(request.Limit, envInt("CONNECTED_SOURCE_FILE_LIMIT", 100))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	maxBytes := firstPositiveInt64(request.MaxBytes, envInt64("CONNECTED_SOURCE_MAX_BYTES", 2*1024*1024))
	if maxBytes <= 0 || maxBytes > 10*1024*1024 {
		maxBytes = 2 * 1024 * 1024
	}

	baseItems := []ImportItem{}
	err = filepath.WalkDir(folder, func(path string, entry os.DirEntry, errWalk error) error {
		if errWalk != nil {
			return nil
		}
		if len(baseItems) >= limit {
			return errLocalFolderLimitReached
		}
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			s.audit(source.ID, "source.whatsapp_symlink_skipped", fmt.Sprintf("skipped symlink %s", path))
			return nil
		}
		if entry.IsDir() {
			if path != folder && shouldExclude(source.ExcludePatterns, name) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldExclude(source.ExcludePatterns, path) || strings.ToLower(filepath.Ext(path)) != ".txt" {
			return nil
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return nil
		}
		if request.Mode != ModeHistoricalBackfill && source.LastSyncedAt != nil && !info.ModTime().After(*source.LastSyncedAt) {
			return nil
		}
		content, errRead := readLocalTextFile(path, maxBytes)
		if errRead != nil || strings.TrimSpace(content) == "" {
			return nil
		}
		rel, _ := filepath.Rel(folder, path)
		baseItems = append(baseItems, ImportItem{
			ExternalID: "whatsapp-file:" + filepath.ToSlash(rel),
			Title:      strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel)),
			Content:    content,
			SourceURI:  "file://" + filepath.ToSlash(path),
			ItemType:   "whatsapp_export",
			ProjectKey: projectKey,
			Metadata:   fmt.Sprintf("source=whatsapp-export;size=%d;modified=%s", info.Size(), info.ModTime().UTC().Format(time.RFC3339)),
		})
		return nil
	})
	if err != nil && err != errLocalFolderLimitReached {
		return nil, err
	}
	items := expandWhatsAppImportItems(baseItems, projectKey, source.Name, envInt("WHATSAPP_EXPORT_CHUNK_MESSAGES", defaultWhatsAppChunkMessages))
	s.audit(source.ID, "source.whatsapp_export_scanned", fmt.Sprintf("parsed %d source file(s) into %d bounded WhatsApp conversation windows", len(baseItems), len(items)))
	return items, nil
}

type odooHERPApp struct {
	Name      string
	Domain    string
	Role      string
	Signals   []string
	Workflows []string
	Risk      string
	Autonomy  string
}

func odooHERPItems(source *models.ConnectedSource, request ImportRequest) []ImportItem {
	projectKey := firstNonEmpty(request.ProjectKey, source.DefaultProjectKey, "Robert-life-os")
	apps := selectedOdooHERPApps(firstNonEmpty(request.FolderPath, source.SyncTarget))
	items := make([]ImportItem, 0, len(apps))
	for _, app := range apps {
		slug := slugText(app.Name)
		items = append(items, ImportItem{
			ExternalID: "odoo-herp-app:" + slug,
			Title:      "Odoo " + app.Name + " app",
			Content:    renderOdooHERPApp(app),
			SourceURI:  sourceURIForOdooApp(source.SyncTarget, slug),
			ItemType:   "odoo_herp_app",
			ProjectKey: projectKey,
			Metadata: fmt.Sprintf(
				"source=odoo-herp;domain=%s;risk=%s;autonomy=%s;read_only_default=true",
				app.Domain,
				app.Risk,
				app.Autonomy,
			),
		})
	}
	return items
}

func selectedOdooHERPApps(selector string) []odooHERPApp {
	apps := defaultOdooHERPApps()
	raw := strings.TrimSpace(selector)
	if raw == "" {
		return apps
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.RawQuery != "" {
		raw = firstNonEmpty(parsed.Query().Get("apps"), parsed.Query().Get("modules"), parsed.Query().Get("domains"))
	}
	if raw == "" || strings.Contains(strings.ToLower(raw), "odoo.com/odoo") {
		return apps
	}
	requested := map[string]bool{}
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	}) {
		key := slugText(value)
		if key != "" {
			requested[key] = true
		}
	}
	if len(requested) == 0 {
		return apps
	}
	selected := []odooHERPApp{}
	for _, app := range apps {
		if odooAppRequested(app, requested) {
			selected = append(selected, app)
		}
	}
	if len(selected) == 0 {
		return apps
	}
	return selected
}

func odooAppRequested(app odooHERPApp, requested map[string]bool) bool {
	appKey := slugText(app.Name)
	domainKey := slugText(app.Domain)
	for key := range requested {
		if key == appKey || key == domainKey || strings.Contains(appKey, key) || (len(key) >= 6 && strings.Contains(domainKey, key)) {
			return true
		}
	}
	return false
}

func renderOdooHERPApp(app odooHERPApp) string {
	return strings.Join([]string{
		"Odoo app: " + app.Name,
		"HERP domain: " + app.Domain,
		"HAI role: " + app.Role,
		"Source signals: " + strings.Join(app.Signals, "; "),
		"Workflow candidates: " + strings.Join(app.Workflows, "; "),
		"Task: map " + app.Name + " records into source-linked HAI workflows, next-best actions, and review queues.",
		"Follow up: review fresh " + app.Domain + " records during Odoo sync and create follow-up tasks when an owner, deadline, quote, invoice, stock movement, customer reply, or blocked item appears.",
		"Decision: keep Odoo write-back read-only by default; quotes, invoices, payments, public messages, inventory changes, account changes, and external sends require Robert approval.",
		"Risk: " + app.Risk,
		"Autonomy: " + app.Autonomy,
	}, "\n")
}

func sourceURIForOdooApp(syncTarget, slug string) string {
	target := strings.TrimSpace(syncTarget)
	if target == "" || strings.ContainsAny(target, ",;|") {
		return "odoo://app/" + slug
	}
	if strings.Contains(target, "#") {
		return target + "&app=" + slug
	}
	return strings.TrimRight(target, "/") + "#app=" + slug
}

func defaultOdooHERPApps() []odooHERPApp {
	return []odooHERPApp{
		{Name: "CRM", Domain: "relationships", Role: "turn leads, opportunities, promises, and stalled conversations into next actions.", Signals: []string{"lead stage", "expected revenue", "next activity", "lost reason"}, Workflows: []string{"follow up stale opportunities", "draft client replies", "surface high-value leads"}, Risk: "medium", Autonomy: "draft and schedule low-risk reminders"},
		{Name: "Sales", Domain: "commercial operations", Role: "track quotes, orders, customer commitments, and deal blockers.", Signals: []string{"quotation status", "expiration date", "customer acceptance", "delivery promise"}, Workflows: []string{"draft quotation follow-ups", "flag expiring offers", "prepare handoff checklists"}, Risk: "medium", Autonomy: "draft only for commercial commitments"},
		{Name: "Invoicing and Accounting", Domain: "finance", Role: "connect invoices, bills, overdue amounts, payments, and financial obligations to safe review queues.", Signals: []string{"invoice due date", "payment status", "amount", "vendor or customer"}, Workflows: []string{"detect overdue invoices", "prepare payment review", "summarize cash obligations"}, Risk: "high", Autonomy: "approval required for financial action"},
		{Name: "Website and eCommerce", Domain: "public presence", Role: "track orders, forms, public pages, and publishable content without making unsupported public changes.", Signals: []string{"order status", "form submission", "page change", "abandoned cart"}, Workflows: []string{"triage customer submissions", "draft content updates", "flag public publishing approvals"}, Risk: "high", Autonomy: "draft only for public publishing"},
		{Name: "Inventory", Domain: "stock and assets", Role: "monitor stock levels, reservations, receiving, delivery readiness, and missing materials.", Signals: []string{"on-hand quantity", "reserved quantity", "reorder rule", "picking status"}, Workflows: []string{"detect low stock", "prepare procurement tasks", "flag delivery blockers"}, Risk: "medium", Autonomy: "suggest and checklist, approval for stock moves"},
		{Name: "Purchase", Domain: "procurement", Role: "track supplier requests, purchase orders, approvals, expected receipts, and price changes.", Signals: []string{"RFQ status", "vendor", "expected arrival", "purchase amount"}, Workflows: []string{"chase supplier replies", "flag late receipts", "prepare approval packages"}, Risk: "high", Autonomy: "approval required for spend"},
		{Name: "Manufacturing", Domain: "production", Role: "map bills of materials, work orders, shortages, and completion blockers into operational steps.", Signals: []string{"work order status", "component shortage", "planned date", "quality check"}, Workflows: []string{"detect blocked production", "prepare material checklist", "flag overdue work orders"}, Risk: "medium", Autonomy: "execute only low-risk status checklists"},
		{Name: "Project", Domain: "project delivery", Role: "turn project tasks, blockers, assignees, milestones, and due dates into HAI work queues.", Signals: []string{"task stage", "assignee", "deadline", "blocked reason"}, Workflows: []string{"clear blockers", "generate next actions", "summarize project health"}, Risk: "medium", Autonomy: "safe admin updates only"},
		{Name: "Timesheets", Domain: "capacity and billing", Role: "connect time entries, missing logs, billability, and workload imbalance to review.", Signals: []string{"timesheet entry", "billable flag", "employee", "project"}, Workflows: []string{"detect missing time", "prepare billing summary", "flag overloaded weeks"}, Risk: "medium", Autonomy: "suggest corrections, approval for billed changes"},
		{Name: "Helpdesk", Domain: "support", Role: "route tickets, customer pain, SLA risk, and repeated issues into workflows and knowledge.", Signals: []string{"ticket priority", "SLA deadline", "customer", "status"}, Workflows: []string{"prioritize urgent tickets", "draft customer updates", "detect repeat incidents"}, Risk: "medium", Autonomy: "draft customer replies, send only with approval when sensitive"},
		{Name: "Field Service", Domain: "field operations", Role: "coordinate appointments, locations, materials, travel dependencies, and completion evidence.", Signals: []string{"appointment time", "address", "task status", "materials needed"}, Workflows: []string{"prepare visit checklist", "detect impossible schedules", "draft after-job summary"}, Risk: "medium", Autonomy: "admin reminders and checklists"},
		{Name: "Planning", Domain: "resource planning", Role: "monitor shifts, workload, availability, and conflicts before they become missed commitments.", Signals: []string{"resource allocation", "shift", "conflict", "capacity"}, Workflows: []string{"flag schedule conflicts", "suggest workload moves", "prepare staffing reminders"}, Risk: "medium", Autonomy: "suggest only for people assignments"},
		{Name: "Employees and HR", Domain: "people operations", Role: "surface HR tasks, documents, onboarding, absence, and sensitive people records with strict privacy.", Signals: []string{"employee record", "contract date", "leave request", "document"}, Workflows: []string{"prepare onboarding checklist", "flag expiring documents", "route HR approvals"}, Risk: "high", Autonomy: "review-gated only"},
		{Name: "Expenses", Domain: "finance", Role: "track receipts, reimbursements, policy gaps, and missing evidence.", Signals: []string{"receipt", "amount", "approval status", "employee"}, Workflows: []string{"collect missing receipts", "prepare reimbursement review", "flag policy exceptions"}, Risk: "high", Autonomy: "approval required for financial action"},
		{Name: "Documents and Sign", Domain: "documents", Role: "connect contracts, signed files, evidence, folders, and document requests to provenance-backed workflows.", Signals: []string{"document owner", "signature status", "folder", "version"}, Workflows: []string{"detect unsigned documents", "prepare evidence bundles", "link source files to cases"}, Risk: "high", Autonomy: "never delete or sign automatically"},
		{Name: "Calendar and Appointments", Domain: "time", Role: "turn appointments, meetings, deadlines, and scheduling conflicts into preparation and follow-up work.", Signals: []string{"event date", "attendee", "location", "booking status"}, Workflows: []string{"create preparation tasks", "flag conflicts", "draft reschedule options"}, Risk: "medium", Autonomy: "low-risk reminders only"},
		{Name: "Discuss and Contacts", Domain: "communication", Role: "link people, threads, commitments, and unanswered messages to projects and follow-ups.", Signals: []string{"message thread", "contact role", "company", "last reply"}, Workflows: []string{"detect unanswered threads", "summarize commitments", "draft next reply"}, Risk: "high", Autonomy: "draft only for external communication"},
		{Name: "Marketing and Social", Domain: "public communication", Role: "plan campaigns, audience messages, and publishing work with source-grounded approval controls.", Signals: []string{"campaign status", "mailing result", "social post", "audience segment"}, Workflows: []string{"draft campaign updates", "flag public posting approvals", "review unsupported claims"}, Risk: "high", Autonomy: "draft only for public content"},
		{Name: "Knowledge", Domain: "organizational memory", Role: "turn internal notes, procedures, and repeated answers into verified HAI memory and source-grounded responses.", Signals: []string{"article update", "procedure", "owner", "tag"}, Workflows: []string{"refresh procedural memory", "detect stale guidance", "propose knowledge updates"}, Risk: "medium", Autonomy: "propose memory updates with source links"},
		{Name: "Point of Sale", Domain: "front-office sales", Role: "connect POS orders, sessions, cash movements, returns, and stock effects to reviewable operations.", Signals: []string{"session status", "order", "refund", "cash difference"}, Workflows: []string{"flag cash differences", "summarize daily sales", "detect refund review needs"}, Risk: "high", Autonomy: "approval required for cash or refund action"},
		{Name: "Subscriptions", Domain: "recurring revenue", Role: "track renewal dates, failed payments, churn risk, and customer commitments.", Signals: []string{"renewal date", "subscription status", "MRR", "payment failure"}, Workflows: []string{"draft renewal follow-ups", "flag failed payments", "surface churn risk"}, Risk: "medium", Autonomy: "draft only for customer commitments"},
	}
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

func (s *service) connectorByKey(connectorKey string) (models.SourceConnector, error) {
	connectors, err := s.repo.FindConnectors()
	if err != nil {
		return models.SourceConnector{}, err
	}
	for _, connector := range connectors {
		if connector.ConnectorKey == connectorKey {
			return connector, nil
		}
	}
	return models.SourceConnector{}, fmt.Errorf("connector %s is not registered", connectorKey)
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
	existing.Content = item.Content
	existing.Metadata = item.Metadata
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
	existing.Sensitive = source.ConnectorKey == "whatsapp-export" || containsAny(strings.ToLower(clean), "password", "secret", "token", "bank", "invoice", "contract", "legal", "medical", "juridisch", "medisch", "rekening", "factuur")
	existing.Uncertain = len(clean) < 40 || containsAny(strings.ToLower(clean), "maybe", "unclear", "unknown")
	existing.LastIndexedAt = &now
	return s.repo.SaveExtraction(existing)
}

func (s *service) storeUsefulMemory(source *models.ConnectedSource, extraction *models.SourceExtraction) {
	if s.memoryService == nil || source == nil || extraction == nil {
		return
	}
	if extraction.Sensitive || extraction.Uncertain || extraction.Summary == "" {
		return
	}
	created, err := s.memoryService.Create(memory.CreateRequest{
		ProjectKey:  extraction.ProjectKey,
		Kind:        "source",
		Content:     extraction.Summary,
		Summary:     extraction.Summary,
		Tags:        []string{"connected-source", source.Category, source.ConnectorKey},
		Confidence:  0.68,
		SourceURI:   extraction.SourceURI,
		SourceLabel: extraction.SourceLabel,
	})
	if err != nil {
		s.audit(source.ID, "memory.source_create_failed", compact(safety.RedactSecrets(err.Error()), 240))
		return
	}
	if created != nil {
		s.autoLinkPursuitMemory(source, extraction, created)
	}
}

func (s *service) rememberExtractionCorrection(before, after *models.SourceExtraction) {
	if s.memoryService == nil || before == nil || after == nil || !extractionCorrectionUseful(before, after) {
		return
	}
	source, _ := s.repo.FindSource(after.SourceID)
	request := extractionCorrectionMemoryRequest(source, before, after)
	created, err := s.memoryService.Create(request)
	if err != nil {
		s.audit(after.SourceID, "extraction.correction_memory_failed", compact(safety.RedactSecrets(err.Error()), 240))
		return
	}
	if created != nil {
		s.autoLinkPursuitMemory(source, after, created)
	}
	s.audit(after.SourceID, "extraction.correction_memory_created", "stored reviewable source correction lesson")
}

func extractionCorrectionUseful(before, after *models.SourceExtraction) bool {
	if before.ID == uuid.Nil || after.ID == uuid.Nil || before.ID != after.ID {
		return false
	}
	fields := []struct {
		left  string
		right string
	}{
		{before.ProjectKey, after.ProjectKey},
		{before.Summary, after.Summary},
		{before.Entities, after.Entities},
		{before.Dates, after.Dates},
		{before.Tasks, after.Tasks},
		{before.Decisions, after.Decisions},
		{before.FollowUps, after.FollowUps},
	}
	for _, field := range fields {
		if normalizeSpaces(field.left) != normalizeSpaces(field.right) {
			return true
		}
	}
	return before.Sensitive != after.Sensitive || before.Uncertain != after.Uncertain
}

func extractionCorrectionMemoryRequest(source *models.ConnectedSource, before, after *models.SourceExtraction) memory.CreateRequest {
	sourceLabel := firstNonEmpty(after.SourceLabel, "Corrected connected-source extraction")
	sourceURI := firstNonEmpty(after.SourceURI, "source-extraction://"+after.ID.String())
	tags := []string{"connected-source", "source-correction", "correction"}
	if source != nil {
		tags = append(tags, source.Category, source.ConnectorKey)
	}
	tags = append(tags, after.ContentType, after.ProjectKey)

	if after.Sensitive {
		content := strings.Join([]string{
			"Robert corrected a sensitive connected-source extraction.",
			"Future behavior: keep similar records review-gated, avoid storing raw sensitive content as memory, and ask for confirmation before workflow, task, or memory use.",
		}, " ")
		return memory.CreateRequest{
			ProjectKey:  after.ProjectKey,
			Kind:        "lesson",
			Content:     content,
			Summary:     "Sensitive source correction requires review before future use.",
			Tags:        append(tags, "sensitive", "review-required"),
			Confidence:  0.62,
			SourceURI:   "source-extraction://" + after.ID.String(),
			SourceLabel: "Sensitive connected-source correction",
		}
	}

	changedFields := extractionCorrectionChangedFields(before, after)
	content := strings.Join([]string{
		"Robert corrected connected-source extraction behavior.",
		"Changed fields: " + strings.Join(changedFields, ", ") + ".",
		extractionCorrectionValue("Previous summary", before.Summary),
		extractionCorrectionValue("Revised summary", after.Summary),
		extractionCorrectionValue("Previous tasks", before.Tasks),
		extractionCorrectionValue("Revised tasks", after.Tasks),
		extractionCorrectionValue("Previous follow-ups", before.FollowUps),
		extractionCorrectionValue("Revised follow-ups", after.FollowUps),
		"Future behavior: prefer Robert-corrected project matching, task extraction, follow-up detection, and review gating for similar connected-source records; if source evidence conflicts, mark needs_review.",
	}, " ")

	confidence := 0.78
	if after.Uncertain {
		confidence = 0.66
		tags = append(tags, "uncertain", "review-required")
	}
	return memory.CreateRequest{
		ProjectKey:  after.ProjectKey,
		Kind:        "lesson",
		Content:     compact(content, 1300),
		Summary:     compact("Learn from source correction for "+sourceLabel+": "+strings.Join(changedFields, ", "), 240),
		Tags:        tags,
		Confidence:  confidence,
		SourceURI:   safety.RedactURL(sourceURI),
		SourceLabel: sourceLabel,
	}
}

func extractionCorrectionChangedFields(before, after *models.SourceExtraction) []string {
	fields := []struct {
		name  string
		left  string
		right string
	}{
		{"project", before.ProjectKey, after.ProjectKey},
		{"summary", before.Summary, after.Summary},
		{"entities", before.Entities, after.Entities},
		{"dates", before.Dates, after.Dates},
		{"tasks", before.Tasks, after.Tasks},
		{"decisions", before.Decisions, after.Decisions},
		{"follow-ups", before.FollowUps, after.FollowUps},
	}
	changed := []string{}
	for _, field := range fields {
		if normalizeSpaces(field.left) != normalizeSpaces(field.right) {
			changed = append(changed, field.name)
		}
	}
	if before.Sensitive != after.Sensitive {
		changed = append(changed, "sensitivity")
	}
	if before.Uncertain != after.Uncertain {
		changed = append(changed, "uncertainty")
	}
	if len(changed) == 0 {
		return []string{"operator correction"}
	}
	return changed
}

func extractionCorrectionValue(label, value string) string {
	value = strings.TrimSpace(safety.RedactSecrets(value))
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s.", label, compact(value, 260))
}

func (s *service) createWorkflowFromExtraction(source *models.ConnectedSource, extraction *models.SourceExtraction) error {
	if s.workflowService == nil {
		return nil
	}
	taskSignal := firstNonEmpty(extraction.Tasks, extraction.FollowUps)
	if taskSignal == "" {
		return nil
	}
	input := strings.Join([]string{
		firstNonEmpty(extraction.Summary, extraction.SourceLabel),
		"Tasks: " + extraction.Tasks,
		"Follow-ups: " + extraction.FollowUps,
		"Dates: " + extraction.Dates,
	}, "\n")
	record, err := s.workflowService.Intake(workflow.IntakeRequest{
		Input:          input,
		ProjectKey:     extraction.ProjectKey,
		SourceType:     source.Category,
		SourceID:       extraction.ID.String(),
		SourceURI:      firstNonEmpty(extraction.SourceURI, "source-extraction://"+extraction.ID.String()),
		SourceLabel:    extraction.SourceLabel,
		Trigger:        "source.extraction",
		Actor:          "source-worker",
		RequiresReview: extraction.Uncertain || extraction.Sensitive,
		ReviewReason:   extractionReviewReason(extraction),
	})
	if err != nil {
		s.audit(source.ID, "workflow.intake_failed", err.Error())
		return err
	}
	if record != nil {
		s.autoLinkPursuitWorkflow(source, extraction, record, input)
	}
	s.audit(source.ID, "workflow.intake_created", "actionable extraction sent to workflow engine")
	return nil
}

func (s *service) autoLinkPursuitWorkflow(source *models.ConnectedSource, extraction *models.SourceExtraction, record *workflow.WorkflowRecord, input string) {
	if s.pursuitLinker == nil || source == nil || record == nil || record.Item.ID == uuid.Nil {
		return
	}
	request := pursuit.AutoLinkWorkflowRequest{
		WorkflowID:           record.Item.ID,
		Input:                input,
		ProjectKey:           source.DefaultProjectKey,
		SourceType:           source.Category,
		SourceID:             source.ID.String(),
		SourceURI:            safety.RedactURL(source.SyncTarget),
		SourceLabel:          source.Name,
		Actor:                "source-worker",
		AllowCreateCandidate: true,
	}
	if extraction != nil {
		request.ProjectKey = firstNonEmpty(extraction.ProjectKey, source.DefaultProjectKey)
		request.SourceType = source.Category
		request.SourceID = extraction.ID.String()
		request.SourceURI = firstNonEmpty(extraction.SourceURI, "source-extraction://"+extraction.ID.String())
		request.SourceLabel = firstNonEmpty(extraction.SourceLabel, source.Name)
		request.ExtractionID = extraction.ID.String()
		if extraction.RawItemID != uuid.Nil {
			request.RawItemID = extraction.RawItemID.String()
		}
	}
	result, err := s.pursuitLinker.AutoLinkWorkflow(request)
	if err != nil {
		s.audit(source.ID, "pursuit.auto_link_failed", compact(err.Error(), 260))
		return
	}
	if result != nil && result.Linked {
		s.audit(source.ID, "pursuit.auto_linked", fmt.Sprintf("workflow %s linked to pursuit %s with %.2f confidence", record.Item.ID, result.PursuitID, result.Score))
		return
	}
	if result != nil && result.Message != "" {
		s.audit(source.ID, "pursuit.auto_link_skipped", result.Message)
	}
}

func (s *service) autoLinkPursuitMemory(source *models.ConnectedSource, extraction *models.SourceExtraction, memoryRecord *models.ContextMemory) {
	if s.pursuitLinker == nil || source == nil || extraction == nil || memoryRecord == nil || memoryRecord.ID == uuid.Nil {
		return
	}
	request := pursuit.AutoLinkMemoryRequest{
		MemoryID:             memoryRecord.ID,
		Input:                firstNonEmpty(extraction.Summary, memoryRecord.Summary, memoryRecord.Content),
		ProjectKey:           firstNonEmpty(extraction.ProjectKey, source.DefaultProjectKey, memoryRecord.ProjectKey),
		SourceURI:            firstNonEmpty(memoryRecord.SourceURI, extraction.SourceURI, "source-extraction://"+extraction.ID.String()),
		SourceLabel:          firstNonEmpty(memoryRecord.SourceLabel, extraction.SourceLabel, source.Name),
		Actor:                "source-worker",
		AllowCreateCandidate: false,
	}
	result, err := s.pursuitLinker.AutoLinkMemory(request)
	if err != nil {
		s.audit(source.ID, "pursuit.memory_auto_link_failed", compact(err.Error(), 260))
		return
	}
	if result != nil && result.Linked {
		s.audit(source.ID, "pursuit.memory_auto_linked", fmt.Sprintf("memory %s linked to pursuit %s with %.2f confidence", memoryRecord.ID, result.PursuitID, result.Score))
		return
	}
	if result != nil && result.Message != "" {
		s.audit(source.ID, "pursuit.memory_auto_link_skipped", result.Message)
	}
}

func extractionReviewReason(extraction *models.SourceExtraction) string {
	reasons := []string{}
	if extraction.Uncertain {
		reasons = append(reasons, "extraction is uncertain")
	}
	if extraction.Sensitive {
		reasons = append(reasons, "extraction contains sensitive content")
	}
	return strings.Join(reasons, "; ")
}

func (s *service) reconcileWorkflowFromExtraction(extraction *models.SourceExtraction) error {
	if firstNonEmpty(extraction.Tasks, extraction.FollowUps) == "" {
		return nil
	}
	source, err := s.repo.FindSource(extraction.SourceID)
	if err != nil {
		s.audit(extraction.SourceID, "workflow.reconcile_failed", err.Error())
		return err
	}
	if err := s.createWorkflowFromExtraction(source, extraction); err != nil {
		s.audit(extraction.SourceID, "workflow.reconcile_failed", err.Error())
		return err
	}
	return nil
}

func (s *service) retractWorkflowForExtraction(extraction *models.SourceExtraction, reason string) error {
	if s.workflowService == nil {
		return nil
	}
	source, err := s.repo.FindSource(extraction.SourceID)
	if err != nil {
		return err
	}
	return s.workflowService.RetractSource(source.Category, extraction.ID.String(), reason)
}

func (s *service) indexExtraction(extraction *models.SourceExtraction) error {
	keywords := strings.Join(mapKeys(tokenSet(extraction.Text+" "+extraction.Summary+" "+extraction.Entities+" "+extraction.Tasks)), ",")
	if _, err := s.repo.SaveIndexEntry(&models.SourceIndexEntry{
		SourceID:     extraction.SourceID,
		ExtractionID: extraction.ID,
		ProjectKey:   extraction.ProjectKey,
		IndexType:    "keyword",
		Keywords:     keywords,
	}); err != nil {
		return err
	}
	if _, err := s.repo.SaveIndexEntry(&models.SourceIndexEntry{
		SourceID:     extraction.SourceID,
		ExtractionID: extraction.ID,
		ProjectKey:   extraction.ProjectKey,
		IndexType:    "vector_ref",
		VectorRef:    "local-vector-pending:" + extraction.ID.String(),
	}); err != nil {
		return err
	}
	return nil
}

func (s *service) beginSync(sourceID uuid.UUID) bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.activeSyncs == nil {
		s.activeSyncs = map[uuid.UUID]bool{}
	}
	if s.activeSyncs[sourceID] {
		return false
	}
	s.activeSyncs[sourceID] = true
	return true
}

func (s *service) endSync(sourceID uuid.UUID) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	delete(s.activeSyncs, sourceID)
}

func itemFailure(item ImportItem, stage string, err error) string {
	label := firstNonEmpty(item.ExternalID, item.Title, "unknown item")
	return compact(fmt.Sprintf("%s: %s: %v", label, stage, err), 320)
}

func (s *service) audit(sourceID uuid.UUID, action, message string) {
	_, _ = s.repo.SaveAuditLog(&models.SourceAuditLog{
		SourceID: sourceID,
		Action:   action,
		Message:  message,
	})
}

// Adapter status values. These describe honestly what a connector actually does,
// which is not the same question as whether it can be used:
//
//	AdapterOperational — connects to and fetches from the live external service.
//	AdapterLocalOnly   — functional, but ingests from local files/folders/exports
//	                     rather than the live cloud service its name suggests.
//	AdapterModeled     — functional, but produces built-in domain models rather
//	                     than reading a real source.
//	AdapterNotImplemented — registered as a contract only; no working adapter.
//
// All but AdapterNotImplemented are "usable" (see adapterIsUsable): a source can
// be created and will ingest. The distinction exists so the UI can stop
// reporting a local-folder reader as a live Gmail/Trello/Drive connector.
const (
	AdapterOperational    = "operational"
	AdapterLocalOnly      = "local_only"
	AdapterModeled        = "modeled"
	AdapterNotImplemented = "not_implemented"
)

// adapterIsUsable reports whether a connector with the given status can back a
// created source. Everything except an unimplemented (or unset) adapter can.
func adapterIsUsable(status string) bool {
	switch strings.TrimSpace(status) {
	case AdapterOperational, AdapterLocalOnly, AdapterModeled:
		return true
	default:
		return false
	}
}

func defaultConnectors() []models.SourceConnector {
	modes := joinValues([]string{ModeManualImport, ModeScheduledSync, ModeWebhookSync, ModeHistoricalBackfill, ModeIncrementalSync})
	return []models.SourceConnector{
		{ConnectorKey: "email", Name: "Email exports (MBOX/EML)", Category: "email", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "reads MBOX/EML export files from an allowlisted local folder; does not connect to Gmail/IMAP"},
		{ConnectorKey: "calendar", Name: "Calendar exports (ICS)", Category: "calendar", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "reads ICS export files from an allowlisted local folder; does not connect to Google/Outlook Calendar"},
		{ConnectorKey: "cloud-documents", Name: "Synced cloud document folders", Category: "cloud_document", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "reads a locally-synced folder (bounded by the folder allowlist); does not connect to a Drive/Dropbox API"},
		{ConnectorKey: "project-board", Name: "Trello project-board exports", Category: "project_board", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "reads Trello JSON export files from an allowlisted local folder; does not connect to the Trello API"},
		{ConnectorKey: "github", Name: "GitHub repositories and work", Category: "github", SupportedModes: modes, RequiredScopes: "metadata,read", LocalOnlyCapable: false, Enabled: true, AdapterStatus: AdapterOperational, StatusReason: "live read-only GitHub REST sync of repositories, issues, pull requests, commits, and workflow runs; optional token in GITHUB_SOURCE_TOKEN"},
		{ConnectorKey: "local-folder", Name: "Selected local folders", Category: "local_folder", SupportedModes: joinValues([]string{ModeManualImport, ModeScheduledSync, ModeFolderWatcher, ModeIncrementalSync}), RequiredScopes: "selected-folder-read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "manual and scheduled ingestion of an allowlisted local folder"},
		{ConnectorKey: "json-feed", Name: "Allowlisted JSON feed", Category: "generic_feed", SupportedModes: joinValues([]string{ModeManualImport, ModeScheduledSync, ModeHistoricalBackfill, ModeIncrementalSync}), RequiredScopes: "metadata,read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterOperational, StatusReason: "live scheduled and incremental fetch of a normalized JSON feed over HTTP, with host allowlisting and bounded responses"},
		{ConnectorKey: "whatsapp-export", Name: "WhatsApp exported chats", Category: "chat", SupportedModes: joinValues([]string{ModeManualImport, ModeScheduledSync, ModeHistoricalBackfill, ModeIncrementalSync}), RequiredScopes: "selected-chat-export-read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterLocalOnly, StatusReason: "parses local WhatsApp .txt export files into bounded, sensitive, review-gated records; does not connect to WhatsApp"},
		{ConnectorKey: "odoo-herp", Name: "Odoo / HERP operations", Category: "herp", SupportedModes: joinValues([]string{ModeManualImport, ModeScheduledSync, ModeHistoricalBackfill, ModeIncrementalSync}), RequiredScopes: "metadata,read,herp:read", LocalOnlyCapable: true, Enabled: true, AdapterStatus: AdapterModeled, StatusReason: "generates built-in Odoo app-domain models from manual selection; no live Odoo connection; write-back disabled by default"},
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

func sourceUsesLocalFolder(connectorKey string) bool {
	switch strings.TrimSpace(connectorKey) {
	case "local-folder", "email", "calendar", "cloud-documents", "project-board":
		return true
	default:
		return false
	}
}

func sourceHasNativeAdapter(connectorKey string) bool {
	return sourceUsesLocalFolder(connectorKey) || connectorKey == "json-feed" || connectorKey == "github" || connectorKey == "whatsapp-export" || connectorKey == "odoo-herp"
}

func filterConnectorLocalItems(items []ImportItem, connectorKey string) []ImportItem {
	allowed := map[string]bool{}
	itemType := ""
	switch connectorKey {
	case "email":
		allowed = map[string]bool{".mbox": true, ".eml": true}
		itemType = "email_export"
	case "calendar":
		allowed = map[string]bool{".ics": true}
		itemType = "calendar_export"
	case "project-board":
		allowed = map[string]bool{".json": true}
		itemType = "project_board_export"
	default:
		return items
	}
	filtered := make([]ImportItem, 0, len(items))
	for _, item := range items {
		ext := strings.ToLower(filepath.Ext(item.Title))
		if !allowed[ext] {
			continue
		}
		item.ItemType = itemType
		filtered = append(filtered, item)
	}
	return filtered
}

func minimalPermissions(category string, requested []string) []string {
	if len(requested) == 0 {
		return []string{"metadata:read", category + ":read"}
	}
	allowed := map[string]bool{"metadata:read": true, category + ":read": true, "selected-folder-read": true, "selected-chat-export-read": true, "herp:read": true, "odoo:read": true}
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
	if !sourceHasNativeAdapter(source.ConnectorKey) {
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

type jsonFeedEnvelope struct {
	Items      []ImportItem `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

func fetchJSONFeed(source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	target, err := url.Parse(strings.TrimSpace(source.SyncTarget))
	if err != nil || target.Scheme == "" || target.Hostname() == "" {
		return nil, "", fmt.Errorf("json-feed sync target must be an absolute HTTP(S) URL")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, "", fmt.Errorf("json-feed sync target must use HTTP or HTTPS")
	}
	if target.User != nil {
		return nil, "", fmt.Errorf("json-feed credentials must not be embedded in syncTarget")
	}
	if !sourceHTTPHostAllowed(target.Hostname()) {
		return nil, "", fmt.Errorf("json-feed host %s is not allowlisted; set CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS deliberately", target.Hostname())
	}
	if sourceHTTPAddressBlocked(target.Hostname()) {
		return nil, "", fmt.Errorf("json-feed target uses link-local, metadata, or unspecified address space")
	}
	if source.Cursor != "" {
		query := target.Query()
		query.Set("cursor", source.Cursor)
		target.RawQuery = query.Encode()
	}
	request, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("create json-feed request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout:   sourceHTTPTimeout(),
		Transport: sourceHTTPTransport(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("fetch json-feed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("json-feed returned HTTP %d", response.StatusCode)
	}
	maxBytes := sourceHTTPMaxBytes()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read json-feed: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, "", fmt.Errorf("json-feed response exceeds %d bytes", maxBytes)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return []ImportItem{}, source.Cursor, nil
	}
	if body[0] == '[' {
		var items []ImportItem
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, "", fmt.Errorf("decode json-feed items: %w", err)
		}
		return normalizeFeedItems(items, source), source.Cursor, nil
	}
	var envelope jsonFeedEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode json-feed envelope: %w", err)
	}
	return normalizeFeedItems(envelope.Items, source), firstNonEmpty(strings.TrimSpace(envelope.NextCursor), source.Cursor), nil
}

func fetchGitHubSource(source *models.ConnectedSource) ([]ImportItem, string, error) {
	if source == nil {
		return nil, "", fmt.Errorf("source is required")
	}
	repository := strings.Trim(strings.TrimSpace(source.SyncTarget), "/")
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, "", fmt.Errorf("github syncTarget must be an owner/repository slug")
	}
	base := strings.TrimRight(firstNonEmpty(os.Getenv("GITHUB_SOURCE_API_BASE_URL"), "https://api.github.com"), "/")
	parsedBase, err := url.Parse(base)
	if err != nil || (parsedBase.Scheme != "http" && parsedBase.Scheme != "https") || parsedBase.Hostname() == "" {
		return nil, "", fmt.Errorf("GITHUB_SOURCE_API_BASE_URL must be an absolute HTTP(S) URL")
	}
	if !sourceHTTPHostAllowed(parsedBase.Hostname()) || sourceHTTPAddressBlocked(parsedBase.Hostname()) {
		return nil, "", fmt.Errorf("github API host %s is not allowlisted", parsedBase.Hostname())
	}
	endpoints := []struct {
		path string
		kind string
	}{
		{"/repos/" + repository, "repository"},
		{"/repos/" + repository + "/issues", "issue"},
		{"/repos/" + repository + "/pulls", "pull_request"},
		{"/repos/" + repository + "/commits", "commit"},
		{"/repos/" + repository + "/actions/runs", "workflow_run"},
	}
	items := []ImportItem{}
	latest := strings.TrimSpace(source.Cursor)
	for _, endpoint := range endpoints {
		value, err := fetchGitHubJSON(parsedBase, endpoint.path, source.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("fetch github %s: %w", endpoint.kind, err)
		}
		records := githubRecords(value, endpoint.kind)
		for _, record := range records {
			item, updated := githubImportItem(record, endpoint.kind, source.DefaultProjectKey, repository)
			if item.ExternalID == "" || item.Content == "" {
				continue
			}
			items = append(items, item)
			if updated > latest {
				latest = updated
			}
		}
	}
	return items, latest, nil
}

func fetchGitHubJSON(base *url.URL, resourcePath, cursor string) (any, error) {
	target := *base
	target.Path = strings.TrimRight(base.Path, "/") + resourcePath
	query := target.Query()
	query.Set("per_page", "100")
	query.Set("state", "all")
	query.Set("sort", "updated")
	query.Set("direction", "asc")
	if cursor != "" && resourcePath != "" && resourcePath != "/repos/" {
		if _, err := time.Parse(time.RFC3339, cursor); err == nil && (strings.HasSuffix(resourcePath, "/issues") || strings.HasSuffix(resourcePath, "/pulls")) {
			query.Set("since", cursor)
		}
	}
	target.RawQuery = query.Encode()
	request, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "HAI-connected-source")
	if token := strings.TrimSpace(os.Getenv("GITHUB_SOURCE_TOKEN")); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: sourceHTTPTimeout(), Transport: sourceHTTPTransport(), CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, sourceHTTPMaxBytes()+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > sourceHTTPMaxBytes() {
		return nil, fmt.Errorf("GitHub response exceeds %d bytes", sourceHTTPMaxBytes())
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("decode GitHub response: %w", err)
	}
	return value, nil
}

func githubRecords(value any, kind string) []map[string]any {
	if object, ok := value.(map[string]any); ok {
		if kind == "workflow_run" {
			if runs, ok := object["workflow_runs"].([]any); ok {
				return githubRecordSlice(runs)
			}
		}
		return []map[string]any{object}
	}
	if list, ok := value.([]any); ok {
		return githubRecordSlice(list)
	}
	return nil
}

func githubRecordSlice(items []any) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			result = append(result, record)
		}
	}
	return result
}

func githubImportItem(record map[string]any, kind, projectKey, repository string) (ImportItem, string) {
	identifier := githubString(record, "node_id", "id", "sha", "number", "name")
	if identifier == "" {
		return ImportItem{}, ""
	}
	title := githubString(record, "title", "full_name", "name", "display_title", "sha")
	if title == "" {
		title = kind + " from " + repository
	}
	body := githubString(record, "body", "message", "description", "name", "status")
	if nested, ok := record["commit"].(map[string]any); ok {
		body = firstNonEmpty(body, githubString(nested, "message"))
	}
	content := strings.TrimSpace(strings.Join([]string{
		"GitHub " + strings.ReplaceAll(kind, "_", " ") + " from " + repository,
		"Title: " + title,
		"Status: " + githubString(record, "state", "status", "conclusion"),
		body,
	}, "\n"))
	updated := githubString(record, "updated_at", "created_at", "run_started_at", "timestamp")
	return ImportItem{
		ExternalID: "github:" + kind + ":" + identifier,
		Title:      compact(title, 500),
		Content:    compact(content, 12000),
		SourceURI:  githubString(record, "html_url", "url"),
		ItemType:   "github_" + kind,
		ProjectKey: projectKey,
		Metadata:   fmt.Sprintf("source=github;repository=%s;kind=%s;updated=%s", repository, kind, updated),
	}, updated
}

func githubString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			case float64:
				return strconv.FormatInt(int64(typed), 10)
			}
		}
	}
	return ""
}

func normalizeFeedItems(items []ImportItem, source *models.ConnectedSource) []ImportItem {
	result := make([]ImportItem, 0, len(items))
	for _, item := range items {
		item.ExternalID = strings.TrimSpace(item.ExternalID)
		item.Title = strings.TrimSpace(item.Title)
		item.Content = strings.TrimSpace(item.Content)
		item.ProjectKey = firstNonEmpty(strings.TrimSpace(item.ProjectKey), source.DefaultProjectKey)
		if item.ExternalID == "" || item.Content == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func sourceHTTPHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, allowed := range strings.Split(firstNonEmpty(os.Getenv("CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS"), defaultHTTPFeedAllowedHosts), ",") {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func sourceHTTPAddressBlocked(host string) bool {
	if envBool("CONNECTED_SOURCE_HTTP_ALLOW_LINK_LOCAL") {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	return ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.String() == "169.254.169.254"
}

func sourceHTTPTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: sourceHTTPTimeout()}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid json-feed network address: %w", err)
			}
			resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve json-feed host: %w", err)
			}
			for _, candidate := range resolved {
				if sourceHTTPAddressBlocked(candidate.IP.String()) {
					return nil, fmt.Errorf("json-feed host resolved to blocked address space")
				}
			}
			if len(resolved) == 0 {
				return nil, fmt.Errorf("json-feed host resolved to no addresses")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
		},
	}
}

func sourceHTTPTimeout() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CONNECTED_SOURCE_HTTP_TIMEOUT_SECONDS")))
	if err != nil || seconds < 1 || seconds > 120 {
		seconds = 20
	}
	return time.Duration(seconds) * time.Second
}

func sourceHTTPMaxBytes() int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("CONNECTED_SOURCE_HTTP_MAX_BYTES")), 10, 64)
	if err != nil || value < 1024 || value > 20*1024*1024 {
		return 2 * 1024 * 1024
	}
	return value
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
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

type whatsAppMessage struct {
	DateTime string
	Sender   string
	Body     string
}

func expandWhatsAppImportItems(items []ImportItem, projectKey, sourceName string, chunkMessages int) []ImportItem {
	if chunkMessages <= 0 || chunkMessages > 200 {
		chunkMessages = defaultWhatsAppChunkMessages
	}
	result := []ImportItem{}
	for _, item := range items {
		messages := parseWhatsAppMessages(item.Content)
		if len(messages) == 0 {
			normalized := item
			normalized.ItemType = firstNonEmpty(normalized.ItemType, "whatsapp_export")
			normalized.ProjectKey = firstNonEmpty(normalized.ProjectKey, projectKey)
			normalized.Metadata = firstNonEmpty(normalized.Metadata, "source=whatsapp-export;format=unparsed")
			result = append(result, normalized)
			continue
		}
		for start := 0; start < len(messages); start += chunkMessages {
			end := start + chunkMessages
			if end > len(messages) {
				end = len(messages)
			}
			window := messages[start:end]
			title := whatsAppWindowTitle(item, sourceName, window, start, end)
			result = append(result, ImportItem{
				ExternalID: firstNonEmpty(item.ExternalID, hashText(item.Title+item.SourceURI)) + fmt.Sprintf(":messages:%d-%d", start+1, end),
				Title:      title,
				Content:    renderWhatsAppWindow(window),
				SourceURI:  firstNonEmpty(item.SourceURI, "whatsapp-export://"+hashText(item.Title)),
				ItemType:   "whatsapp_chat_window",
				ProjectKey: firstNonEmpty(item.ProjectKey, projectKey),
				Metadata: fmt.Sprintf(
					"source=whatsapp-export;messages=%d;window_start=%d;window_end=%d;chat=%s",
					len(window),
					start+1,
					end,
					firstNonEmpty(item.Title, sourceName, "WhatsApp export"),
				),
			})
		}
	}
	return result
}

func parseWhatsAppMessages(content string) []whatsAppMessage {
	messages := []whatsAppMessage{}
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := whatsAppMessageLine.FindStringSubmatch(line)
		if len(matches) == 5 {
			messages = append(messages, whatsAppMessage{
				DateTime: strings.TrimSpace(matches[1] + " " + matches[2]),
				Sender:   strings.TrimSpace(matches[3]),
				Body:     strings.TrimSpace(matches[4]),
			})
			continue
		}
		if len(messages) > 0 {
			last := &messages[len(messages)-1]
			last.Body = strings.TrimSpace(last.Body + "\n" + strings.TrimSpace(line))
		}
	}
	return messages
}

func renderWhatsAppWindow(messages []whatsAppMessage) string {
	lines := []string{
		"WhatsApp conversation export window.",
		"Treat this as private connected-source evidence. Do not send, publish, or store as stable memory without Robert's approval.",
	}
	for _, message := range messages {
		lines = append(lines, fmt.Sprintf("%s | %s: %s", message.DateTime, message.Sender, message.Body))
	}
	return strings.Join(lines, "\n")
}

func whatsAppWindowTitle(item ImportItem, sourceName string, messages []whatsAppMessage, start, end int) string {
	chat := firstNonEmpty(item.Title, sourceName, "WhatsApp export")
	if len(messages) == 0 {
		return chat
	}
	return fmt.Sprintf("%s messages %d-%d (%s to %s)", chat, start+1, end, messages[0].DateTime, messages[len(messages)-1].DateTime)
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
		lower := strings.ToLower(clean)
		if strings.Contains(clean, "202") || strings.Contains(lower, "deadline") || strings.Contains(lower, "tomorrow") || strings.Contains(lower, "today") || strings.Contains(lower, "morgen") || strings.Contains(lower, "vandaag") || strings.Contains(lower, "maandag") || strings.Contains(lower, "dinsdag") || strings.Contains(lower, "woensdag") || strings.Contains(lower, "donderdag") || strings.Contains(lower, "vrijdag") {
			result = append(result, clean)
		}
	}
	return uniqueStrings(limitValues(result, 20))
}

func extractTasks(text string) []string {
	return extractSentences(text, "todo", "must", "should", "need to", "action", "task", "moet", "moeten", "nodig", "actie", "taak", "regelen", "uitzoeken", "oppakken")
}

func extractDecisions(text string) []string {
	return extractSentences(text, "decided", "decision", "approved", "rejected", "agreed", "besloten", "beslissing", "goedgekeurd", "afgewezen", "akkoord", "afgesproken")
}

func extractFollowUps(text string) []string {
	return extractSentences(text, "follow up", "waiting", "open loop", "remind", "next", "opvolgen", "wachten", "wacht op", "herinner", "reminder", "reageer", "antwoord", "volgende")
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

func slugText(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	builder := strings.Builder{}
	lastDash := false
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
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
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("connected-source root is not accessible: %w", err)
	}
	rootAbs, err = filepath.Abs(rootResolved)
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
	folderResolved, err := filepath.EvalSymlinks(folderAbs)
	if err != nil {
		return "", fmt.Errorf("folder path is not accessible: %w", err)
	}
	folderAbs, err = filepath.Abs(folderResolved)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(rootAbs, folderAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("folder path must not resolve outside allowlisted root %s", rootAbs)
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
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".yaml", ".yml", ".log", ".mbox", ".eml", ".ics":
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
