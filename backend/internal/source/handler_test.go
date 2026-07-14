package source

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestHandlerOnlyListsVisibleSourcesAndRejectsForeignControls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	aliceID := uuid.New()
	bobID := uuid.New()
	service := NewService(newFakeSourceRepo(
		&models.ConnectedSource{ID: aliceID, OwnerIdentity: "alice", Name: "Alice source", Enabled: true, Status: "active"},
		&models.ConnectedSource{ID: bobID, OwnerIdentity: "bob", Name: "Bob source", Enabled: true, Status: "active"},
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

	foreignRequest := httptest.NewRequest(http.MethodPost, "/sources/"+bobID.String()+"/pause", nil)
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign pause status = %d, body=%s", foreignResponse.Code, foreignResponse.Body.String())
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
