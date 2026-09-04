package source

import (
	"automation-hub-backend/internal/apierror"
	"automation-hub-backend/internal/docling"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/whispercpp"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	service           Service
	transcriber       whispercpp.Service
	documentExtractor docling.Service
}

type ownerScopedSources interface {
	SourcesForOwner(ownerIdentity string, includeDisabled bool) ([]models.ConnectedSource, error)
}

type sourceHistoryService interface {
	SyncJobsForSources(sourceIDs []uuid.UUID, limit int) ([]models.SourceSyncJob, error)
	AuditLogsForSources(sourceIDs []uuid.UUID, limit int) ([]models.SourceAuditLog, error)
}

const (
	defaultExtractionPageLimit = 100
	maxExtractionPageLimit     = 500
	googleOAuthStateCookieName = "hai_google_oauth_state"
	googleOAuthStateCookieAge  = 10 * 60
)

type mutableSourceLookup interface {
	MutableSourceForOwner(id uuid.UUID, ownerIdentity string) (*models.ConnectedSource, error)
	MutableExtractionForOwner(id uuid.UUID, ownerIdentity string) (*models.SourceExtraction, error)
}

func NewHandler(service Service, transcribers ...whispercpp.Service) *Handler {
	transcriber := whispercpp.DefaultService()
	if len(transcribers) > 0 && transcribers[0] != nil {
		transcriber = transcribers[0]
	}
	return NewHandlerWithDocumentExtractor(service, docling.DefaultService(), transcriber)
}

func NewHandlerWithDocumentExtractor(service Service, extractor docling.Service, transcribers ...whispercpp.Service) *Handler {
	transcriber := whispercpp.DefaultService()
	if len(transcribers) > 0 && transcribers[0] != nil {
		transcriber = transcribers[0]
	}
	if extractor == nil {
		extractor = docling.DefaultService()
	}
	return &Handler{service: service, transcriber: transcriber, documentExtractor: extractor}
}

func DefaultHandler() *Handler {
	return NewHandler(DefaultService())
}

func (h *Handler) Connectors(c *gin.Context) {
	connectors, err := h.service.Connectors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "source connectors are unavailable")})
		return
	}
	c.JSON(http.StatusOK, connectors)
}

// StartGoogleOAuth returns the Google consent URL for a Google-backed source. The UI
// opens the returned url so the user authorizes in their own browser.
func (h *Handler) StartGoogleOAuth(c *gin.Context) {
	sourceID, err := uuid.Parse(c.Query("sourceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid sourceId query parameter is required"})
		return
	}
	if !h.requireMutableSource(c, sourceID) {
		return
	}
	url, err := h.service.StartGoogleOAuth(sourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "Google connection could not be started")})
		return
	}
	state, err := googleOAuthStateFromAuthorizeURL(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Google connection could not be started"})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(googleOAuthStateCookieName, state, googleOAuthStateCookieAge, "/api/v1/sources/oauth/google", "", googleOAuthCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"authorizeUrl": url})
}

// GoogleOAuthCallback is the redirect target the user's browser returns to
// after Google consent. The browser may not carry a HAI session, so the callback
// is protected by signed, expiring state. On success it returns the browser to
// the connected-sources page.
func (h *Handler) GoogleOAuthCallback(c *gin.Context) {
	defer clearGoogleOAuthStateCookie(c)
	if oauthErr := c.Query("error"); oauthErr != "" {
		c.Redirect(http.StatusFound, "/connected-sources?oauth=denied")
		return
	}
	state := c.Query("state")
	if !googleOAuthStateCookieMatches(c, state) {
		c.Redirect(http.StatusFound, "/connected-sources?oauth=error")
		return
	}
	_, err := h.service.CompleteGoogleOAuth(c.Request.Context(), c.Query("code"), state)
	if err != nil {
		c.Redirect(http.StatusFound, "/connected-sources?oauth=error")
		return
	}
	c.Redirect(http.StatusFound, "/connected-sources?oauth=connected")
}

func googleOAuthStateFromAuthorizeURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if state == "" {
		return "", errors.New("Google authorization URL omitted state")
	}
	return state, nil
}

func googleOAuthStateCookieMatches(c *gin.Context, state string) bool {
	stored, err := c.Cookie(googleOAuthStateCookieName)
	if err != nil || strings.TrimSpace(state) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(state)) == 1
}

func clearGoogleOAuthStateCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(googleOAuthStateCookieName, "", -1, "/api/v1/sources/oauth/google", "", googleOAuthCookieSecure(c), true)
}

func googleOAuthCookieSecure(c *gin.Context) bool {
	return c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func (h *Handler) CreateSource(c *gin.Context) {
	var request CreateSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connected source request is invalid"})
		return
	}
	request.OwnerIdentity = sourceOwner(c)
	source, err := h.service.CreateSource(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "connected source could not be created")})
		return
	}
	c.JSON(http.StatusCreated, source)
}

func (h *Handler) Sources(c *gin.Context) {
	includeDisabled, _ := strconv.ParseBool(c.Query("includeDisabled"))
	sources, err := h.sourcesForOwner(sourceOwner(c), includeDisabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected sources are unavailable")})
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
	if sourceID == nil {
		visibleSourceIDs, err := h.visibleSourceIDs(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected source history is unavailable")})
			return
		}
		jobs, err := h.recentSyncJobs(sourceIDsFromSet(visibleSourceIDs))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected source history is unavailable")})
			return
		}
		c.JSON(http.StatusOK, jobs)
		return
	}
	jobs, err := h.recentSyncJobs([]uuid.UUID{*sourceID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected source history is unavailable")})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *Handler) ConnectionHealth(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	if !h.requireSourceAccess(c, id) {
		return
	}
	healthService, ok := h.service.(ConnectionHealthService)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "connection health is not available"})
		return
	}
	health, err := healthService.ConnectionHealth(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "connection health is unavailable")})
		return
	}
	c.JSON(http.StatusOK, health)
}

// ConnectionHealths returns overview health only for sources visible to the
// authenticated owner. It is a local status derivation; it never probes or
// synchronizes external accounts.
func (h *Handler) ConnectionHealths(c *gin.Context) {
	healthService, ok := h.service.(ConnectionHealthBatchService)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "connection health is not available"})
		return
	}
	sources, err := h.sourcesForOwner(sourceOwner(c), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load connected sources"})
		return
	}
	health, err := healthService.ConnectionHealths(filterVisibleSources(sources, sourceOwner(c)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not derive connection health"})
		return
	}
	c.JSON(http.StatusOK, health)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "connected source update request is invalid"})
		return
	}
	source, err := h.service.UpdateSource(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "connected source could not be updated")})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "source sync request is invalid"})
		return
	}
	result, err := syncSourceWithContext(c.Request.Context(), h.service, id, request)
	if err != nil {
		if errors.Is(err, ErrSyncInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": "source sync is already in progress"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "source sync could not be completed")})
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
	source, ok := h.mutableSource(c, id)
	if !ok {
		return
	}
	if source.ConnectorKey != "whisper-audio" || !source.LocalOnly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcription requires an enabled local-only whisper-audio source"})
		return
	}
	folder, err := selectedAudioFolder(source.SyncTarget)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "audio source folder is invalid")})
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
	result, err := syncSourceWithContext(c.Request.Context(), h.service, id, ImportRequest{Mode: ModeManualImport, Items: items, ProjectKey: source.DefaultProjectKey, controlledTranscription: true})
	if errors.Is(err, ErrSyncInProgress) {
		c.JSON(http.StatusConflict, gin.H{"error": "source sync is already in progress"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "transcription results could not be saved")})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ExtractDocuments invokes the opt-in local Docling runner for a configured
// source. It accepts no browser supplied files, paths, models, or parser
// options; the source's approved folder remains the sole input scope.
func (h *Handler) ExtractDocuments(c *gin.Context) {
	if c.Request.ContentLength != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document extraction uses the source's configured selected folder and accepts no caller-provided files, model, or parser options"})
		return
	}
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	source, ok := h.mutableSource(c, id)
	if !ok {
		return
	}
	if source.ConnectorKey != doclingDocumentsConnectorKey || !source.LocalOnly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document extraction requires an enabled local-only Docling document source"})
		return
	}
	folder, err := selectedDocumentFolder(source.SyncTarget)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "document source folder is invalid")})
		return
	}
	documents, err := h.documentExtractor.Extract(c.Request.Context(), folder)
	if errors.Is(err, docling.ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local Docling document extractor is not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "local Docling document extraction could not complete"})
		return
	}
	items := make([]ImportItem, 0, len(documents))
	for _, document := range documents {
		items = append(items, ImportItem{
			ExternalID: "docling:" + document.Path,
			Title:      filepath.Base(document.Path),
			Content:    document.Text,
			SourceURI:  "document://selected-source/" + id.String() + "/" + document.Path,
			ItemType:   "document_extraction",
			ProjectKey: source.DefaultProjectKey,
			Metadata:   "engine=docling;format=" + document.Format + ";page_count=" + strconv.Itoa(document.PageCount) + ";content_digest=" + document.ContentDigest + ";source_retained=false;consent=source_owner",
		})
	}
	result, err := syncSourceWithContext(c.Request.Context(), h.service, id, ImportRequest{Mode: ModeManualImport, Items: items, ProjectKey: source.DefaultProjectKey, controlledDocumentExtraction: true})
	if errors.Is(err, ErrSyncInProgress) {
		c.JSON(http.StatusConflict, gin.H{"error": "source sync is already in progress"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "document extraction results could not be saved")})
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
			c.JSON(http.StatusConflict, gin.H{"error": "source sync is already in progress"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "source reindex could not be completed")})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "scheduled source sync could not be completed")})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "connected source could not be paused")})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "connected source could not be resumed")})
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
	destructive, ok := h.service.(DestructiveEffectService)
	if !ok {
		writeDestructiveEffectError(c, ErrDestructiveAuthorizationRequired)
		return
	}
	source, err := destructive.RevokeAuthorized(
		c.Request.Context(),
		id,
		destructiveAuthorization(c),
	)
	if err != nil {
		writeDestructiveEffectError(c, err)
		return
	}
	c.JSON(http.StatusOK, source)
}

func (h *Handler) Search(c *gin.Context) {
	var request SearchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connected source search request is invalid"})
		return
	}
	request.OwnerIdentity = sourceOwner(c)
	result, err := h.service.Search(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected source search is unavailable")})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Extractions(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	limit, err := extractionPageLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid extraction limit"})
		return
	}
	if paged, ok := h.service.(ExtractionPageService); ok {
		page, err := paged.ExtractionPageForOwner(sourceOwner(c), c.Query("projectKey"), includeArchived, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "source extractions are unavailable"})
			return
		}
		c.Header("X-Total-Count", strconv.FormatInt(page.TotalCount, 10))
		c.Header("X-Result-Limit", strconv.Itoa(page.Limit))
		c.JSON(http.StatusOK, page.Items)
		return
	}
	extractions, err := h.service.ExtractionsForOwner(sourceOwner(c), c.Query("projectKey"), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "source extractions are unavailable"})
		return
	}
	c.JSON(http.StatusOK, extractions)
}

func extractionPageLimit(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultExtractionPageLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxExtractionPageLimit {
		return 0, errors.New("limit must be between 1 and 500")
	}
	return limit, nil
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "extraction update request is invalid"})
		return
	}
	extraction, err := h.service.UpdateExtraction(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "extraction could not be updated")})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": apierror.PublicMessage(err, "extraction could not be archived")})
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
	destructive, ok := h.service.(DestructiveEffectService)
	if !ok {
		writeDestructiveEffectError(c, ErrDestructiveAuthorizationRequired)
		return
	}
	if err := destructive.DeleteExtractionAuthorized(
		c.Request.Context(),
		id,
		destructiveAuthorization(c),
	); err != nil {
		writeDestructiveEffectError(c, err)
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
	if sourceID == nil {
		visibleSourceIDs, err := h.visibleSourceIDs(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected-source access is unavailable")})
			return
		}
		logs, err := h.recentAuditLogs(sourceIDsFromSet(visibleSourceIDs))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected-source audit logs are unavailable")})
			return
		}
		c.JSON(http.StatusOK, logs)
		return
	}
	logs, err := h.recentAuditLogs([]uuid.UUID{*sourceID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected-source audit logs are unavailable")})
		return
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

func destructiveAuthorization(c *gin.Context) DestructiveEffectAuthorization {
	owner := sourceOwner(c)
	return DestructiveEffectAuthorization{
		OwnerIdentity:         owner,
		ActorIdentity:         owner,
		IdempotencyKey:        c.GetHeader("X-HAI-Idempotency-Key"),
		TaskID:                c.GetHeader("X-HAI-Task-ID"),
		ApprovalSourceID:      c.GetHeader("X-HAI-Approval-Source-ID"),
		ApprovalBindingDigest: c.GetHeader("X-HAI-Approval-Binding-Digest"),
	}
}

func writeDestructiveEffectError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDestructiveOwnerMismatch),
		errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "connected-source resource not found"})
	case errors.Is(err, ErrDestructiveAuthorizationRequired):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "destructive source changes require an approval service"})
	case errors.Is(err, ErrDestructiveAuthorizationDenied),
		errors.Is(err, ErrDestructiveAuthorizationMismatch):
		c.JSON(http.StatusForbidden, gin.H{"error": "destructive source change approval is invalid or unavailable"})
	case errors.Is(err, ErrSourceEmergencyStopActive):
		c.JSON(http.StatusLocked, gin.H{"error": "emergency stop is active; destructive source changes are blocked"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "destructive source change could not be completed"})
	}
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

func (h *Handler) sourcesForOwner(ownerIdentity string, includeDisabled bool) ([]models.ConnectedSource, error) {
	scoped, ok := h.service.(ownerScopedSources)
	if !ok {
		return nil, errors.New("owner-scoped source reads are unavailable")
	}
	return scoped.SourcesForOwner(ownerIdentity, includeDisabled)
}

func (h *Handler) recentSyncJobs(sourceIDs []uuid.UUID) ([]models.SourceSyncJob, error) {
	history, ok := h.service.(sourceHistoryService)
	if !ok {
		return nil, errors.New("source history reads are unavailable")
	}
	return history.SyncJobsForSources(sourceIDs, 100)
}

func (h *Handler) recentAuditLogs(sourceIDs []uuid.UUID) ([]models.SourceAuditLog, error) {
	history, ok := h.service.(sourceHistoryService)
	if !ok {
		return nil, errors.New("source history reads are unavailable")
	}
	return history.AuditLogsForSources(sourceIDs, 100)
}

func (h *Handler) visibleSourceIDs(c *gin.Context) (map[uuid.UUID]bool, error) {
	sources, err := h.sourcesForOwner(sourceOwner(c), true)
	if err != nil {
		return nil, err
	}
	visible := make(map[uuid.UUID]bool, len(sources))
	for _, source := range filterVisibleSources(sources, sourceOwner(c)) {
		visible[source.ID] = true
	}
	return visible, nil
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

func selectedDocumentFolder(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", errors.New("an explicit relative selected document folder is required")
	}
	cleaned := filepath.ToSlash(filepath.Clean(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > 400 {
		return "", errors.New("document folder must stay inside the selected intake root")
	}
	return cleaned, nil
}

func (h *Handler) requireSourceAccess(c *gin.Context, id uuid.UUID) bool {
	visibleSourceIDs, err := h.visibleSourceIDs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apierror.PublicMessage(err, "connected-source access is unavailable")})
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
	_, ok := h.mutableSource(c, id)
	return ok
}

// mutableSource resolves one source at the database boundary when the service
// supports it. Local runners need the source configuration after ownership is
// checked, so returning that exact record avoids a second full source-inventory
// read on every transcription or document extraction request.
func (h *Handler) mutableSource(c *gin.Context, id uuid.UUID) (*models.ConnectedSource, bool) {
	if lookup, ok := h.service.(mutableSourceLookup); ok {
		if source, err := lookup.MutableSourceForOwner(id, sourceOwner(c)); err == nil {
			return source, true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify connected source ownership"})
			return nil, false
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "connected source not found"})
		return nil, false
	}
	// Compatibility path for older service implementations used in focused tests
	// and downstream integrations that have not yet implemented exact lookups.
	sources, err := h.service.Sources(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify connected source ownership"})
		return nil, false
	}
	for index := range sources {
		if sources[index].ID == id && sourceMutable(sources[index], sourceOwner(c)) {
			return &sources[index], true
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "connected source not found"})
	return nil, false
}

func (h *Handler) requireMutableExtraction(c *gin.Context, id uuid.UUID) bool {
	if lookup, ok := h.service.(mutableSourceLookup); ok {
		if _, err := lookup.MutableExtractionForOwner(id, sourceOwner(c)); err == nil {
			return true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify source extraction ownership"})
			return false
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "source extraction not found"})
		return false
	}
	extractions, err := h.service.ExtractionsForOwner(sourceOwner(c), "", true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify source extraction ownership"})
		return false
	}
	mutableSourceIDs, err := h.mutableSourceIDs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify source extraction ownership"})
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
