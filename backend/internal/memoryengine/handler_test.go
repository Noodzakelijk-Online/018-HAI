package memoryengine

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestImportHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &memoryEngineRepoStub{}
	handler := NewHandler(NewService(repo, &memoryEngineMemoryStub{}, nil, "test-memory-encryption-secret"))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	engine.POST("/memory-engine/import", handler.Import)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/memory-engine/import", strings.NewReader(`{
		"ownerIdentity":"untrusted",
		"platform":"chatgpt",
		"externalId":"owner-test",
		"title":"Owner test",
		"sourceUri":"https://chatgpt.com/c/owner-test",
		"messages":[{"role":"user","content":"Draft an internal follow-up."}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("import status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(repo.conversations) != 1 || repo.conversations[0].OwnerIdentity != "alice" {
		t.Fatalf("stored conversations = %#v, want verified owner alice", repo.conversations)
	}
}

func TestDeleteConversationHandlerRejectsOwnerlessLegacyArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conversationID := uuid.New()
	repo := &memoryEngineRepoStub{conversations: []models.AIConversationArchive{{
		ID:         conversationID,
		Platform:   "chatgpt",
		ExternalID: "legacy-thread",
		SourceURI:  "https://chatgpt.com/c/legacy-thread",
	}}}
	handler := NewHandler(NewService(repo, &memoryEngineMemoryStub{}, nil, "test-memory-encryption-secret"))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	engine.DELETE("/memory-engine/conversations/:id", handler.DeleteConversation)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/memory-engine/conversations/"+conversationID.String(), nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("delete ownerless archive status = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
	if len(repo.conversations) != 1 || repo.conversations[0].ID != conversationID {
		t.Fatalf("ownerless archive was deleted: %#v", repo.conversations)
	}
}
