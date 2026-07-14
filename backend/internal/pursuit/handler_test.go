package pursuit

import (
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
