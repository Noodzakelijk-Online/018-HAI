package pursuit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestVerifiedActorUsesAuthenticatedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(identity.ContextSubjectKey, "bundled-idp-user")

	if got := verifiedActor(context, "operator"); got != "bundled-idp-user" {
		t.Fatalf("verifiedActor() = %q, want authenticated principal", got)
	}
}

func TestVerifiedActorCanLeaveOwnerUnsetWithoutSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got := verifiedActor(context, ""); got != "" {
		t.Fatalf("verifiedActor() = %q, want empty owner without an authenticated session", got)
	}
}

func TestVerifiedActorDoesNotUseClientSuppliedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got := verifiedActor(context, "operator"); got != "operator" {
		t.Fatalf("verifiedActor() = %q, want honest local fallback", got)
	}
}

func TestArchiveEndpointRequiresExplicitArchiveIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	pursuit, err := service.Create(CreateRequest{Title: "Keep lifecycle explicit"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	router := gin.New()
	router.POST("/pursuits/:id/archive", NewHandler(service).Archive)

	archive := httptest.NewRecorder()
	router.ServeHTTP(archive, httptest.NewRequest(http.MethodPost, "/pursuits/"+pursuit.ID.String()+"/archive", strings.NewReader(`{"archived":true}`)))
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body=%s", archive.Code, archive.Body.String())
	}

	for _, body := range []string{`{"archived":false}`, `{}`, `{`} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pursuits/"+pursuit.ID.String()+"/archive", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("archive body %q status = %d, want %d; body=%s", body, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
		stored, findErr := repo.FindByID(pursuit.ID)
		if findErr != nil {
			t.Fatalf("FindByID after archive body %q: %v", body, findErr)
		}
		if !stored.Archived || stored.Status != StatusArchived {
			t.Fatalf("archive body %q changed closed pursuit: %#v", body, stored)
		}
	}
}

func TestPursuitEndpointsScopeRecordsToAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	alice, err := service.Create(CreateRequest{Title: "Alice pursuit", OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("Create Alice pursuit: %v", err)
	}
	bob, err := service.Create(CreateRequest{Title: "Bob pursuit", OwnerIdentity: "bob"})
	if err != nil {
		t.Fatalf("Create Bob pursuit: %v", err)
	}
	legacy, err := service.Create(CreateRequest{Title: "Local legacy pursuit"})
	if err != nil {
		t.Fatalf("Create legacy pursuit: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	handler := NewHandler(service)
	router.GET("/pursuits", handler.List)
	router.GET("/pursuits/:id", handler.Get)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/pursuits", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}
	var visible []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &visible); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	seen := map[string]bool{}
	for _, pursuit := range visible {
		seen[pursuit.ID] = true
	}
	if !seen[alice.ID.String()] || !seen[legacy.ID.String()] || seen[bob.ID.String()] {
		t.Fatalf("owner-scoped list leaked or hid records: %#v", seen)
	}

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/pursuits/"+bob.ID.String(), nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("cross-owner detail status = %d, want %d; body=%s", denied.Code, http.StatusNotFound, denied.Body.String())
	}
}

func TestPursuitMatchDoesNotExposeAnotherOwnersSourceLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	bob, err := service.Create(CreateRequest{Title: "Bob private claim", OwnerIdentity: "bob", ProjectKey: "private"})
	if err != nil {
		t.Fatalf("Create Bob pursuit: %v", err)
	}
	if _, err := service.Link(bob.ID, LinkRequest{LinkType: LinkSourceItem, LinkID: "private-source", Relationship: "evidence"}); err != nil {
		t.Fatalf("Link Bob source: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.POST("/pursuits/match", NewHandler(service).Match)

	recorder := httptest.NewRecorder()
	recorderRequest := httptest.NewRequest(http.MethodPost, "/pursuits/match", strings.NewReader(`{"sourceType":"source_item","sourceId":"private-source"}`))
	recorderRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, recorderRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("match status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var matches []MatchCandidate
	if err := json.Unmarshal(recorder.Body.Bytes(), &matches); err != nil {
		t.Fatalf("decode matches: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("cross-owner source link was matched: %#v", matches)
	}
}
