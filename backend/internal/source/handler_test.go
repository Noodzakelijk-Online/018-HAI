package source

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/whispercpp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sourceTranscriberStub struct {
	transcripts []whispercpp.Transcript
	err         error
	folder      string
}

func (s *sourceTranscriberStub) Status() whispercpp.Status { return whispercpp.Status{} }
func (s *sourceTranscriberStub) Probe(context.Context) (*whispercpp.ProbeResult, error) {
	return &whispercpp.ProbeResult{Reachable: true}, nil
}
func (s *sourceTranscriberStub) Transcribe(_ context.Context, folder string) ([]whispercpp.Transcript, error) {
	s.folder = folder
	return s.transcripts, s.err
}

func TestHandlerOnlyListsVisibleSourcesAndRejectsForeignControls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(identity.LegacyDataOwnerEnv, "")
	aliceID := uuid.New()
	bobID := uuid.New()
	legacyID := uuid.New()
	service := NewService(newFakeSourceRepo(
		&models.ConnectedSource{ID: aliceID, OwnerIdentity: "alice", Name: "Alice source", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: bobID, OwnerIdentity: "bob", Name: "Bob source", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: legacyID, Name: "Ownerless legacy source", Enabled: true, Status: "active"},
	), nil)
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
	})
	router.GET("/sources", handler.Sources)
	router.POST("/sources/:id/pause", handler.Pause)

	listRequest := httptest.NewRequest(http.MethodGet, "/sources", nil)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", listResponse.Code, listResponse.Body.String())
	}
	var sources []models.ConnectedSource
	if err := json.Unmarshal(listResponse.Body.Bytes(), &sources); err != nil {
		t.Fatalf("decode sources: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != aliceID {
		t.Fatalf("visible sources = %#v, want only Alice source", sources)
	}

	t.Setenv(identity.LegacyDataOwnerEnv, "alice")
	legacyResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyResponse, httptest.NewRequest(http.MethodGet, "/sources", nil))
	if legacyResponse.Code != http.StatusOK {
		t.Fatalf("legacy-owner list status = %d, body=%s", legacyResponse.Code, legacyResponse.Body.String())
	}
	if err := json.Unmarshal(legacyResponse.Body.Bytes(), &sources); err != nil {
		t.Fatalf("decode legacy-owner sources: %v", err)
	}
	if len(sources) != 2 || !containsSourceID(sources, aliceID) || !containsSourceID(sources, legacyID) {
		t.Fatalf("legacy-owner visible sources = %#v, want Alice and ownerless legacy sources", sources)
	}

	unauthenticatedRouter := gin.New()
	unauthenticatedRouter.GET("/sources", handler.Sources)
	unauthenticatedResponse := httptest.NewRecorder()
	unauthenticatedRouter.ServeHTTP(unauthenticatedResponse, httptest.NewRequest(http.MethodGet, "/sources", nil))
	if unauthenticatedResponse.Code != http.StatusOK {
		t.Fatalf("ownerless request status = %d, body=%s", unauthenticatedResponse.Code, unauthenticatedResponse.Body.String())
	}
	if err := json.Unmarshal(unauthenticatedResponse.Body.Bytes(), &sources); err != nil {
		t.Fatalf("decode ownerless request sources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("ownerless request exposed sources: %#v", sources)
	}

	foreignRequest := httptest.NewRequest(http.MethodPost, "/sources/"+bobID.String()+"/pause", nil)
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign pause status = %d, body=%s", foreignResponse.Code, foreignResponse.Body.String())
	}
}

func containsSourceID(sources []models.ConnectedSource, id uuid.UUID) bool {
	for _, source := range sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func TestHandlerBoundsAuditLogsAfterOwnerFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	aliceID := uuid.New()
	bobID := uuid.New()
	repo := newFakeSourceRepo(
		&models.ConnectedSource{ID: aliceID, OwnerIdentity: "alice", Name: "Alice source", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: bobID, OwnerIdentity: "bob", Name: "Bob source", Enabled: true, Status: "active"},
	)
	repo.auditLogs = []models.SourceAuditLog{
		{ID: uuid.New(), SourceID: bobID, Action: "bob.hidden", Message: "must be filtered first"},
		{ID: uuid.New(), SourceID: aliceID, Action: "alice.latest", Message: "latest"},
		{ID: uuid.New(), SourceID: aliceID, Action: "alice.previous", Message: "previous"},
		{ID: uuid.New(), SourceID: aliceID, Action: "alice.old", Message: "old"},
	}

	handler := NewHandler(NewService(repo, nil))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(identity.ContextSubjectKey, "alice") })
	router.GET("/sources/audit-logs", handler.AuditLogs)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sources/audit-logs?limit=2", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("audit logs status = %d, body=%s", response.Code, response.Body.String())
	}
	var logs []models.SourceAuditLog
	if err := json.Unmarshal(response.Body.Bytes(), &logs); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	if len(logs) != 2 || logs[0].SourceID != aliceID || logs[1].SourceID != aliceID {
		t.Fatalf("bounded owner logs = %#v, want two Alice logs", logs)
	}

	for _, query := range []string{"limit=0", "limit=501", "limit=invalid"} {
		invalid := httptest.NewRecorder()
		router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/sources/audit-logs?"+query, nil))
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, body=%s", query, invalid.Code, invalid.Body.String())
		}
	}
}

func TestGoogleOAuthStartRejectsForeignSourceBeforeConfigurationLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	foreignID := uuid.New()
	handler := NewHandler(NewService(newFakeSourceRepo(
		&models.ConnectedSource{ID: foreignID, OwnerIdentity: "bob", ConnectorKey: gmailConnectorKey, Name: "Bob Gmail", Enabled: true, Status: "active"},
	), nil))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(identity.ContextSubjectKey, "alice") })
	router.GET("/sources/oauth/google/start", handler.StartGoogleOAuth)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sources/oauth/google/start?sourceId="+foreignID.String(), nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign OAuth start status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerRunsDueSyncsOnlyForAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(newFakeSourceRepo(), nil)
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/sources/sync-due", handler.RunDueScheduledSyncs)

	unauthenticatedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthenticatedResponse, httptest.NewRequest(http.MethodPost, "/sources/sync-due", nil))
	if unauthenticatedResponse.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status = %d, want 403: %s", unauthenticatedResponse.Code, unauthenticatedResponse.Body.String())
	}

	authenticatedRouter := gin.New()
	authenticatedRouter.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
	})
	authenticatedRouter.POST("/sources/sync-due", handler.RunDueScheduledSyncs)
	authenticatedResponse := httptest.NewRecorder()
	authenticatedRouter.ServeHTTP(authenticatedResponse, httptest.NewRequest(http.MethodPost, "/sources/sync-due", nil))
	if authenticatedResponse.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200: %s", authenticatedResponse.Code, authenticatedResponse.Body.String())
	}
}

func TestHandlerListsOnlyOwnerScopedExtractionsFromRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	aliceID := uuid.New()
	bobID := uuid.New()
	repo := newFakeSourceRepo(
		&models.ConnectedSource{ID: aliceID, OwnerIdentity: "alice", Name: "Alice source", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: bobID, OwnerIdentity: "bob", Name: "Bob source", Enabled: true, Status: "active"},
	)
	if _, err := repo.SaveExtraction(&models.SourceExtraction{ID: uuid.New(), SourceID: aliceID, Summary: "Alice private context"}); err != nil {
		t.Fatalf("SaveExtraction Alice: %v", err)
	}
	if _, err := repo.SaveExtraction(&models.SourceExtraction{ID: uuid.New(), SourceID: bobID, Summary: "Bob private context"}); err != nil {
		t.Fatalf("SaveExtraction Bob: %v", err)
	}
	handler := NewHandler(NewService(repo, nil))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
	})
	router.GET("/sources/extractions", handler.Extractions)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sources/extractions", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("extractions status = %d, body=%s", response.Code, response.Body.String())
	}
	var extractions []models.SourceExtraction
	if err := json.Unmarshal(response.Body.Bytes(), &extractions); err != nil {
		t.Fatalf("decode extractions: %v", err)
	}
	if len(extractions) != 1 || extractions[0].SourceID != aliceID {
		t.Fatalf("visible extractions = %#v, want only Alice extraction", extractions)
	}
	for _, sourceID := range repo.lastExtractionSourceIDs {
		if sourceID == bobID {
			t.Fatalf("handler repository query included Bob's private source")
		}
	}
}

func TestHandlerRejectsOwnerlessLegacySourceAndExtractionMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sourceID := uuid.New()
	extractionID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID:      sourceID,
		Name:    "Legacy local source",
		Enabled: true,
		Status:  "active",
	})
	if _, err := repo.SaveExtraction(&models.SourceExtraction{ID: extractionID, SourceID: sourceID, Summary: "Legacy context"}); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	handler := NewHandler(NewService(repo, nil))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
	})
	router.POST("/sources/:id/pause", handler.Pause)
	router.POST("/sources/extractions/:id/archive", handler.ArchiveExtraction)

	pauseResponse := httptest.NewRecorder()
	router.ServeHTTP(pauseResponse, httptest.NewRequest(http.MethodPost, "/sources/"+sourceID.String()+"/pause", nil))
	if pauseResponse.Code != http.StatusNotFound {
		t.Fatalf("ownerless source pause status = %d, want 404: %s", pauseResponse.Code, pauseResponse.Body.String())
	}

	archiveResponse := httptest.NewRecorder()
	router.ServeHTTP(archiveResponse, httptest.NewRequest(http.MethodPost, "/sources/extractions/"+extractionID.String()+"/archive", nil))
	if archiveResponse.Code != http.StatusNotFound {
		t.Fatalf("ownerless extraction archive status = %d, want 404: %s", archiveResponse.Code, archiveResponse.Body.String())
	}

	storedSource, err := repo.FindSource(sourceID)
	if err != nil {
		t.Fatalf("FindSource: %v", err)
	}
	if !storedSource.Enabled || storedSource.Status != "active" {
		t.Fatalf("ownerless source was mutated: %#v", storedSource)
	}
	storedExtraction, err := repo.FindExtraction(extractionID)
	if err != nil {
		t.Fatalf("FindExtraction: %v", err)
	}
	if storedExtraction.Archived {
		t.Fatalf("ownerless extraction was archived: %#v", storedExtraction)
	}
}

func TestHandlerTranscribesOnlyAnOwnedExplicitAudioSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, OwnerIdentity: "alice", ConnectorKey: "whisper-audio", Name: "Meeting notes", Category: "audio",
		Enabled: true, LocalOnly: true, Status: "active", SyncTarget: "voice-notes/2026-07", DefaultProjectKey: "Robert-life-os",
	})
	stub := &sourceTranscriberStub{transcripts: []whispercpp.Transcript{{Path: "voice-notes/2026-07/meeting.m4a", Text: "Follow up with the lawyer.", ModelID: "ggml-base.en.bin", Language: "en"}}}
	handler := NewHandler(NewService(repo, nil), stub)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(identity.ContextSubjectKey, "alice") })
	router.POST("/sources/:id/transcribe", handler.Transcribe)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sources/"+sourceID.String()+"/transcribe", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("transcribe status = %d, body=%s", response.Code, response.Body.String())
	}
	if stub.folder != "voice-notes/2026-07" {
		t.Fatalf("folder = %q", stub.folder)
	}
	if len(repo.rawItems) != 1 {
		t.Fatalf("raw items = %#v", repo.rawItems)
	}
	var raw *models.SourceRawItem
	for _, item := range repo.rawItems {
		raw = item
	}
	if raw == nil || raw.SourceURI == "" || raw.ItemType != "audio_transcript" {
		t.Fatalf("raw item = %#v", raw)
	}
	if !strings.HasPrefix(raw.SourceURI, "audio://selected-source/") {
		t.Fatalf("source uri = %q", raw.SourceURI)
	}
}

func TestHandlerTranscriptionRejectsCallerPayloadAndNonAudioSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{ID: sourceID, OwnerIdentity: "alice", ConnectorKey: "local-folder", Name: "Files", Category: "local_folder", Enabled: true, LocalOnly: true, Status: "active", SyncTarget: "notes"})
	handler := NewHandler(NewService(repo, nil), &sourceTranscriberStub{})
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(identity.ContextSubjectKey, "alice") })
	router.POST("/sources/:id/transcribe", handler.Transcribe)

	payloadResponse := httptest.NewRecorder()
	router.ServeHTTP(payloadResponse, httptest.NewRequest(http.MethodPost, "/sources/"+sourceID.String()+"/transcribe", strings.NewReader(`{"path":"anywhere"}`)))
	if payloadResponse.Code != http.StatusBadRequest {
		t.Fatalf("payload status = %d, body=%s", payloadResponse.Code, payloadResponse.Body.String())
	}

	nonAudioResponse := httptest.NewRecorder()
	nonAudioRequest := httptest.NewRequest(http.MethodPost, "/sources/"+sourceID.String()+"/transcribe", nil)
	router.ServeHTTP(nonAudioResponse, nonAudioRequest)
	if nonAudioResponse.Code != http.StatusBadRequest {
		t.Fatalf("non-audio status = %d, body=%s", nonAudioResponse.Code, nonAudioResponse.Body.String())
	}
}
