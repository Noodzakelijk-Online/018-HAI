package pursuit

import (
	"automation-hub-backend/internal/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func TestDelegationPackageEndpointDoesNotExposeAnotherOwnersWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	alice, err := service.Create(CreateRequest{Title: "Alice delegation", OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("Create Alice pursuit: %v", err)
	}
	bob, err := service.Create(CreateRequest{Title: "Bob delegation", OwnerIdentity: "bob"})
	if err != nil {
		t.Fatalf("Create Bob pursuit: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.GET("/pursuits/:id/delegation", NewHandler(service).DelegationPackage)

	visible := httptest.NewRecorder()
	router.ServeHTTP(visible, httptest.NewRequest(http.MethodGet, "/pursuits/"+alice.ID.String()+"/delegation", nil))
	if visible.Code != http.StatusOK {
		t.Fatalf("own delegation package status = %d, body=%s", visible.Code, visible.Body.String())
	}
	var packageResult PursuitDelegationPackage
	if err := json.Unmarshal(visible.Body.Bytes(), &packageResult); err != nil {
		t.Fatalf("decode delegation package: %v", err)
	}
	if packageResult.PursuitID != alice.ID.String() || packageResult.Title != alice.Title {
		t.Fatalf("delegation package identity = %#v", packageResult)
	}

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/pursuits/"+bob.ID.String()+"/delegation", nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("cross-owner delegation status = %d, want %d; body=%s", denied.Code, http.StatusNotFound, denied.Body.String())
	}
}

func TestResolveDecisionEndpointRejectsHiddenCrossOwnerCompletionEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	alice, err := service.Create(CreateRequest{Title: "Alice completion", OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("Create Alice pursuit: %v", err)
	}
	bobWorkflowID := uuid.New()
	repo.workflows[bobWorkflowID] = models.WorkflowItem{
		ID:                 bobWorkflowID,
		OwnerIdentity:      "bob",
		Title:              "Bob verified workflow",
		CurrentState:       "completed",
		VerificationStatus: "verified",
	}
	// Simulate a malformed legacy link that predates owner-aware link checks.
	legacyLinkID := uuid.New()
	repo.links[legacyLinkID] = models.PursuitLink{
		ID:           legacyLinkID,
		PursuitID:    alice.ID,
		LinkType:     LinkWorkflow,
		LinkID:       bobWorkflowID.String(),
		Relationship: "legacy_evidence",
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.POST("/pursuits/:id/decisions/resolve", NewHandler(service).ResolveDecision)

	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{"decisionId":%q,"decisionType":"pursuit_completion_review","approved":true}`, completionReviewDecisionID(alice.ID))
	request := httptest.NewRequest(http.MethodPost, "/pursuits/"+alice.ID.String()+"/decisions/resolve", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("cross-owner completion status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	stored, err := repo.FindByID(alice.ID)
	if err != nil {
		t.Fatalf("Find Alice pursuit: %v", err)
	}
	if stored.Status == StatusCompleted || stored.CompletionState == CompletionVerified {
		t.Fatalf("hidden cross-owner evidence completed Alice pursuit: %#v", stored)
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

func TestPursuitLinkRejectsAnotherOwnersPrivateRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	pursuit, err := service.Create(CreateRequest{Title: "Alice pursuit", OwnerIdentity: "alice"})
	if err != nil {
		t.Fatalf("Create Alice pursuit: %v", err)
	}
	bobWorkflowID := uuid.New()
	repo.workflows[bobWorkflowID] = models.WorkflowItem{ID: bobWorkflowID, OwnerIdentity: "bob", Title: "Bob private workflow"}
	bobMemoryID := uuid.New()
	repo.memories[bobMemoryID] = models.ContextMemory{ID: bobMemoryID, OwnerIdentity: "bob", Content: "Bob private memory"}
	bobSourceID := uuid.New()
	repo.sourceOwners[bobSourceID] = "bob"
	repo.sourceItems[uuid.New()] = models.SourceRawItem{ID: uuid.New(), SourceID: bobSourceID, ExternalID: "bob-private-source", Title: "Bob private source"}
	bobVerificationID := uuid.New()
	repo.verificationRuns[bobVerificationID] = models.VerificationRun{ID: bobVerificationID, OwnerIdentity: "bob", Question: "Bob private verification"}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	router.POST("/pursuits/:id/links", NewHandler(service).Link)

	for _, target := range []struct {
		linkType string
		linkID   string
	}{
		{linkType: LinkWorkflow, linkID: bobWorkflowID.String()},
		{linkType: LinkMemory, linkID: bobMemoryID.String()},
		{linkType: LinkSourceItem, linkID: "bob-private-source"},
		{linkType: LinkVerification, linkID: bobVerificationID.String()},
	} {
		recorder := httptest.NewRecorder()
		body := fmt.Sprintf(`{"linkType":%q,"linkId":%q,"relationship":"evidence"}`, target.linkType, target.linkID)
		request := httptest.NewRequest(http.MethodPost, "/pursuits/"+pursuit.ID.String()+"/links", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("cross-owner %s link status = %d, want %d; body=%s", target.linkType, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	}
	links, err := repo.FindLinks(pursuit.ID)
	if err != nil {
		t.Fatalf("FindLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("cross-owner link was persisted: %#v", links)
	}
}
