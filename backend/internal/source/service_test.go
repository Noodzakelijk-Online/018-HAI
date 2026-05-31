package source

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestSyncLocalFolderExtractsReadableFilesWithProvenance(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root+"/project-note.md", "Decision: local folder ingestion should extract useful project context. Follow up: verify provenance before task planning.")
	writeTestFile(t, root+"/binary.bin", "\x00\x01ignored")
	t.Setenv("CONNECTED_SOURCE_LOCAL_ROOT", root)

	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID:              sourceID,
		ConnectorKey:    "local-folder",
		Name:            "Local project folder",
		Category:        "local_folder",
		Enabled:         true,
		LocalOnly:       true,
		Status:          "active",
		ExcludePatterns: "ignored",
	})
	mem := &fakeSourceMemoryService{}
	service := NewService(repo, mem)

	result, err := service.Sync(sourceID, ImportRequest{
		Mode:       ModeHistoricalBackfill,
		FolderPath: ".",
		ProjectKey: "018-HAI",
		Limit:      10,
		MaxBytes:   4096,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if result.Job.ItemsSeen != 1 {
		t.Fatalf("ItemsSeen = %d, want 1", result.Job.ItemsSeen)
	}
	if result.Job.ItemsAdded != 1 {
		t.Fatalf("ItemsAdded = %d, want 1", result.Job.ItemsAdded)
	}
	if len(result.Extractions) != 1 {
		t.Fatalf("extractions = %d, want 1", len(result.Extractions))
	}
	extraction := result.Extractions[0]
	if extraction.ProjectKey != "018-HAI" {
		t.Fatalf("ProjectKey = %q, want 018-HAI", extraction.ProjectKey)
	}
	if extraction.SourceLabel != "project-note.md" {
		t.Fatalf("SourceLabel = %q, want project-note.md", extraction.SourceLabel)
	}
	if !strings.HasPrefix(extraction.SourceURI, "file://") {
		t.Fatalf("SourceURI = %q, want file URI", extraction.SourceURI)
	}
	if !strings.Contains(extraction.Tasks, "Follow up") {
		t.Fatalf("Tasks = %q, want extracted follow up/task", extraction.Tasks)
	}
	if len(mem.created) != 1 {
		t.Fatalf("created memories = %d, want 1", len(mem.created))
	}
	if !repo.hasAudit("source.local_folder_scanned") || !repo.hasAudit("source.synced") {
		t.Fatalf("expected scan and sync audit records")
	}
}

func TestSyncLocalFolderBlocksTraversalOutsideAllowlistedRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CONNECTED_SOURCE_LOCAL_ROOT", root)

	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID:           sourceID,
		ConnectorKey: "local-folder",
		Name:         "Local project folder",
		Category:     "local_folder",
		Enabled:      true,
		LocalOnly:    true,
		Status:       "active",
	})
	service := NewService(repo, &fakeSourceMemoryService{})

	result, err := service.Sync(sourceID, ImportRequest{
		Mode:       ModeIncrementalSync,
		FolderPath: "..",
	})
	if err == nil {
		t.Fatalf("expected traversal error")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if len(repo.jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 failed job", len(repo.jobs))
	}
	if repo.jobs[0].Status != "failed" {
		t.Fatalf("job status = %q, want failed", repo.jobs[0].Status)
	}
	if !repo.hasAudit("source.sync_failed") {
		t.Fatalf("expected failed sync audit record")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

type fakeSourceRepo struct {
	connectors  map[string]models.SourceConnector
	sources     map[uuid.UUID]*models.ConnectedSource
	jobs        []models.SourceSyncJob
	rawItems    map[uuid.UUID]*models.SourceRawItem
	extractions map[uuid.UUID]*models.SourceExtraction
	index       []models.SourceIndexEntry
	auditLogs   []models.SourceAuditLog
}

func newFakeSourceRepo(sources ...*models.ConnectedSource) *fakeSourceRepo {
	repo := &fakeSourceRepo{
		connectors:  map[string]models.SourceConnector{},
		sources:     map[uuid.UUID]*models.ConnectedSource{},
		rawItems:    map[uuid.UUID]*models.SourceRawItem{},
		extractions: map[uuid.UUID]*models.SourceExtraction{},
	}
	for _, source := range sources {
		repo.sources[source.ID] = source
	}
	return repo
}

func (r *fakeSourceRepo) SaveConnector(connector *models.SourceConnector) (*models.SourceConnector, error) {
	if connector.ID == uuid.Nil {
		connector.ID = uuid.New()
	}
	r.connectors[connector.ConnectorKey] = *connector
	return connector, nil
}

func (r *fakeSourceRepo) FindConnectors() ([]models.SourceConnector, error) {
	result := []models.SourceConnector{}
	for _, connector := range r.connectors {
		result = append(result, connector)
	}
	return result, nil
}

func (r *fakeSourceRepo) CreateSource(source *models.ConnectedSource) (*models.ConnectedSource, error) {
	if source.ID == uuid.Nil {
		source.ID = uuid.New()
	}
	now := time.Now().UTC()
	source.CreatedAt = now
	source.UpdatedAt = now
	r.sources[source.ID] = source
	return source, nil
}

func (r *fakeSourceRepo) UpdateSource(source *models.ConnectedSource) (*models.ConnectedSource, error) {
	source.UpdatedAt = time.Now().UTC()
	r.sources[source.ID] = source
	return source, nil
}

func (r *fakeSourceRepo) FindSources(includeDisabled bool) ([]models.ConnectedSource, error) {
	result := []models.ConnectedSource{}
	for _, source := range r.sources {
		if includeDisabled || (source.Enabled && source.Status != "revoked") {
			result = append(result, *source)
		}
	}
	return result, nil
}

func (r *fakeSourceRepo) FindSource(id uuid.UUID) (*models.ConnectedSource, error) {
	source, ok := r.sources[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *source
	return &copied, nil
}

func (r *fakeSourceRepo) CreateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error) {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	r.jobs = append(r.jobs, *job)
	return job, nil
}

func (r *fakeSourceRepo) UpdateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error) {
	job.UpdatedAt = time.Now().UTC()
	for index := range r.jobs {
		if r.jobs[index].ID == job.ID {
			r.jobs[index] = *job
			return job, nil
		}
	}
	r.jobs = append(r.jobs, *job)
	return job, nil
}

func (r *fakeSourceRepo) FindSyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error) {
	result := []models.SourceSyncJob{}
	for _, job := range r.jobs {
		if sourceID == nil || job.SourceID == *sourceID {
			result = append(result, job)
		}
	}
	return result, nil
}

func (r *fakeSourceRepo) FindRawItem(sourceID uuid.UUID, externalID string) (*models.SourceRawItem, error) {
	for _, item := range r.rawItems {
		if item.SourceID == sourceID && item.ExternalID == externalID {
			copied := *item
			return &copied, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeSourceRepo) SaveRawItem(item *models.SourceRawItem) (*models.SourceRawItem, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	r.rawItems[item.ID] = item
	return item, nil
}

func (r *fakeSourceRepo) FindRawItems(sourceID uuid.UUID) ([]models.SourceRawItem, error) {
	result := []models.SourceRawItem{}
	for _, item := range r.rawItems {
		if item.SourceID == sourceID {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (r *fakeSourceRepo) FindExtractionByRawItem(rawItemID uuid.UUID) (*models.SourceExtraction, error) {
	for _, extraction := range r.extractions {
		if extraction.RawItemID == rawItemID {
			copied := *extraction
			return &copied, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeSourceRepo) SaveExtraction(extraction *models.SourceExtraction) (*models.SourceExtraction, error) {
	if extraction.ID == uuid.Nil {
		extraction.ID = uuid.New()
	}
	now := time.Now().UTC()
	if extraction.CreatedAt.IsZero() {
		extraction.CreatedAt = now
	}
	extraction.UpdatedAt = now
	r.extractions[extraction.ID] = extraction
	return extraction, nil
}

func (r *fakeSourceRepo) FindExtractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	result := []models.SourceExtraction{}
	for _, extraction := range r.extractions {
		if projectKey != "" && extraction.ProjectKey != projectKey {
			continue
		}
		if !includeArchived && extraction.Archived {
			continue
		}
		result = append(result, *extraction)
	}
	return result, nil
}

func (r *fakeSourceRepo) FindExtraction(id uuid.UUID) (*models.SourceExtraction, error) {
	extraction, ok := r.extractions[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *extraction
	return &copied, nil
}

func (r *fakeSourceRepo) DeleteExtraction(id uuid.UUID) error {
	delete(r.extractions, id)
	return nil
}

func (r *fakeSourceRepo) SaveIndexEntry(entry *models.SourceIndexEntry) (*models.SourceIndexEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	r.index = append(r.index, *entry)
	return entry, nil
}

func (r *fakeSourceRepo) SaveAuditLog(log *models.SourceAuditLog) (*models.SourceAuditLog, error) {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	log.CreatedAt = time.Now().UTC()
	r.auditLogs = append(r.auditLogs, *log)
	return log, nil
}

func (r *fakeSourceRepo) FindAuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error) {
	result := []models.SourceAuditLog{}
	for _, log := range r.auditLogs {
		if sourceID == nil || log.SourceID == *sourceID {
			result = append(result, log)
		}
	}
	return result, nil
}

func (r *fakeSourceRepo) hasAudit(action string) bool {
	for _, log := range r.auditLogs {
		if log.Action == action {
			return true
		}
	}
	return false
}

type fakeSourceMemoryService struct {
	created []memory.CreateRequest
}

func (s *fakeSourceMemoryService) Create(request memory.CreateRequest) (*models.ContextMemory, error) {
	s.created = append(s.created, request)
	return &models.ContextMemory{
		ID:         uuid.New(),
		ProjectKey: request.ProjectKey,
		Kind:       request.Kind,
		Content:    request.Content,
		Summary:    request.Summary,
		Confidence: request.Confidence,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}, nil
}

func (s *fakeSourceMemoryService) Update(id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return nil, nil
}

func (s *fakeSourceMemoryService) FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	return nil, nil
}

func (s *fakeSourceMemoryService) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *fakeSourceMemoryService) Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return nil, nil
}

func (s *fakeSourceMemoryService) Delete(id uuid.UUID) error {
	return nil
}

func (s *fakeSourceMemoryService) Retrieve(request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	return &memory.RetrieveResult{Query: request.Query}, nil
}
