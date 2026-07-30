package source

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/whispercpp"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service     Service
	transcriber whispercpp.Service
}

func NewHandler(service Service, transcribers ...whispercpp.Service) *Handler {
	transcriber := whispercpp.DefaultService()
	if len(transcribers) > 0 && transcribers[0] != nil {
		transcriber = transcribers[0]
	}
	return &Handler{service: service, transcriber: transcriber}
}

func DefaultHandler() *Handler {
	return NewHandler(DefaultService())
}

func (h *Handler) Connectors(c *gin.Context) {
	connectors, err := h.service.Connectors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, connectors)
}

// StartGoogleOAuth returns the Google consent URL for a gmail source. The UI
// opens the returned url so the user authorizes in their own browser.
func (h *Handler) StartGoogleOAuth(c *gin.Context) {
	sourceID, err := uuid.Parse(c.Query("sourceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid sourceId query parameter is required"})
		return
	}
	url, err := h.service.StartGoogleOAuth(sourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorizeUrl": url})
}

// GoogleOAuthCallback is the redirect target Google calls after consent. It is
// reached without a session (Google calls it directly), so it is protected by
// the signed state rather than the gateway login. On success it redirects the
// browser back to the connected-sources page.
func (h *Handler) GoogleOAuthCallback(c *gin.Context) {
	if oauthErr := c.Query("error"); oauthErr != "" {
		c.Redirect(http.StatusFound, "/connected-sources?oauth=denied")
		return
	}
	_, err := h.service.CompleteGoogleOAuth(c.Request.Context(), c.Query("code"), c.Query("state"))
	if err != nil {
		c.Redirect(http.StatusFound, "/connected-sources?oauth=error")
		return
	}
	c.Redirect(http.StatusFound, "/connected-sources?oauth=connected")
}

func (h *Handler) CreateSource(c *gin.Context) {
	var request CreateSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.OwnerIdentity = sourceOwner(c)
	source, err := h.service.CreateSource(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, source)
}

func (h *Handler) Sources(c *gin.Context) {
	includeDisabled, _ := strconv.ParseBool(c.Query("includeDisabled"))
	sources, err := h.service.Sources(includeDisabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, filterVisibleSources(sources, sourceOwner(c)))
}

func (h *Handler) SyncJobs(c *gin.Context) {
	var sourceID *uuid.UUID
	if raw := c.Query("sourceId"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceId"})
			return
		}
		sourceID = &parsed
		if !h.requireSourceAccess(c, parsed) {
			return
		}
	}
	jobs, err := h.service.SyncJobs(sourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sourceID == nil {
		visibleSourceIDs, err := h.visibleSourceIDs(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		jobs = filterVisibleSyncJobs(jobs, visibleSourceIDs)
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *Handler) UpdateSource(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	var request UpdateSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	source, err := h.service.UpdateSource(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, source)
}

func (h *Handler) Sync(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	var request ImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Sync(id, request)
	if err != nil {
		if errors.Is(err, ErrSyncInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Transcribe invokes the opt-in local whisper.cpp runner for a configured
// whisper-audio source. It deliberately accepts no request body: the source's
// approved folder is the sole file scope, and the runner owns model/language
// configuration. The resulting text is persisted through the normal source
// sync path, preserving existing provenance, review, workflow, and audit gates.
func (h *Handler) Transcribe(c *gin.Context) {
	if c.Request.ContentLength != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcription uses the source's configured selected folder and accepts no caller-provided files, model, language, or audio"})
		return
	}
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	source, err := h.sourceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connected source not found"})
		return
	}
	if source.ConnectorKey != "whisper-audio" || !source.LocalOnly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcription requires an enabled local-only whisper-audio source"})
		return
	}
	folder, err := selectedAudioFolder(source.SyncTarget)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	transcripts, err := h.transcriber.Transcribe(c.Request.Context(), folder)
	if errors.Is(err, whispercpp.ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local whisper.cpp transcription runner is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local whisper.cpp transcription could not complete"})
		return
	}
	items := make([]ImportItem, 0, len(transcripts))
	for _, transcript := range transcripts {
		items = append(items, ImportItem{
			ExternalID: "whisper:" + transcript.Path,
			Title:      filepath.Base(transcript.Path),
			Content:    transcript.Text,
			SourceURI:  "audio://selected-source/" + id.String() + "/" + transcript.Path,
			ItemType:   "audio_transcript",
			ProjectKey: source.DefaultProjectKey,
			Metadata:   "engine=whisper.cpp;model=" + transcript.ModelID + ";language=" + transcript.Language + ";audio_retained=false;consent=source_owner",
		})
	}
	result, err := h.service.Sync(id, ImportRequest{Mode: ModeManualImport, Items: items, ProjectKey: source.DefaultProjectKey, controlledTranscription: true})
	if errors.Is(err, ErrSyncInProgress) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Reindex(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	result, err := h.service.Reindex(id)
	if err != nil {
		if errors.Is(err, ErrSyncInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RunDueScheduledSyncs(c *gin.Context) {
	ownerIdentity := sourceOwner(c)
	if ownerIdentity == "" {
		// The global scheduler is the only allowed ownerless source worker.
		// An HTTP request must carry a verified identity so it cannot trigger a
		// cross-owner refresh batch.
		c.JSON(http.StatusForbidden, gin.H{"error": "scheduled source sync requires an authenticated owner"})
		return
	}
	result, err := h.service.RunDueScheduledSyncsForOwner(time.Now().UTC(), ownerIdentity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Pause(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	source, err := h.service.Pause(id, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, source)
}

func (h *Handler) Resume(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	source, err := h.service.Pause(id, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, source)
}

func (h *Handler) Revoke(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableSource(c, id) {
		return
	}
	source, err := h.service.Revoke(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, source)
}

func (h *Handler) Search(c *gin.Context) {
	var request SearchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.OwnerIdentity = sourceOwner(c)
	result, err := h.service.Search(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Extractions(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	extractions, err := h.service.ExtractionsForOwner(sourceOwner(c), c.Query("projectKey"), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, extractions)
}

func (h *Handler) UpdateExtraction(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableExtraction(c, id) {
		return
	}
	var request models.SourceExtraction
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	extraction, err := h.service.UpdateExtraction(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, extraction)
}

func (h *Handler) ArchiveExtraction(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableExtraction(c, id) {
		return
	}
	extraction, err := h.service.ArchiveExtraction(id, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, extraction)
}

func (h *Handler) DeleteExtraction(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireMutableExtraction(c, id) {
		return
	}
	if err := h.service.DeleteExtraction(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AuditLogs(c *gin.Context) {
	var sourceID *uuid.UUID
	if raw := c.Query("sourceId"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceId"})
			return
		}
		sourceID = &parsed
		if !h.requireSourceAccess(c, parsed) {
			return
		}
	}
	logs, err := h.service.AuditLogs(sourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sourceID == nil {
		visibleSourceIDs, err := h.visibleSourceIDs(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		logs = filterVisibleAuditLogs(logs, visibleSourceIDs)
	}
	c.JSON(http.StatusOK, logs)
}

func sourceOwner(c *gin.Context) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok {
			return strings.TrimSpace(subject)
		}
	}
	return ""
}

func sourceVisible(source models.ConnectedSource, owner string) bool {
	owner = strings.TrimSpace(owner)
	return owner == "" || source.OwnerIdentity == "" || source.OwnerIdentity == owner
}

// sourceMutable is stricter than sourceVisible. Ownerless legacy records may
// remain readable during local migration, but a signed-in operator cannot
// adopt, sync, alter, revoke, or delete their source-derived records.
func sourceMutable(source models.ConnectedSource, owner string) bool {
	owner = strings.TrimSpace(owner)
	return owner != "" && strings.TrimSpace(source.OwnerIdentity) == owner
}

func filterVisibleSources(sources []models.ConnectedSource, owner string) []models.ConnectedSource {
	visible := make([]models.ConnectedSource, 0, len(sources))
	for _, source := range sources {
		if sourceVisible(source, owner) {
			visible = append(visible, source)
		}
	}
	return visible
}

func (h *Handler) visibleSourceIDs(c *gin.Context) (map[uuid.UUID]bool, error) {
	sources, err := h.service.Sources(true)
	if err != nil {
		return nil, err
	}
	visible := make(map[uuid.UUID]bool, len(sources))
	for _, source := range filterVisibleSources(sources, sourceOwner(c)) {
		visible[source.ID] = true
	}
	return visible, nil
}

func (h *Handler) sourceByID(id uuid.UUID) (*models.ConnectedSource, error) {
	sources, err := h.service.Sources(true)
	if err != nil {
		return nil, err
	}
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index], nil
		}
	}
	return nil, errors.New("connected source not found")
}

func selectedAudioFolder(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", errors.New("an explicit relative selected audio folder is required")
	}
	cleaned := filepath.ToSlash(filepath.Clean(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > 400 {
		return "", errors.New("audio folder must stay inside the selected intake root")
	}
	return cleaned, nil
}

func (h *Handler) requireSourceAccess(c *gin.Context, id uuid.UUID) bool {
	visibleSourceIDs, err := h.visibleSourceIDs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if visibleSourceIDs[id] {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "connected source not found"})
	return false
}

func (h *Handler) mutableSourceIDs(c *gin.Context) (map[uuid.UUID]bool, error) {
	sources, err := h.service.Sources(true)
	if err != nil {
		return nil, err
	}
	owner := sourceOwner(c)
	mutable := make(map[uuid.UUID]bool, len(sources))
	for _, source := range sources {
		if sourceMutable(source, owner) {
			mutable[source.ID] = true
		}
	}
	return mutable, nil
}

func (h *Handler) requireMutableSource(c *gin.Context, id uuid.UUID) bool {
	mutableSourceIDs, err := h.mutableSourceIDs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if mutableSourceIDs[id] {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "connected source not found"})
	return false
}

func (h *Handler) requireMutableExtraction(c *gin.Context, id uuid.UUID) bool {
	extractions, err := h.service.ExtractionsForOwner(sourceOwner(c), "", true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	mutableSourceIDs, err := h.mutableSourceIDs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	for _, extraction := range extractions {
		if extraction.ID == id && mutableSourceIDs[extraction.SourceID] {
			return true
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "source extraction not found"})
	return false
}

func filterVisibleSyncJobs(jobs []models.SourceSyncJob, sourceIDs map[uuid.UUID]bool) []models.SourceSyncJob {
	visible := make([]models.SourceSyncJob, 0, len(jobs))
	for _, job := range jobs {
		if sourceIDs[job.SourceID] {
			visible = append(visible, job)
		}
	}
	return visible
}

func filterVisibleAuditLogs(logs []models.SourceAuditLog, sourceIDs map[uuid.UUID]bool) []models.SourceAuditLog {
	visible := make([]models.SourceAuditLog, 0, len(logs))
	for _, log := range logs {
		if sourceIDs[log.SourceID] {
			visible = append(visible, log)
		}
	}
	return visible
}

func parseUUID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return uuid.UUID{}, false
	}
	return id, true
}
